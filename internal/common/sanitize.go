package common

import (
	"regexp"

	"github.com/microcosm-cc/bluemonday"
)

// safeCSSProperties는 style 속성에서 허용하는 CSS 속성 목록
var safeCSSProperties = []string{
	"font-size", "font-weight", "font-style", "font-family",
	"color", "background-color",
	"text-align", "text-decoration", "line-height", "letter-spacing",
	"margin", "margin-top", "margin-bottom", "margin-left", "margin-right",
	"padding", "padding-top", "padding-bottom", "padding-left", "padding-right",
	"border", "border-radius",
	"width", "max-width", "height", "max-height",
}

// allowedIframeSrcRegex는 허용하는 iframe src 도메인 정규식
var allowedIframeSrcRegex = regexp.MustCompile(`^https://(www\.)?(` +
	`youtube\.com|youtube-nocookie\.com|` + // YouTube
	`platform\.twitter\.com|syndication\.twitter\.com|platform\.x\.com|` + // Twitter/X
	`embed\.bsky\.app|` + // Bluesky
	`instagram\.com|` + // Instagram
	`player\.vimeo\.com|` + // Vimeo
	`open\.spotify\.com|` + // Spotify
	`codepen\.io|` + // CodePen
	`tiktok\.com` + // TikTok
	`)/`)

// newPostPolicy는 게시글 본문용 bluemonday 정책을 생성한다.
func newPostPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	// 허용 태그 및 속성
	p.AllowElements("h1", "h2", "h3", "h4", "h5", "h6")
	p.AllowElements("p", "br", "hr", "div", "span")
	p.AllowElements("strong", "b", "em", "i", "u", "s", "del")
	p.AllowElements("ul", "ol", "li")
	p.AllowElements("blockquote", "code", "pre")
	p.AllowElements("table", "thead", "tbody", "tr")
	p.AllowElements("figure", "figcaption")
	p.AllowElements("details", "summary")
	p.AllowElements("a", "img", "iframe")
	p.AllowElements("video", "audio", "source")

	// th, td에 colspan, rowspan 허용
	p.AllowAttrs("colspan", "rowspan").OnElements("th", "td")

	// a 태그
	p.AllowAttrs("href", "title", "target", "rel").OnElements("a")
	p.AllowURLSchemes("https", "http", "mailto", "tel")

	// img 태그
	p.AllowAttrs("src", "alt", "title", "width", "height", "loading").OnElements("img")

	// iframe — 허용 도메인만 통과 (후처리에서 검증)
	p.AllowAttrs("src", "width", "height", "frameborder", "allow", "allowfullscreen",
		"referrerpolicy", "loading", "scrolling", "allowtransparency").OnElements("iframe")
	p.AllowAttrs("allowfullscreen").OnElements("iframe")

	// video/audio/source 속성
	p.AllowAttrs("src", "type", "controls", "autoplay", "muted", "loop",
		"playsinline", "preload", "width", "height").OnElements("video", "audio", "source")

	// 공통 속성
	p.AllowAttrs("class").Globally()
	p.AllowAttrs("alt", "title").Globally()

	// data 속성 (에디터/임베드용)
	p.AllowAttrs("data-youtube-video", "data-platform", "data-embed-height",
		"data-tweet-id", "data-bluesky-uri", "data-bluesky-cid").Globally()

	// details 태그의 open 속성
	p.AllowAttrs("open").OnElements("details")

	// start 속성 (ol)
	p.AllowAttrs("start").OnElements("ol")

	// style 속성 — 안전한 CSS만 허용
	p.AllowStyles(safeCSSProperties...).Globally()

	// target="_blank" 자동으로 rel 추가
	p.RequireNoFollowOnLinks(false)

	return p
}

// newCommentPolicy는 댓글용 bluemonday 정책을 생성한다.
func newCommentPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	p.AllowElements("p", "br", "span")
	p.AllowElements("strong", "b", "em", "i", "code", "s", "del")
	p.AllowElements("img")

	// a 태그 — nofollow noopener 강제
	p.AllowAttrs("href", "target", "rel").OnElements("a")
	p.AllowURLSchemes("https", "http", "mailto", "tel")
	p.RequireNoFollowOnLinks(true)
	p.RequireNoFollowOnFullyQualifiedLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)

	// 댓글 이미지 — 업로드된 첨부 이미지만 안전하게 렌더링
	p.AllowAttrs("src", "alt", "title", "width", "height", "loading").OnElements("img")

	return p
}

// 싱글톤 정책 (재사용)
var (
	postPolicy    = newPostPolicy()
	commentPolicy = newCommentPolicy()
	strictPolicy  = bluemonday.StrictPolicy()
)

// SanitizePostContent는 게시글 본문 HTML을 정제한다.
// 안전한 태그/속성만 허용하고, iframe은 허용 도메인만 통과시킨다.
func SanitizePostContent(html string) string {
	sanitized := postPolicy.Sanitize(html)
	// iframe src가 허용 도메인이 아닌 경우 제거
	sanitized = removeDisallowedIframes(sanitized)
	return sanitized
}

// SanitizeComment는 댓글 HTML을 정제한다.
// 기본 서식만 허용하고, 모든 링크에 rel="nofollow noopener"를 강제한다.
func SanitizeComment(html string) string {
	return commentPolicy.Sanitize(html)
}

// SanitizeMessage는 메시지 텍스트에서 모든 HTML을 제거하고 텍스트만 반환한다.
func SanitizeMessage(text string) string {
	return strictPolicy.Sanitize(text)
}

// iframeAnyRegex는 src 유무와 무관하게 모든 iframe 태그를 매칭한다.
var iframeAnyRegex = regexp.MustCompile(`(?i)<iframe\b[^>]*>[\s\S]*?</iframe>`)

// iframeSrcAttrRegex는 iframe의 src 속성값을 추출한다(없으면 미매칭).
var iframeSrcAttrRegex = regexp.MustCompile(`(?i)\bsrc\s*=\s*"([^"]*)"`)

// emptyYoutubeWrapperRegex는 iframe이 제거돼 빈 껍데기만 남은
// tiptap youtube 래퍼(<div data-youtube-video>)를 매칭한다.
var emptyYoutubeWrapperRegex = regexp.MustCompile(`(?i)<div\b[^>]*\bdata-youtube-video\b[^>]*>\s*</div>`)

// removeDisallowedIframes는 허용 도메인 src를 가진 iframe만 남기고 나머지를 제거한다.
// #12686: 기존에는 src="..."가 있어야만 매칭돼, src가 누락된 깨진 iframe
// (예: tiptap youtube 노드가 src=null로 직렬화된 경우)이 무방비로 통과·저장됐다.
// 그 깨진 노드는 수정 진입 시 에디터 본문을 비우는 원인이 된다. 이제 src가 없거나
// 허용 도메인이 아닌 iframe은 모두 제거하고, 비게 된 youtube 래퍼도 정리한다.
func removeDisallowedIframes(html string) string {
	html = iframeAnyRegex.ReplaceAllStringFunc(html, func(match string) string {
		m := iframeSrcAttrRegex.FindStringSubmatch(match)
		if len(m) < 2 || !allowedIframeSrcRegex.MatchString(m[1]) {
			return "" // src 없음 또는 비허용 도메인 → 제거
		}
		return match
	})
	return emptyYoutubeWrapperRegex.ReplaceAllString(html, "")
}
