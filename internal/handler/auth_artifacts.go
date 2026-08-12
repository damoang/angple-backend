package handler

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// authCacheRedis 는 web(SvelteKit) 이 쓰는 세션·회원 캐시를 무효화하기 위한 Redis 핸들이다.
// main 에서 SetAuthCacheRedis 로 주입한다. nil 이면 캐시 무효화를 건너뛴다(파기 자체는 진행).
var authCacheRedis *redis.Client

// SetAuthCacheRedis 는 인증 캐시 무효화용 Redis 클라이언트를 주입한다.
func SetAuthCacheRedis(c *redis.Client) { authCacheRedis = c }

// purgeAuthArtifacts 는 회원의 인증 산출물(세션·리프레시 토큰)을 파기하고,
// web 이 들고 있는 세션·회원 캐시까지 무효화한다.
//
// 왜 — 분쟁조정위 26R05-00197 로 드러난 문제:
//
//	탈퇴 처리가 기존 세션을 무효화하지 않아, 탈퇴 전에 발급된 세션 쿠키로
//	인증 상태가 유지됐다. 신청인 계정에서 탈퇴 17일 뒤 포인트·XP·mb_today_login
//	이 갱신된 것이 그 결과다(신규 로그인이 아니라 잔존 세션에 의한 접근).
//
// ⛔ revoke 가 아니라 DELETE 다.
//
//	angple_sessions·angple_refresh_tokens 는 IP·User-Agent 를 보유해
//	사실상 접속기록이다. 행을 남기면 개인정보가 그대로 보존된다.
//
// ⛔ **DB 만 지우면 부족하다.** web 의 TieredCache 가 L2(Redis, TTL 300초)에
//
//	세션(sess:<hash>)과 회원(member:<mb_id>)을 들고 있어, DB 행이 사라져도
//	최대 5분간 캐시로 인증이 통과한다. 그래서 여기서 Redis 키까지 지운다.
//	세션 해시는 DELETE 전에 읽어 둬야 한다 — 지운 뒤에는 알 수 없다.
//
// ⛔ 실패해도 에러를 반환하지 않는다.
//
//	회원 UPDATE 가 끝난 뒤 호출되며, 파기 실패로 탈퇴가 되돌아가면 안 된다.
//	회원은 이미 탈퇴를 신청했고, 실패하면 더 곤란해진다. 로그만 남긴다.
//
// ⛔ 트랜잭션에 묶어 호출하지 말 것. 위 원칙이 깨진다.
// ⛔ 탈퇴 **취소** 경로에서는 부르지 않는다 — 복귀시키는 동작이다.
//
// 호출처: applySelfLeave(자가 탈퇴) · AdminMemberHandler.UpdateMember(관리자 탈퇴).
// 새 탈퇴 진입점을 만들면 여기도 함께 배선할 것.
func purgeAuthArtifacts(db *gorm.DB, mbID string) {
	if mbID == "" {
		return
	}

	// 캐시 키 확보 — DELETE 전에 해야 한다.
	var hashes []string
	if err := db.Raw("SELECT session_id_hash FROM angple_sessions WHERE mb_id = ?", mbID).
		Scan(&hashes).Error; err != nil {
		log.Printf("[purgeAuthArtifacts] 세션 해시 조회 실패 (%s): %v", mbID, err)
	}

	if err := db.Exec("DELETE FROM angple_sessions WHERE mb_id = ?", mbID).Error; err != nil {
		log.Printf("[purgeAuthArtifacts] 세션 파기 실패 (%s): %v", mbID, err)
	}
	if err := db.Exec("DELETE FROM angple_refresh_tokens WHERE mb_id = ?", mbID).Error; err != nil {
		log.Printf("[purgeAuthArtifacts] 리프레시 토큰 파기 실패 (%s): %v", mbID, err)
	}

	invalidateWebAuthCache(mbID, hashes)
}

// invalidateWebAuthCache 는 web TieredCache 의 L2 키를 지운다.
//
// 키 규칙은 web cache.ts 의 redisKey() 와 같아야 한다 —
// CACHE_NAMESPACE 가 있으면 `{ns}:{prefix}:{key}`, 없으면 `{prefix}:{key}`.
// ⚠️ web 의 L1(프로세스 메모리, 60초)까지는 지울 수 없다. 그쪽은 web 이
// 같은 탈퇴 흐름에서 purgeAuthArtifacts 를 한 번 더 부르며 정리한다.
func invalidateWebAuthCache(mbID string, sessionHashes []string) {
	if authCacheRedis == nil {
		return
	}
	prefix := ""
	if ns := os.Getenv("CACHE_NAMESPACE"); ns != "" {
		prefix = ns + ":"
	}

	keys := make([]string, 0, len(sessionHashes)+1)
	for _, h := range sessionHashes {
		if h != "" {
			keys = append(keys, prefix+"sess:"+h)
		}
	}
	keys = append(keys, prefix+"member:"+mbID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := authCacheRedis.Del(ctx, keys...).Err(); err != nil {
		log.Printf("[purgeAuthArtifacts] 캐시 무효화 실패 (%s): %v", mbID, err)
	}
}
