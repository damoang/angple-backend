package worker

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
)

// keywordAlertReloadTTL 은 키워드 목록을 DB 에서 다시 읽는 주기다.
// 새로 등록한 키워드는 최대 이 시간 뒤부터 알림에 반영된다.
const keywordAlertReloadTTL = 60 * time.Second

// keywordAlertLoadTimeout 은 키워드 목록 한 번 적재에 쓰는 시간 상한이다.
const keywordAlertLoadTimeout = 5 * time.Second

// KeywordAlertRow 는 g5_member_keyword_alert 의 한 행(회원·키워드)이다.
type KeywordAlertRow struct {
	MbID    string `gorm:"column:mb_id"`
	Keyword string `gorm:"column:keyword"`
}

// KeywordAlertLoader 는 전체 키워드 알림 목록을 읽어 온다.
// 테스트에서 DB 없이 매처를 검증할 수 있도록 인터페이스로 둔다.
type KeywordAlertLoader interface {
	LoadKeywordAlerts(ctx context.Context) ([]KeywordAlertRow, error)
}

type dbKeywordAlertLoader struct {
	db *gorm.DB
}

// NewDBKeywordAlertLoader 는 g5_member_keyword_alert 를 읽는 로더를 만든다.
func NewDBKeywordAlertLoader(db *gorm.DB) KeywordAlertLoader {
	return &dbKeywordAlertLoader{db: db}
}

