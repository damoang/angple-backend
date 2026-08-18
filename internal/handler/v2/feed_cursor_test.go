package v2

// feed_cursor_test.go — GET /api/v2/feed 의 커서 코덱·발췌 헬퍼 단위 테스트.
// 커서 = 보드slug→wr_id 워터마크 맵의 base64url(JSON). 깨진 커서는 빈 맵(=첫 페이지)으로
// 관대하게 처리한다(라이브 배포된 계약 — 400 으로 바꾸면 구버전 앱 커서가 깨질 수 있음).

import (
	"strings"
	"testing"
)

func TestFeedCursorRoundTrip(t *testing.T) {
	in := map[string]int{"free": 6712345, "qa": 98123, "new": 1}
	s := encodeFeedCursor(in)
	if s == "" {
		t.Fatal("encodeFeedCursor: 비어있지 않은 맵인데 빈 문자열")
	}
	// base64url(raw, no padding) — URL 쿼리에 그대로 실을 수 있어야 한다.
	if strings.ContainsAny(s, "+/=") {
		t.Errorf("encodeFeedCursor: URL-safe 하지 않은 문자 포함: %q", s)
	}
	out := decodeFeedCursor(s)
	if len(out) != len(in) {
		t.Fatalf("round trip 크기 불일치: got %d want %d", len(out), len(in))
	}
	for k, v := range in {
		if out[k] != v {
			t.Errorf("round trip %s: got %d want %d", k, out[k], v)
		}
	}
}

func TestFeedCursorEmpty(t *testing.T) {
	if s := encodeFeedCursor(nil); s != "" {
		t.Errorf("encodeFeedCursor(nil): got %q want \"\"", s)
	}
	if s := encodeFeedCursor(map[string]int{}); s != "" {
		t.Errorf("encodeFeedCursor(empty): got %q want \"\"", s)
	}
	if out := decodeFeedCursor(""); out == nil || len(out) != 0 {
		t.Errorf("decodeFeedCursor(\"\"): got %v want 빈 맵", out)
	}
}

func TestFeedCursorBrokenInputIsLenient(t *testing.T) {
	// 깨진 커서(잘못된 base64, JSON 아닌 페이로드, 타입 불일치)는 에러 없이 처리되고,
	// 양수 워터마크가 새어 나오면 안 된다. (리포는 wm > 0 일 때만 wr_id < wm 을 적용하므로
	// 0 값 잔존 엔트리는 무해 — 예: {"free":"abc"} 는 부분 언마샬로 free:0 이 남는다.)
	for _, s := range []string{
		"!!!not-base64!!!",
		"bm90LWpzb24",                   // "not-json"
		"eyJmcmVlIjoiYWJjIn0",           // {"free":"abc"} — 값 타입 불일치
		strings.Repeat("A", 10) + "%%%", // 쿼리 오염
	} {
		out := decodeFeedCursor(s)
		if out == nil {
			t.Fatalf("decodeFeedCursor(%q): nil 반환 — 맵이어야 함", s)
		}
		for k, v := range out {
			if v > 0 {
				t.Errorf("decodeFeedCursor(%q): 양수 워터마크 유출 %s=%d", s, k, v)
			}
		}
	}
}

func TestMakeExcerptStripsTagsAndEntities(t *testing.T) {
	in := `<p>안녕하세요 &amp; 반갑습니다.</p><div><img src="x.jpg">사진&nbsp;한 장</div>`
	got := makeExcerpt(in)
	want := "안녕하세요 & 반갑습니다. 사진 한 장"
	if got != want {
		t.Errorf("makeExcerpt: got %q want %q", got, want)
	}
	if strings.ContainsAny(got, "<>") {
		t.Errorf("makeExcerpt: 태그 잔존: %q", got)
	}
}

func TestMakeExcerptCollapsesWhitespace(t *testing.T) {
	got := makeExcerpt("  줄1\n\n줄2\t  줄3  ")
	if got != "줄1 줄2 줄3" {
		t.Errorf("makeExcerpt 공백 정리: got %q", got)
	}
}

func TestMakeExcerptTruncatesAt140Runes(t *testing.T) {
	// 멀티바이트(한글)에서도 바이트가 아닌 rune 기준 140자로 잘라야 한다.
	in := strings.Repeat("가", 200)
	got := makeExcerpt(in)
	r := []rune(got)
	if len(r) != 141 { // 140자 + 말줄임표
		t.Fatalf("makeExcerpt 길이: got %d runes want 141", len(r))
	}
	if r[len(r)-1] != '…' {
		t.Errorf("makeExcerpt: 말줄임표 누락, 끝 문자 %q", string(r[len(r)-1]))
	}
	// 정확히 140자면 자르지 않는다.
	exact := strings.Repeat("나", 140)
	if got := makeExcerpt(exact); got != exact {
		t.Errorf("makeExcerpt 140자 경계: 잘리면 안 됨, got %d runes", len([]rune(got)))
	}
}

func TestMakeExcerptEmpty(t *testing.T) {
	if got := makeExcerpt(""); got != "" {
		t.Errorf("makeExcerpt(\"\"): got %q", got)
	}
	if got := makeExcerpt("<br><p></p>"); got != "" {
		t.Errorf("makeExcerpt(태그만): got %q", got)
	}
}
