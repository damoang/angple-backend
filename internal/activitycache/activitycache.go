// Package activitycache 는 회원 최근 활동 응답(작성자 활동 패널·회원 프로필) 캐시의
// 키·TTL·무효화를 한곳에 모은다.
//
// 같은 응답을 두 곳이 캐시한다 — 둘 다 같은 Redis 에, 접두사 없이 둔다.
//
//	백엔드 GetMemberActivity : member_activity:{mb_id}:{limit}   (limit 1~50)
//	web fetchMemberActivity  : member_activity:{mb_id}:l{limit}  (글 상세 SSR, limit 5)
//
// 무효화는 SCAN MATCH 대신 키 목록을 만들어 DEL 한다. 회원별 패턴이라도 SCAN 은
// 전체 키공간을 훑어야 해서 쓰기마다 부르기엔 비싸다. 없는 키 DEL 은 해시 조회뿐이다.
//
// handler·service·worker·cron 모두에서 부르므로 별도 패키지로 둔다(webcache 와 같은 이유).
package activitycache

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// TTL 은 활동 응답 캐시의 수명이다.
//
// 쓰기 경로(작성·수정·삭제·복구·잠금·이용제한·비공개 전환)에서 키를 지우므로,
// TTL 은 「무효화가 빠진 경로가 있을 때 최대로 늦게 반영되는 시간」이다.
const TTL = 5 * time.Minute

// MaxLimit 은 GetMemberActivity 가 허용하는 limit 상한이다. 키 목록도 1~MaxLimit 를 덮는다.
const MaxLimit = 50

// webLimits 는 web 이 자기 캐시 키(`:l{limit}`)에 쓰는 limit 값이다.
// ⛔ web fetchMemberActivity 를 다른 limit 로 부르기 시작하면 여기도 추가할 것.
var webLimits = []int{5}

// invalidateTimeout 은 DEL 한 번에 허용하는 최대 시간이다. 시간 안에 못 지우면 TTL 로 만료된다.
const invalidateTimeout = 500 * time.Millisecond

// delBatchKeys 는 DEL 한 번에 보낼 최대 키 수다. 댓글이 많은 글은 회원 수 × 51 키가 되므로 나눠 보낸다.
const delBatchKeys = 1000

// DeleteFunc 는 주어진 키들을 지우는 함수다. Redis Del 과 같은 의미를 갖는다.
type DeleteFunc func(ctx context.Context, keys ...string) error

var (
	mu      sync.RWMutex
	deleter DeleteFunc
)

// SetRedis 는 무효화에 쓸 Redis 클라이언트를 주입한다.
// nil 이면 무효화를 건너뛴다(캐시는 TTL 로 자연 만료된다).
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

// Key 는 백엔드 GetMemberActivity 의 캐시 키다.
func Key(mbID string, limit int) string {
	return fmt.Sprintf("member_activity:%s:%d", mbID, limit)
}

// Keys 는 한 회원의 활동 캐시 키 전부(백엔드 limit 1~MaxLimit + web 키)를 돌려준다.
// mbID 가 비어 있으면 nil.
func Keys(mbID string) []string {
	if mbID == "" {
		return nil
	}
	keys := make([]string, 0, MaxLimit+len(webLimits))
	for limit := 1; limit <= MaxLimit; limit++ {
		keys = append(keys, Key(mbID, limit))
	}
	for _, limit := range webLimits {
		keys = append(keys, fmt.Sprintf("member_activity:%s:l%d", mbID, limit))
	}
	return keys
}

