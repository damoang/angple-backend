package handler

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// authCacheRedis 는 web(SvelteKit) 이 쓰는 세션·회원 캐시를 무효화하기 위한 Redis 핸들이다.
// main 에서 SetAuthCacheRedis 로 주입한다. nil 이면 캐시 무효화를 건너뛴다(DB 파기 자체는 진행).
var authCacheRedis *redis.Client

// SetAuthCacheRedis 는 인증 캐시 무효화용 Redis 클라이언트를 주입한다.
// ⛔ 라우팅 개시(router.Run) 전에 불러야 한다. main 의 Redis 초기화 직후 배선돼 있다.
func SetAuthCacheRedis(c *redis.Client) { authCacheRedis = c }

// defaultAuthCacheNamespaces 는 AUTH_CACHE_NAMESPACES 미설정 시 **추가로** 커버할
// 네임스페이스다(접두사 없음 = prod 는 항상 포함되므로 여기 적지 않는다).
// 2026-08-12 클러스터 전수 실측 기준 CACHE_NAMESPACE 를 쓰는 워크로드는
// angple-web-canary("canary") 하나뿐이었다.
const defaultAuthCacheNamespaces = "canary"

// purgeAuthArtifacts 는 회원의 인증 산출물(세션·리프레시 토큰)을 파기하고,
// web 이 들고 있는 세션·회원 캐시(L2)까지 무효화한다.
//
// 왜 — 분쟁조정위 26R05-00197 로 드러난 문제:
//
//	탈퇴 처리가 기존 세션을 무효화하지 않아, 탈퇴 전에 발급된 세션 쿠키로
//	인증 상태가 유지됐다. 신청인 계정에서 탈퇴 17일 뒤 포인트·XP·mb_today_login
//	이 갱신된 것이 그 결과다(신규 로그인이 아니라 잔존 세션에 의한 접근).
//
// ⛔ **이 함수가 탈퇴 파기의 단일 소유자다.**
//
//	web 쪽 탈퇴 라우트(routes/member/leave/+page.server.ts)는 파기를 부르지 않는다.
//	예전에는 web 도 같이 불렀지만 소득이 거의 없었다 — 이 함수가 먼저 DB 행을 지우고
//	오므로 web 의 세션 조회가 0행이 되어 sess: 키를 하나도 못 지운다.
//	(회원 캐시 invalidateMemberCache 는 DB 와 무관하게 지워지지만, 그건 아래
//	 네임스페이스 목록으로 여기서 이미 커버한다 — canary 까지 포함해서.)
//	즉 web 호출은 "처리한 파드 1대의 회원 캐시 L1" 만 더 지우는 중복이었고,
//	파기 주체가 둘로 나뉘어 추적만 어려워졌다. 그래서 여기 한 곳으로 모았다.
//	새 탈퇴 진입점을 만들면 여기를 부를 것.
//
// ⛔ revoke 가 아니라 DELETE 다.
//
//	angple_sessions·angple_refresh_tokens 는 IP·User-Agent 를 보유해
//	사실상 접속기록이다. 행을 남기면 개인정보가 그대로 보존된다.
//
// ⛔ **DB 만 지우면 부족하다.** web 의 TieredCache 가 L2(Redis, TTL 300초)에
//
//	세션(sess:<hash>)과 회원(member:<mb_id>)을 들고 있어, DB 행이 사라져도
//	최대 5분간 캐시로 인증이 통과한다. 그래서 Redis 키까지 지운다.
//
// ⛔ 실패해도 에러를 반환하지 않는다.
//
//	회원 UPDATE 가 끝난 뒤 호출되며, 파기 실패로 탈퇴가 되돌아가면 안 된다.
//	회원은 이미 탈퇴를 신청했고, 실패하면 더 곤란해진다. 로그만 남긴다.
//
// ⛔ 트랜잭션에 묶어 호출하지 말 것. 위 원칙이 깨진다.
// ⛔ 탈퇴 **취소** 경로에서는 부르지 않는다 — 복귀시키는 동작이다.
//
// ⚠️ 남는 창: web 의 L1(프로세스 메모리, TTL 60초)은 여기서 지울 수 없다.
//
//	파드가 여러 개라 크로스파드 무효화 수단이 없기 때문이다. 세션 캐시와
//	회원 캐시가 **동시에** warm 한 파드에서 최대 60초간 인증이 유지될 수 있고,
//	그 뒤 자연 소멸한다. 따라서 대외 문안에 "즉시 차단"은 쓰지 말 것.
//
// 호출처: applySelfLeave(자가 탈퇴) · AdminMemberHandler.UpdateMember(관리자 탈퇴).
func purgeAuthArtifacts(db *gorm.DB, mbID string) {
	if mbID == "" {
		return
	}

	// ① 회원 캐시부터 지운다 — **순서가 중요하다.**
	//    이 시점에 mb_leave_date 는 이미 커밋돼 있다. 회원 캐시를 먼저 비우면 이후
	//    누가 캐시를 재적재해도 "탈퇴함"을 읽으므로, hooks.server.ts 의 탈퇴자 세션
	//    차단(2차 방어)이 확실히 발화한다.
	//    반대로 세션 행을 먼저 지우면, DELETE 와 캐시 DEL 사이 수 ms 동안
	//    "L2 세션 히트 + 낡은 회원 캐시" 조합으로 통과하는 창이 생긴다.
	memberDeleted := delWebAuthCacheKeys(mbID, buildAuthCacheKeys("member:", []string{mbID}))

	// ② 세션 캐시 키 확보 — DELETE 전에 해야 한다. 지운 뒤에는 어떤 해시였는지 알 수 없다.
	//    ⛔ Pluck 을 쓴다(단일 컬럼 전용 API). Scan(&[]string) 도 동작하지만 그건
	//       "컬럼이 정확히 하나"일 때만 성립해서, 컬럼을 하나 추가하면 조용히 깨진다.
	var hashes []string
	if err := db.Table("angple_sessions").Where("mb_id = ?", mbID).
		Pluck("session_id_hash", &hashes).Error; err != nil {
		log.Printf("[purgeAuthArtifacts] 세션 해시 조회 실패 (%s): %v", mbID, err)
	}

	// ③ DB 파기.
	var sessionRows, tokenRows int64
	if res := db.Exec("DELETE FROM angple_sessions WHERE mb_id = ?", mbID); res.Error != nil {
		log.Printf("[purgeAuthArtifacts] 세션 파기 실패 (%s): %v", mbID, res.Error)
	} else {
		sessionRows = res.RowsAffected
	}
	if res := db.Exec("DELETE FROM angple_refresh_tokens WHERE mb_id = ?", mbID); res.Error != nil {
		log.Printf("[purgeAuthArtifacts] 리프레시 토큰 파기 실패 (%s): %v", mbID, res.Error)
	} else {
		tokenRows = res.RowsAffected
	}

	// ④ 세션 캐시 키 삭제.
	sessDeleted := delWebAuthCacheKeys(mbID, buildAuthCacheKeys("sess:", hashes))

	// ⑤ 탈퇴 게이트 캐시를 "탈퇴함"으로 즉시 세팅한다.
	//    안 하면 게이트가 최대 TTL(60초) 동안 낡은 "미탈퇴" 판정을 들고 있어
	//    그 사이 API 요청이 통과한다. 실패해도 무시 — 게이트가 캐시 미스 시
	//    DB 를 보므로 결과는 같고, 조금 늦어질 뿐이다.
	if authCacheRedis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := authCacheRedis.Set(ctx, middleware.WithdrawalGateKey(mbID), "1", time.Minute).Err(); err != nil {
			log.Printf("[purgeAuthArtifacts] 탈퇴 게이트 캐시 세팅 실패 (%s): %v", mbID, err)
		}
		cancel()
	}

	// 파기 사실을 한 줄로 남긴다. 조사 대응상 "언제 무엇을 지웠는지"의 근거가 되고,
	// 캐시 키가 있었는데 0개 삭제되면 네임스페이스 불일치 신호다(Del 은 없는 키에도
	// 에러를 내지 않으므로, 개수를 안 남기면 조용히 실패한다).
	log.Printf("[purgeAuthArtifacts] mb_id=%s sessions=%d tokens=%d cache(member=%d sess=%d/%d)",
		mbID, sessionRows, tokenRows, memberDeleted, sessDeleted, len(hashes)*len(authCacheKeyPrefixes()))
}

