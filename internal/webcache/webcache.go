// Package webcache 는 web(SvelteKit) 이 Redis 에 들고 있는 캐시 키를
// 백엔드에서 지우기 위한 공용 도구다.
//
// web 의 TieredCache 는 CACHE_NAMESPACE 가 있으면 `{ns}:{prefix}:{key}`,
// 없으면 `{prefix}:{key}` 로 키를 만든다(web cache.ts 의 redisKey()).
// 백엔드는 그 규칙을 그대로 따라 키를 조립해 DEL 한다.
//
// handler·service 양쪽에서 쓰기 위해 별도 패키지로 둔다(handler 는 service 를
// import 하므로 service 가 handler 의 함수를 부를 수 없다).
package webcache

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// defaultNamespaces 는 AUTH_CACHE_NAMESPACES 미설정 시 **추가로** 커버할
// 네임스페이스다(접두사 없음은 항상 포함되므로 여기 적지 않는다).
const defaultNamespaces = "canary"

// memberCardKind 는 회원 표시 정보(닉네임·사진·사진 시각·탈퇴 여부) 캐시의
// web TieredCache prefix + 콜론이다.
const memberCardKind = "mbcard:"

// invalidateTimeout 은 DEL 한 번에 허용하는 최대 시간이다.
// 무효화는 쓰기 요청 안에서 동기로 돌므로, Redis 가 느려도 요청 지연이
// 이 값 이상 늘지 않게 짧게 둔다. 시간 안에 못 지우면 캐시 TTL 로 자연 만료된다.
const invalidateTimeout = 500 * time.Millisecond

// DeleteFunc 는 주어진 키들을 지우는 함수다. Redis Del 과 같은 의미를 갖는다.
type DeleteFunc func(ctx context.Context, keys ...string) error

var (
	mu      sync.RWMutex
	deleter DeleteFunc
)

// SetRedis 는 무효화에 쓸 Redis 클라이언트를 주입한다.
// nil 이면 무효화를 건너뛴다(캐시는 TTL 로 자연 만료된다).
// 라우팅 개시 전에 부를 것.
func SetRedis(c *redis.Client) {
	if c == nil {
		setDeleter(nil)
		return
	}
	setDeleter(func(ctx context.Context, keys ...string) error {
		return c.Del(ctx, keys...).Err()
	})
}

// SetDeleterForTest 는 삭제 백엔드를 바꿔 끼우고, 원래대로 돌리는 함수를 돌려준다.
// 테스트 전용이다 — 운영 코드는 SetRedis 를 쓴다.
func SetDeleterForTest(fn DeleteFunc) (restore func()) {
	mu.Lock()
	prev := deleter
	deleter = fn
	mu.Unlock()
	return func() { setDeleter(prev) }
}

func setDeleter(fn DeleteFunc) {
	mu.Lock()
	deleter = fn
	mu.Unlock()
}

func currentDeleter() DeleteFunc {
	mu.RLock()
	defer mu.RUnlock()
	return deleter
}

// NamespacePrefixes 는 무효화 대상 web 캐시 네임스페이스의 키 접두사 목록을 돌려준다.
//
// ⛔ 백엔드 자신의 CACHE_NAMESPACE 를 쓰면 안 된다. 지워야 하는 것은 web 의 키인데,
// web 과 백엔드의 네임스페이스는 서로 다를 수 있다. 그래서 목록을 env 로 받는다 —
// AUTH_CACHE_NAMESPACES, 콤마 구분.
//
//	AUTH_CACHE_NAMESPACES="canary"        →  ["", "canary:"]
//	AUTH_CACHE_NAMESPACES="canary,blue"   →  ["", "canary:", "blue:"]
//	(미설정)                               →  ["", "canary:"]
//
// ⛔ env 는 "추가"이지 "치환"이 아니다. 접두사 없음("")은 항상 첫 번째로 포함한다.
// 치환식이면 네임스페이스 하나를 추가하려다 접두사 없는 키 커버가 조용히 빠진다
// (Del 은 없는 키에도 에러를 내지 않는다).
//
// ⛔ web 에 새 CACHE_NAMESPACE 를 추가하면 이 env 도 함께 갱신할 것.
func NamespacePrefixes() []string {
	prefixes := []string{""}
	seen := map[string]struct{}{"": {}}

	raw := os.Getenv("AUTH_CACHE_NAMESPACES")
	if raw == "" {
		raw = defaultNamespaces
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

// MemberCardCacheKeys 는 회원 표시 정보 캐시 키 목록을 돌려준다.
//
// 키 규약(web 과 공유): `mbcard:{lower(mb_id)}`, 네임스페이스가 있으면
// `{ns}:mbcard:{lower(mb_id)}`. 네임스페이스 목록은 NamespacePrefixes 와 같다.
// mbID 가 비어 있으면 nil.
func MemberCardCacheKeys(mbID string) []string {
	if mbID == "" {
		return nil
	}
	id := strings.ToLower(mbID)
	prefixes := NamespacePrefixes()
	keys := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		keys = append(keys, p+memberCardKind+id)
	}
	return keys
}

// InvalidateMemberCard 는 회원 표시 정보(닉네임·사진·사진 시각·탈퇴 여부)가 바뀐 뒤
// 해당 회원의 web 표시 캐시 키를 지운다.
//
// 반드시 **쓰기가 성공한 뒤** 부른다. 실패해도 에러를 돌려주지 않는다 — 쓰기는
// 이미 끝났고, 캐시는 TTL 로 자연 만료되므로 로그만 남긴다.
// Redis 가 주입되지 않았으면 아무것도 하지 않는다.
// 트랜잭션 안에서 부르지 말 것(커밋 전에 지우면 그 사이 낡은 값이 다시 적재될 수 있다).
func InvalidateMemberCard(mbID string) {
	del := currentDeleter()
	if del == nil {
		return
	}
	keys := MemberCardCacheKeys(mbID)
	if len(keys) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), invalidateTimeout)
	defer cancel()
	if err := del(ctx, keys...); err != nil {
		log.Printf("[webcache] 회원 표시 캐시 삭제 실패 (%s): %v", mbID, err)
	}
}
