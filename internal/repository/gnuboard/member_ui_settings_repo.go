package gnuboard

import (
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
)

// 회원 UI 설정 읽기 (bug/13664 쪽지 수신 거부).
//
// ⛔ 이 테이블의 **소유자는 web 이다.**
//
//	`web/apps/web/src/lib/server/ui-settings-store.ts` 가 MySQL 을 직접 read/write 하고
//	`/api/my/ui-settings` 로 노출한다. Redis 캐시(`uis:{mbId}`, TTL 300s)도 web 이 관리한다.
//	**backend 는 읽기만 한다. 절대 쓰지 않는다** — 쓰면 web 의 캐시와 어긋난다.
//
// ⭐ 왜 컬럼을 새로 안 만들었나
//
//	`g5_member` 에 `mb_memo_deny` 를 추가하자는 안이 먼저 있었는데,
//	이 테이블의 `settings` 가 **JSON** 이라 키만 늘리면 된다.
//	DDL 0 · 마이그레이션 0 · 롤백은 키 제거. 17,720명이 이미 쓰고 있다.
//
// ⚠️ 키 이름이 `memoDeny` 가 아니라 **`messageDeny`** 인 이유
//
//	같은 JSON 안에 `blurMemo`·`hideMemoInList`·`memoColorLabels` 가 있는데
//	그것들은 **메모 플러그인(작성자 주석 배지)** 이지 쪽지(DM)가 아니다.
//	`memoDeny` 로 두면 그 옆에서 「메모 거부」로 오독된다.
//	`message` 는 backend 용어(`message_handler`·`/api/v1/messages`)와도 일치한다.

const (
	messageDenyCacheTTL = 60 * time.Second
	// ⛔ 상한을 둔다. 캐시가 무한히 자라면 그게 다음 사고다.
	messageDenyCacheMax = 20000
)

var messageDenyCache struct {
	sync.RWMutex
	deny    map[string]bool
	expires map[string]time.Time
}

// MemberUISettingsRepository 는 web 소유의 회원 UI 설정을 **읽기 전용**으로 본다.
type MemberUISettingsRepository interface {
	// IsMessageDenied 는 그 회원이 쪽지 수신을 꺼두었는지 돌려준다.
	//
	// ⛔ error 는 「거부 아님」이 아니라 **「모른다」** 다. 호출부가 갈라서 다뤄야 한다.
	IsMessageDenied(mbID string) (bool, error)
}

type memberUISettingsRepository struct{ db *gorm.DB }

// NewMemberUISettingsRepository creates a read-only reader for web-owned UI settings.
func NewMemberUISettingsRepository(db *gorm.DB) MemberUISettingsRepository {
	return &memberUISettingsRepository{db: db}
}

func (r *memberUISettingsRepository) IsMessageDenied(mbID string) (bool, error) {
	if mbID == "" {
		return false, nil
	}

	messageDenyCache.RLock()
	if exp, ok := messageDenyCache.expires[mbID]; ok && time.Now().Before(exp) {
		v := messageDenyCache.deny[mbID]
		messageDenyCache.RUnlock()
		return v, nil
	}
	messageDenyCache.RUnlock()

	// ⭐ JSON_UNQUOTE(JSON_EXTRACT(...)) = `->>`. 값이 없으면 NULL 이 온다.
	//    행 자체가 없는 회원(설정을 한 번도 안 건드린 사람)이 대다수라 Raw 로 스캔한다.
	var raw *string
	err := r.db.Raw(
		"SELECT settings->>'$.messageDeny' FROM g5_da_member_ui_settings WHERE mb_id = ? LIMIT 1",
		mbID,
	).Scan(&raw).Error
	if err != nil {
		return false, fmt.Errorf("쪽지 수신 설정 조회 실패 (mb_id=%s): %w", mbID, err)
	}

	// 없음 = 기본값(수신 허용). JSON 은 true/false 를 문자열로 돌려준다.
	denied := raw != nil && (*raw == "true" || *raw == "1")

	messageDenyCache.Lock()
	if messageDenyCache.deny == nil {
		messageDenyCache.deny = make(map[string]bool)
		messageDenyCache.expires = make(map[string]time.Time)
	}
	// ⛔ 만료분을 쓰기 시점에 함께 치운다(별도 타이머 불필요).
	if len(messageDenyCache.deny) > messageDenyCacheMax {
		now := time.Now()
		for k, exp := range messageDenyCache.expires {
			if now.After(exp) {
				delete(messageDenyCache.deny, k)
				delete(messageDenyCache.expires, k)
			}
		}
	}
	messageDenyCache.deny[mbID] = denied
	messageDenyCache.expires[mbID] = time.Now().Add(messageDenyCacheTTL)
	messageDenyCache.Unlock()

	return denied, nil
}

// ResetMessageDenyCacheForTest 는 테스트에서 캐시 누수를 막는다.
func ResetMessageDenyCacheForTest() {
	messageDenyCache.Lock()
	messageDenyCache.deny = nil
	messageDenyCache.expires = nil
	messageDenyCache.Unlock()
}

// LogMessageDenyLookupFailure 는 조회 실패를 한 형식으로 남긴다.
// ⭐ 실패해도 쪽지는 보낸다(fail-open) — 이유는 호출부 주석 참조.
func LogMessageDenyLookupFailure(receiverID string, err error) {
	log.Printf("[message] 수신 거부 설정 조회 실패 receiver=%s: %v — 기존대로 발송한다", receiverID, err)
}