// authCacheKeyPrefixes 는 무효화 대상 캐시 네임스페이스의 키 접두사 목록을 돌려준다.
//
// web 의 TieredCache 는 CACHE_NAMESPACE 가 있으면 `{ns}:{prefix}:{key}`,
// 없으면 `{prefix}:{key}` 로 키를 만든다(web cache.ts 의 redisKey()).
//
// ⛔ **백엔드 자신의 CACHE_NAMESPACE 를 쓰면 안 된다.** 지워야 하는 것은 web 의 키인데,
//
//	web 과 백엔드의 네임스페이스는 서로 다를 수 있다. 2026-08-12 실측에서
//	angple-web-canary 는 CACHE_NAMESPACE=canary 인데 그 요청을 받는 백엔드
//	(canary web 의 BACKEND_URL 은 prod 백엔드를 가리킨다)는 빈 값이었다.
//	백엔드 기준으로 조립하면 카나리 web 의 키를 하나도 못 지운다.
//
// 그래서 목록을 env 로 받는다 — AUTH_CACHE_NAMESPACES, 콤마 구분.
//
//	AUTH_CACHE_NAMESPACES="canary"        →  ["", "canary:"]
//	AUTH_CACHE_NAMESPACES="canary,blue"   →  ["", "canary:", "blue:"]
//	(미설정)                               →  ["", "canary:"]
//
// ⛔ **env 는 "추가"이지 "치환"이 아니다.** 접두사 없음("")은 항상 무조건 포함한다.
//
//	치환식으로 만들면 "canary 를 추가하자"는 의도로 AUTH_CACHE_NAMESPACES=canary
//	한 줄을 넣는 순간 prod 키 커버가 통째로 빠진다 — 그것도 로그 한 줄 없이 조용히
//	(Del 은 없는 키에도 에러를 내지 않는다). 이 사안 전체의 교훈이
//	"조용히 깨지는 구조 제거"이므로 그 형태를 만들지 않는다.
//
// ⛔ web 에 새 CACHE_NAMESPACE 를 추가하면 이 env 도 함께 갱신할 것.
//
//	빠뜨리면 그 환경의 캐시만 조용히 안 지워진다.
func authCacheKeyPrefixes() []string {
	// 접두사 없음(prod)은 항상 첫 번째. 아래 env 는 여기에 더하기만 한다.
	prefixes := []string{""}
	seen := map[string]struct{}{"": {}}

	raw := os.Getenv("AUTH_CACHE_NAMESPACES")
	if raw == "" {
		raw = defaultAuthCacheNamespaces
	}
	for _, ns := range strings.Split(raw, ",") {
		ns = strings.TrimSpace(ns)
		if ns == "" {
			continue // 빈 항목은 무시 — "" 는 위에서 이미 보장된다
		}
		p := ns + ":"
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		prefixes = append(prefixes, p)
	}
	return prefixes
}