// LoadKeywordAlerts 는 전체 회원의 키워드를 읽는다.
func (l *dbKeywordAlertLoader) LoadKeywordAlerts(ctx context.Context) ([]KeywordAlertRow, error) {
	var rows []KeywordAlertRow
	err := l.db.WithContext(ctx).
		Table("g5_member_keyword_alert").
		Select("mb_id, keyword").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// KeywordAlertMatch 는 한 회원에게 걸린 키워드 하나다(회원당 최대 1건).
type KeywordAlertMatch struct {
	MbID    string
	Keyword string
}

type keywordAlertEntry struct {
	mbID       string
	keyword    string // 회원이 등록한 원문(알림 문구용)
	normalized string // NFC + 소문자
	runeLen    int
}

// KeywordAlertMatcher 는 전체 키워드 목록을 메모리(파드별)에 들고 새 글 제목과 대조한다.
//
// 목록은 keywordAlertReloadTTL 마다 다시 읽는다. 다시 읽기가 실패하면 직전 스냅샷을
// 그대로 쓴다 — DB 순간 장애로 키워드 알림이 통째로 멈추지 않게 한다.
// 실패한 경우에도 시도 시각을 갱신해, 장애 중에 글마다 DB 를 두드리지 않는다.
type KeywordAlertMatcher struct {
	loader KeywordAlertLoader
	ttl    time.Duration
	now    func() time.Time

	mu          sync.Mutex
	entries     []keywordAlertEntry
	loading     bool
	attempted   bool
	lastAttempt time.Time
}

// NewKeywordAlertMatcher 는 60초 주기로 목록을 다시 읽는 매처를 만든다.
func NewKeywordAlertMatcher(loader KeywordAlertLoader) *KeywordAlertMatcher {
	return &KeywordAlertMatcher{
		loader: loader,
		ttl:    keywordAlertReloadTTL,
		now:    time.Now,
	}
}

// normalizeKeywordText 는 키워드 차단과 같은 규칙(NFC + 소문자)으로 문자열을 맞춘다.
// macOS 등에서 들어온 자모 분리(NFD) 한글도 같은 글자로 비교된다.
func normalizeKeywordText(s string) string {
	return strings.ToLower(norm.NFC.String(s))
}

// buildKeywordAlertEntries 는 적재한 행을 정규화하고 긴 키워드가 먼저 오도록 정렬한다.
// 한 회원에게 여러 키워드가 걸리면 가장 긴 것 하나만 알림에 쓰기 위해서다.
func buildKeywordAlertEntries(rows []KeywordAlertRow) []keywordAlertEntry {
	entries := make([]keywordAlertEntry, 0, len(rows))
	for _, r := range rows {
		mbID := strings.TrimSpace(r.MbID)
		kw := strings.TrimSpace(r.Keyword)
		normalized := normalizeKeywordText(kw)
		if mbID == "" || normalized == "" {
			continue
		}
		entries = append(entries, keywordAlertEntry{
			mbID:       mbID,
			keyword:    kw,
			normalized: normalized,
			runeLen:    utf8.RuneCountInString(normalized),
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].runeLen != entries[j].runeLen {
			return entries[i].runeLen > entries[j].runeLen
		}
		if entries[i].normalized != entries[j].normalized {
			return entries[i].normalized < entries[j].normalized
		}
		return entries[i].mbID < entries[j].mbID
	})
	return entries
}

// snapshot 은 TTL 이 지났으면 목록을 다시 읽고, 현재 스냅샷을 돌려준다.
//
// DB 적재는 잠금 밖에서 한다 — 적재가 느려도 다른 워커 고루틴이 잠금에 묶이지 않는다.
// 적재는 한 번에 하나만 돌고, 그동안 다른 호출은 직전 스냅샷을 그대로 쓴다.
// 돌려준 슬라이스는 교체만 되고 수정되지 않으므로 잠금 밖에서 읽어도 안전하다.
func (m *KeywordAlertMatcher) snapshot(ctx context.Context) []keywordAlertEntry {
	m.mu.Lock()
	now := m.now()
	if m.loading || (m.attempted && now.Sub(m.lastAttempt) < m.ttl) {
		entries := m.entries
		m.mu.Unlock()
		return entries
	}
	m.loading = true
	m.attempted = true
	m.lastAttempt = now
	m.mu.Unlock()

	return m.reload(ctx)
}

// reload 는 잠금 밖에서 목록을 읽고, 성공하면 스냅샷을 바꾼다. 실패하면 직전 스냅샷을 유지한다.
// 로더가 패닉해도 loading 표식은 반드시 풀린다(풀리지 않으면 영영 재적재하지 않는다).
func (m *KeywordAlertMatcher) reload(ctx context.Context) (entries []keywordAlertEntry) {
	var built []keywordAlertEntry
	var loadErr error
	defer func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.loading = false
		if r := recover(); r != nil {
			loadErr = fmt.Errorf("loader panic: %v", r)
		}
		if loadErr != nil {
			log.Printf("[KeywordAlert] reload failed, keeping previous snapshot (%d entries): %v", len(m.entries), loadErr)
		} else {
			m.entries = built
		}
		entries = m.entries
	}()

	loadCtx, cancel := context.WithTimeout(ctx, keywordAlertLoadTimeout)
	defer cancel()
	rows, err := m.loader.LoadKeywordAlerts(loadCtx)
	if err != nil {
		loadErr = err
		return nil
	}
	built = buildKeywordAlertEntries(rows)
	return built
}

// Match 는 제목에 키워드가 들어간 회원 목록을 돌려준다(회원당 가장 긴 키워드 하나).
// 결과는 회원 ID 순으로 정렬돼 있다 — 수신자 상한으로 자를 때 결과가 흔들리지 않게.
func (m *KeywordAlertMatcher) Match(ctx context.Context, title string) []KeywordAlertMatch {
	if m == nil || m.loader == nil {
		return nil
	}
	entries := m.snapshot(ctx)
	if len(entries) == 0 {
		return nil
	}
	normalizedTitle := normalizeKeywordText(title)
	if normalizedTitle == "" {
		return nil
	}

	seen := make(map[string]bool)
	matches := make([]KeywordAlertMatch, 0, 4)
	for _, e := range entries {
		if seen[e.mbID] {
			continue
		}
		if strings.Contains(normalizedTitle, e.normalized) {
			seen[e.mbID] = true
			matches = append(matches, KeywordAlertMatch{MbID: e.mbID, Keyword: e.keyword})
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].MbID < matches[j].MbID })
	return matches
}
