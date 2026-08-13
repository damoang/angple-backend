package middleware

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// 탈퇴(숙려 포함) 회원의 API 접근 차단 — 분쟁조정위 26R05-00197 Phase 1.
//
// 배경:
//
//	탈퇴해도 이미 발급된 access token(TTL 15분)이 그대로 통했다. JWTAuth 는
//	토큰 서명만 보고 mb_leave_date 를 확인하지 않았기 때문이다. 그래서 세션·토큰을
//	모두 파기해도 **탈퇴자가 글·댓글·리액션을 계속 쓸 수 있었고**, 그때마다
//	포인트·XP 가 움직였다. 신고된 증상("탈퇴 뒤 활동 기록 갱신")의 두 번째 경로다.
//	세션 파기(purgeAuthArtifacts)로는 닫히지 않는다 — 세션 행을 만들지 않는 경로라
//	"잔존 0행"은 지켜지면서 증상만 재현된다.

var (
	withdrawalCheckDB    *gorm.DB
	withdrawalCheckRedis *redis.Client
)

// SetWithdrawalCheck 는 탈퇴 게이트가 쓸 DB·Redis 를 주입한다.
//
// ⛔ JWTAuth 의 시그니처를 바꾸지 않기 위한 패키지 레벨 주입이다.
// 호출처가 77곳이라 인자를 하나 늘리면 전부 고쳐야 한다
// (handler.SetAuthCacheRedis 와 같은 선례).
//
// ⛔ 라우팅 개시(router.Run) 전에 부를 것.
// db 가 nil 이면 게이트는 통째로 skip 된다 — 로컬·테스트에서 안전하고,
// 문제가 생기면 **이미지 롤백 없이 이 배선 한 줄만 빼서** 끌 수 있다.
func SetWithdrawalCheck(db *gorm.DB, rdb *redis.Client) {
	withdrawalCheckDB = db
	withdrawalCheckRedis = rdb
}

// withdrawalGateTTL 은 판정 캐시 수명이다. 짧게 잡아 취소 후 복귀가 오래 막히지 않게 한다
// (취소 시 cancelSelfLeave 가 키를 지우므로 정상 경로는 즉시 반영된다).
const withdrawalGateTTL = 60 * time.Second

// WithdrawalGateKey 는 탈퇴 판정 캐시 키다.
//
// ⛔ AUTH_CACHE_NAMESPACES 접두사를 붙이지 않는다. 이 키는 **Go 만 읽고 쓴다** —
// web 의 TieredCache 키(sess:·member:)와 달리 web 이 관여하지 않으므로
// 네임스페이스를 태우면 handler 와 middleware 가 서로 다른 키를 보게 될 뿐이다.
func WithdrawalGateKey(mbID string) string { return "authgate:left:" + mbID }

// withdrawalAllowlist 는 탈퇴 상태에서도 통과시킬 라우트다.
//
// ⛔ 매칭은 c.FullPath()(라우트 패턴)로 한다. c.Request.URL.Path 는
// 트레일링 슬래시·쿼리 변형에 취약해 우회가 생긴다.
//
// 왜 이 둘뿐인가 — 취소 화면은 로그인 상태가 아니다. 서명된 grace 쿠키만 들고
// 순수 form action 으로 동작해서 클라이언트 API 호출이 0건이다. 그래서 JWTAuth 를
// 타는 것은 아래 취소 요청 하나뿐이다.
var withdrawalAllowlist = map[string]bool{
	// 유일한 복귀 통로. 막으면 숙려 회원이 영구히 돌아올 수 없다.
	"DELETE /api/v1/members/me/leave": true,
	// 로그아웃까지 막을 이유가 없다. 막으면 클라이언트가 토큰을 정상적으로 버리지 못한다.
	"POST /api/v1/auth/logout": true,
}

// blockIfWithdrawn 은 탈퇴 회원이면 403 으로 끊고 true 를 돌려준다.
// JWTAuth 의 **두 인증 분기 각각**에서 c.Next() 직전에 호출한다.
//
// ⛔ 내부신뢰 분기(X-Internal-Auth: sveltekit-session)를 빠뜨리면 안 된다.
// 그 분기는 JWT 검증을 통째로 건너뛰므로, Bearer 쪽에만 게이트를 걸면
// 그대로 우회로가 된다. web 은 두 헤더를 **동시에** 보낸다.
//
// 판정 키는 mb_id 다. Bearer 분기의 userID 는 v2_users.id(숫자)라
// g5_member 조회에 쓸 수 없다 — 반드시 username 을 넘길 것.
func blockIfWithdrawn(c *gin.Context, mbID string) bool {
	if withdrawalCheckDB == nil || mbID == "" {
		return false
	}
	if withdrawalAllowlist[c.Request.Method+" "+c.FullPath()] {
		return false
	}
	if !isWithdrawn(c.Request.Context(), mbID) {
		return false
	}
	common.ErrorResponse(c, http.StatusForbidden,
		"탈퇴 처리된 계정입니다. 숙려기간 내라면 탈퇴 취소 후 이용해 주세요.", nil)
	c.Abort()
	return true
}

// isWithdrawn 은 mb_leave_date 가 설정돼 있는지 본다. 캐시 우선, 미스 시 DB 1회.
//
// ⛔ **fail-open 이다** — DB 오류 시 false(통과)를 돌려준다.
//
//	fail-closed 로 하면 DB 블립 하나에 인증 사용자 6.2만명 전원이 403 이 된다.
//	막지 못해 생기는 노출(탈퇴자 일부가 잠깐 통과)보다 장애 규모가 압도적으로 크다.
//	대신 실패를 반드시 로그로 남긴다 — 열린 상태가 조용히 지속되면 안 된다.
func isWithdrawn(ctx context.Context, mbID string) bool {
	key := WithdrawalGateKey(mbID)

	if withdrawalCheckRedis != nil {
		rctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		v, err := withdrawalCheckRedis.Get(rctx, key).Result()
		cancel()
		if err == nil {
			return v == "1"
		}
		if err != redis.Nil {
			// 캐시 장애는 치명적이지 않다(DB 로 폴백). 다만 조용히 넘기면
			// 모든 요청이 DB 를 때리는 상태를 눈치채지 못한다.
			log.Printf("[withdrawalGate] 캐시 조회 실패 (%s): %v", mbID, err)
		}
	}

	var leaveDate string
	err := withdrawalCheckDB.WithContext(ctx).Table("g5_member").
		Select("mb_leave_date").Where("mb_id = ?", mbID).
		Limit(1).Scan(&leaveDate).Error
	if err != nil {
		// ⛔ fail-open. 여기서 막으면 DB 블립이 전면 장애가 된다.
		log.Printf("[withdrawalGate] ⛔ DB 조회 실패 — fail-open 통과 (%s): %v", mbID, err)
		return false
	}

	withdrawn := leaveDate != "" && leaveDate != "0"
	if withdrawalCheckRedis != nil {
		val := "0"
		if withdrawn {
			val = "1"
		}
		rctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		if err := withdrawalCheckRedis.Set(rctx, key, val, withdrawalGateTTL).Err(); err != nil {
			log.Printf("[withdrawalGate] 캐시 저장 실패 (%s): %v", mbID, err)
		}
		cancel()
	}
	return withdrawn
}