// buildAuthCacheKeys 는 (네임스페이스 × id) 조합으로 Redis 키를 만든다.
// kind 는 web TieredCache 의 prefix + 콜론 — "sess:" 또는 "member:".
func buildAuthCacheKeys(kind string, ids []string) []string {
	prefixes := authCacheKeyPrefixes()
	keys := make([]string, 0, len(ids)*len(prefixes))
	for _, id := range ids {
		if id == "" {
			continue
		}
		for _, p := range prefixes {
			keys = append(keys, p+kind+id)
		}
	}
	return keys
}

// delWebAuthCacheKeys 는 주어진 키를 지우고 삭제된 개수를 돌려준다.
// Redis 미주입(nil)이거나 키가 없으면 0. 실패해도 로그만 남긴다.
func delWebAuthCacheKeys(mbID string, keys []string) int64 {
	if authCacheRedis == nil || len(keys) == 0 {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	n, err := authCacheRedis.Del(ctx, keys...).Result()
	if err != nil {
		log.Printf("[purgeAuthArtifacts] 캐시 무효화 실패 (%s): %v", mbID, err)
		return 0
	}
	return n
}

// clearWithdrawalGateCache 는 탈퇴 취소(복귀) 시 게이트·회원 캐시를 지운다.
//
// ⛔ 파기(purgeAuthArtifacts)의 반대 동작이다. 여기서 지우지 않으면
// 복귀한 회원이 캐시 TTL 동안 계속 차단된다 — 취소를 눌렀는데 아무것도 안 되는
// 상태라 즉시 민원이 된다.
//
// 회원 캐시(member:)까지 지우는 이유: web hooks 의 탈퇴자 세션 차단이
// getMemberById 결과를 보는데, 낡은 "탈퇴함" 을 읽으면 취소 직후 발급된
// 세션을 도로 파기해 버린다.
func clearWithdrawalGateCache(mbID string) {
	if authCacheRedis == nil || mbID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	keys := append(buildAuthCacheKeys("member:", []string{mbID}),
		middleware.WithdrawalGateKey(mbID))
	if err := authCacheRedis.Del(ctx, keys...).Err(); err != nil {
		log.Printf("[clearWithdrawalGateCache] 캐시 삭제 실패 (%s): %v", mbID, err)
	}
}