// InvalidateMembers 는 회원들의 활동 캐시 키를 지운다.
//
// 반드시 **쓰기가 커밋된 뒤** 부른다(커밋 전에 지우면 그 사이 낡은 값이 다시 적재된다).
// 실패해도 에러를 돌려주지 않는다 — 쓰기는 이미 끝났고 캐시는 TTL 로 만료되므로 로그만 남긴다.
// Redis 가 주입되지 않았으면 아무것도 하지 않는다.
func InvalidateMembers(mbIDs ...string) {
	del := currentDeleter()
	if del == nil {
		return
	}
	seen := make(map[string]struct{}, len(mbIDs))
	keys := make([]string, 0, len(mbIDs)*(MaxLimit+len(webLimits)))
	for _, id := range mbIDs {
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		keys = append(keys, Keys(id)...)
	}
	for start := 0; start < len(keys); start += delBatchKeys {
		end := min(start+delBatchKeys, len(keys))
		ctx, cancel := context.WithTimeout(context.Background(), invalidateTimeout)
		err := del(ctx, keys[start:end]...)
		cancel()
		if err != nil {
			log.Printf("[activitycache] 활동 캐시 삭제 실패 (회원 %d명): %v", len(seen), err)
			return
		}
	}
}

// InvalidateMembersAfter 는 delay 뒤에 한 번 더 InvalidateMembers 를 부른다.
//
// 응답 재료 중 파드 메모리에 따로 캐시되는 것(이용제한 근거 목록, 5분)이 있다. 그 캐시가
// 낡은 동안 다시 적재된 응답을 걷어내려면, 메모리 캐시가 만료된 뒤 한 번 더 지워야 한다.
// 파드가 그 사이 재시작되면 이 예약은 사라진다 — 그때는 TTL 로 만료된다.
func InvalidateMembersAfter(delay time.Duration, mbIDs ...string) {
	if currentDeleter() == nil {
		return
	}
	ids := append([]string(nil), mbIDs...)
	time.AfterFunc(delay, func() { InvalidateMembers(ids...) })
}

var boardSlugPattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// MembersForWrite 는 글·댓글 하나가 바뀌었을 때 활동 응답이 달라지는 회원 목록을 돌려준다.
//
//   - 댓글: 그 댓글 작성자.
//   - 글: 글 작성자 + 그 글에 댓글을 단 회원. 댓글 항목도 부모 글의 잠금·삭제·비밀
//     상태에 따라 가려지므로(예: [신고잠금 글의 댓글]) 함께 지워야 한다.
//
// 조회 실패는 찾은 데까지만 돌려준다(무효화는 최선 노력이다).
func MembersForWrite(db *gorm.DB, boardSlug string, wrID int) []string {
	if db == nil || wrID <= 0 || !boardSlugPattern.MatchString(boardSlug) {
		return nil
	}
	table := "g5_write_" + boardSlug
	var row struct {
		MbID      string `gorm:"column:mb_id"`
		IsComment int    `gorm:"column:wr_is_comment"`
	}
	if err := db.Table(table).Select("COALESCE(mb_id, '') AS mb_id, wr_is_comment").
		Where("wr_id = ?", wrID).Limit(1).Scan(&row).Error; err != nil {
		log.Printf("[activitycache] 작성자 조회 실패 (%s/%d): %v", boardSlug, wrID, err)
		return nil
	}
	members := []string{}
	if row.MbID != "" {
		members = append(members, row.MbID)
	}
	if row.IsComment != 0 {
		return members
	}
	var commenters []string
	if err := db.Table("member_activity_feed").
		Distinct("member_id").
		Where("write_table = ? AND parent_write_id = ? AND activity_type = 2", table, wrID).
		Pluck("member_id", &commenters).Error; err != nil {
		log.Printf("[activitycache] 댓글 작성자 조회 실패 (%s/%d): %v", boardSlug, wrID, err)
		return members
	}
	return append(members, commenters...)
}

// InvalidateWrite 는 글·댓글 하나가 바뀐 뒤(커밋 뒤) 영향받는 회원들의 활동 캐시를 지운다.
// 대상은 MembersForWrite 와 같다.
func InvalidateWrite(db *gorm.DB, boardSlug string, wrID int) {
	if currentDeleter() == nil {
		return
	}
	InvalidateMembers(MembersForWrite(db, boardSlug, wrID)...)
}
