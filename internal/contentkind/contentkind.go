// Package contentkind 는 글·댓글 본문의 "종류"(텍스트/이미지/이모티콘/동영상/링크/빈값)를 판정한다.
//
// 텍스트 없는 댓글이 전부 빈칸으로 뭉개져 "빈 댓글"로 오해받던 문제(#13095, #13097) 해소용.
// 판정은 적재(member_activity_sync)·조회(mypage_handler)·백필이 모두 같은 결과를 내야 하므로
// 한 곳(이 패키지)에 모은다 — 예전엔 핸들러에만 있고, 게다가 이미 태그를 벗긴 미리보기에 판정을
// 돌려 이미지·이모티콘만 있는 댓글이 활동피드에서 늘 "빈 댓글"로 표시됐다.
package contentkind

import (
	"regexp"
	"strings"
)

// 콘텐츠 종류 상수. 프론트 getCommentLabel(content-label.ts)의 KIND_LABEL 키와 1:1 대응한다.
const (
	Text     = "text"
	Emoticon = "emoticon"
	Image    = "image"
	Video    = "video"
	Link     = "link"
	Empty    = "empty"
)

var (
	htmlTagRe    = regexp.MustCompile(`<[^>]*>`)
	emoRe        = regexp.MustCompile(`\{emo:[^}]+\}`)
	whitespaceRe = regexp.MustCompile(`\s+`)

	// 다모앙 이모티콘은 두 형식이 공존한다(2026-07-26 전수 실측):
	//   ① {emo:파일명} 코드   ② <img src="/emoticons/...">
	// 첨부 이미지(/data/editor/)와는 경로로 구분한다.
	emoticonImgRe = regexp.MustCompile(`(?i)<img[^>]+src="[^"]*/emoticons/`)
	imgTagRe      = regexp.MustCompile(`(?i)<img\b`)
	videoTagRe    = regexp.MustCompile(`(?i)<(video|iframe)\b`)
	anchorTagRe   = regexp.MustCompile(`(?i)<a\b`)
)

// hasVisibleText 는 태그·이모티콘 코드·HTML 엔티티·공백을 지운 뒤 남는 글자가 있는지 본다.
// 점자공백(U+2800)은 \s 에 안 걸리는데 "빈댓글" 기능이 이 문자를 쓰므로 별도 제거한다(실측 1,791건).
func hasVisibleText(content string) bool {
	s := htmlTagRe.ReplaceAllString(content, "")
	s = emoRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "⠀", "")
	s = whitespaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s) != ""
}

// Classify 는 본문의 종류를 판정한다.
//
// ⛔ 반드시 **원본**(태그 포함) 문자열을 넘겨야 한다. 태그를 이미 벗긴 미리보기를 넘기면
// 이미지·이모티콘을 영영 구분할 수 없다(빈 문자열 → 무조건 Empty).
//
// 판정 순서(전수 실측 기반, 첫 매치 확정):
//
//	텍스트 → 이모티콘 → 이미지 → 동영상 → 링크 → 빈 값(최종 폴백)
//
// 마지막이 폴백이라 미분류가 남지 않는다.
func Classify(content string) string {
	if hasVisibleText(content) {
		return Text
	}
	switch {
	case emoRe.MatchString(content), emoticonImgRe.MatchString(content):
		return Emoticon
	case imgTagRe.MatchString(content):
		return Image
	case videoTagRe.MatchString(content):
		return Video
	case anchorTagRe.MatchString(content):
		return Link
	}
	return Empty
}
