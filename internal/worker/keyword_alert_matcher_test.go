package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"
)

type fakeKeywordLoader struct {
	rows  []KeywordAlertRow
	err   error
	calls int
}

func (f *fakeKeywordLoader) LoadKeywordAlerts(_ context.Context) ([]KeywordAlertRow, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

func matchMap(matches []KeywordAlertMatch) map[string]string {
	out := make(map[string]string, len(matches))
	for _, m := range matches {
		out[m.MbID] = m.Keyword
	}
	return out
}

func TestKeywordAlertMatcherMatch(t *testing.T) {
	loader := &fakeKeywordLoader{rows: []KeywordAlertRow{
		{MbID: "upper", Keyword: "IPHONE"},
		{MbID: "partial", Keyword: "폰 17"},
		{MbID: "multi", Keyword: "아이폰"},
		{MbID: "multi", Keyword: "아이폰 17"},
		{MbID: "nomatch", Keyword: "갤럭시"},
		{MbID: "blank", Keyword: "   "},
	}}
	m := NewKeywordAlertMatcher(loader)

	got := matchMap(m.Match(context.Background(), "새 iPhone 소식 — 아이폰 17 출시"))

	if got["upper"] != "IPHONE" {
		t.Errorf("대소문자 무시 일치 실패: %v", got)
	}
	if got["partial"] != "폰 17" {
		t.Errorf("부분 일치 실패: %v", got)
	}
	if got["multi"] != "아이폰 17" {
		t.Errorf("여러 키워드 일치 시 가장 긴 키워드 하나여야 한다: got %q", got["multi"])
	}
	if _, ok := got["nomatch"]; ok {
		t.Errorf("일치하지 않는 키워드가 걸렸다: %v", got)
	}
	if _, ok := got["blank"]; ok {
		t.Errorf("빈 키워드가 걸렸다: %v", got)
	}
	if len(got) != 3 {
		t.Errorf("회원당 1건이어야 한다: got %d entries %v", len(got), got)
	}
}

func TestKeywordAlertMatcherNFC(t *testing.T) {
	decomposedTitle := norm.NFD.String("아이폰 출시")
	if decomposedTitle == "아이폰 출시" {
		t.Fatal("테스트 전제 실패: NFD 문자열이 NFC 와 같다")
	}
	loader := &fakeKeywordLoader{rows: []KeywordAlertRow{
		{MbID: "composed", Keyword: "아이폰"},
		{MbID: "decomposed", Keyword: norm.NFD.String("출시")},
	}}
	m := NewKeywordAlertMatcher(loader)

	got := matchMap(m.Match(context.Background(), decomposedTitle))
	if _, ok := got["composed"]; !ok {
		t.Errorf("NFD 제목 × NFC 키워드가 일치해야 한다: %v", got)
	}
	if _, ok := got["decomposed"]; !ok {
		t.Errorf("NFC 제목 × NFD 키워드가 일치해야 한다: %v", got)
	}
}

func TestKeywordAlertMatcherReloadTTLAndFailure(t *testing.T) {
	loader := &fakeKeywordLoader{rows: []KeywordAlertRow{{MbID: "user1", Keyword: "키보드"}}}
	m := NewKeywordAlertMatcher(loader)
	now := time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	ctx := context.Background()

	if got := m.Match(ctx, "기계식 키보드 추천"); len(got) != 1 {
		t.Fatalf("첫 적재 후 일치해야 한다: %v", got)
	}
	if loader.calls != 1 {
		t.Fatalf("첫 호출에 1회 적재: got %d", loader.calls)
	}

	// TTL 안에서는 다시 읽지 않는다.
	now = now.Add(59 * time.Second)
	m.Match(ctx, "기계식 키보드 추천")
	if loader.calls != 1 {
		t.Fatalf("TTL 안에서는 재적재하지 않아야 한다: got %d", loader.calls)
	}

	// TTL 이 지나 재적재했는데 실패하면 직전 스냅샷을 유지한다.
	loader.err = errors.New("db down")
	now = now.Add(2 * time.Second)
	if got := m.Match(ctx, "기계식 키보드 추천"); len(got) != 1 || got[0].MbID != "user1" {
		t.Fatalf("재적재 실패 시 이전 스냅샷을 써야 한다: %v", got)
	}
	if loader.calls != 2 {
		t.Fatalf("TTL 경과 후 재적재 시도: got %d", loader.calls)
	}

	// 실패 직후에도 TTL 동안은 다시 시도하지 않는다(장애 중 글마다 DB 를 두드리지 않음).
	now = now.Add(10 * time.Second)
	m.Match(ctx, "기계식 키보드 추천")
	if loader.calls != 2 {
		t.Fatalf("실패 후 TTL 안에서는 재시도하지 않아야 한다: got %d", loader.calls)
	}

	// 복구되면 새 목록으로 바뀐다.
	loader.err = nil
	loader.rows = []KeywordAlertRow{{MbID: "user2", Keyword: "마우스"}}
	now = now.Add(61 * time.Second)
	if got := m.Match(ctx, "기계식 키보드 추천"); len(got) != 0 {
		t.Fatalf("새 목록으로 교체돼야 한다: %v", got)
	}
	if got := m.Match(ctx, "무선 마우스"); len(got) != 1 || got[0].MbID != "user2" {
		t.Fatalf("새 목록 일치 실패: %v", got)
	}
}

func TestKeywordAlertMatcherFirstLoadFailure(t *testing.T) {
	loader := &fakeKeywordLoader{err: errors.New("table missing")}
	m := NewKeywordAlertMatcher(loader)
	if got := m.Match(context.Background(), "아무 제목"); len(got) != 0 {
		t.Fatalf("첫 적재 실패 시 아무에게도 보내지 않는다: %v", got)
	}
}
