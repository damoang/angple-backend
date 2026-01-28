package security

import (
	"html"
	"regexp"
	"strings"
)

// Sanitizer HTML 살균화 도구
type Sanitizer struct {
	// 허용할 HTML 태그 (기본: 없음)
	allowedTags map[string]bool
	// 허용할 속성
	allowedAttributes map[string][]string
}

// NewSanitizer 기본 Sanitizer 생성 (모든 HTML 제거)
func NewSanitizer() *Sanitizer {
	return &Sanitizer{
		allowedTags:       make(map[string]bool),
		allowedAttributes: make(map[string][]string),
	}
}

// NewRichTextSanitizer 리치 텍스트용 Sanitizer 생성 (기본 태그 허용)
func NewRichTextSanitizer() *Sanitizer {
	s := &Sanitizer{
		allowedTags: map[string]bool{
			"p": true, "br": true, "b": true, "i": true, "u": true,
			"strong": true, "em": true, "ul": true, "ol": true, "li": true,
			"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
			"blockquote": true, "pre": true, "code": true,
			"a": true, "img": true,
		},
		allowedAttributes: map[string][]string{
			"a":   {"href", "title", "target"},
			"img": {"src", "alt", "width", "height"},
		},
	}
	return s
}

// SanitizeString 문자열에서 XSS 공격 방지
func (s *Sanitizer) SanitizeString(input string) string {
	if input == "" {
		return ""
	}

	// HTML 엔티티 인코딩
	result := html.EscapeString(input)

	return result
}

// SanitizeHTML HTML에서 허용되지 않은 태그 제거
func (s *Sanitizer) SanitizeHTML(input string) string {
	if input == "" {
		return ""
	}

	// 허용된 태그가 없으면 모든 HTML 제거
	if len(s.allowedTags) == 0 {
		return s.stripAllHTML(input)
	}

	// 위험한 속성 제거 (이벤트 핸들러)
	result := s.removeEventHandlers(input)

	// javascript: 프로토콜 제거
	result = s.removeJavaScriptProtocol(result)

	// 허용되지 않은 태그 제거
	result = s.removeDisallowedTags(result)

	return result
}

// stripAllHTML 모든 HTML 태그 제거
func (s *Sanitizer) stripAllHTML(input string) string {
	// HTML 태그 정규식
	re := regexp.MustCompile(`<[^>]*>`)
	result := re.ReplaceAllString(input, "")

	// HTML 엔티티 디코딩 후 재인코딩 (이중 인코딩 방지)
	result = html.UnescapeString(result)
	result = html.EscapeString(result)

	return result
}

// removeEventHandlers 이벤트 핸들러 속성 제거
func (s *Sanitizer) removeEventHandlers(input string) string {
	// on* 이벤트 핸들러 제거
	re := regexp.MustCompile(`(?i)\s*on\w+\s*=\s*["'][^"']*["']`)
	return re.ReplaceAllString(input, "")
}

// removeJavaScriptProtocol javascript: 프로토콜 제거
func (s *Sanitizer) removeJavaScriptProtocol(input string) string {
	// javascript:, vbscript:, data: 등 위험한 프로토콜 제거
	re := regexp.MustCompile(`(?i)(javascript|vbscript|data)\s*:`)
	return re.ReplaceAllString(input, "blocked:")
}

// removeDisallowedTags 허용되지 않은 태그 제거
func (s *Sanitizer) removeDisallowedTags(input string) string {
	// 위험한 태그 완전 제거
	dangerousTags := []string{"script", "style", "iframe", "frame", "object", "embed", "applet", "form", "input", "button", "select", "textarea"}
	for _, tag := range dangerousTags {
		// 태그와 내용 모두 제거
		re := regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `>`)
		input = re.ReplaceAllString(input, "")
		// 자체 닫힘 태그 제거
		re = regexp.MustCompile(`(?i)<` + tag + `[^>]*/?>`)
		input = re.ReplaceAllString(input, "")
	}

	return input
}

// SanitizeURL URL 살균화 (javascript:, data: 등 차단)
func (s *Sanitizer) SanitizeURL(input string) string {
	if input == "" {
		return ""
	}

	input = strings.TrimSpace(input)
	lower := strings.ToLower(input)

	// 위험한 프로토콜 차단
	dangerousProtocols := []string{"javascript:", "vbscript:", "data:", "file:"}
	for _, proto := range dangerousProtocols {
		if strings.HasPrefix(lower, proto) {
			return ""
		}
	}

	// 허용된 프로토콜만 통과
	allowedProtocols := []string{"http://", "https://", "mailto:", "tel:", "/", "#"}
	hasProtocol := false
	for _, proto := range allowedProtocols {
		if strings.HasPrefix(lower, proto) {
			hasProtocol = true
			break
		}
	}

	// 상대 경로 허용
	if !hasProtocol && !strings.HasPrefix(input, "/") && !strings.HasPrefix(input, "#") {
		// 프로토콜 없는 URL은 상대 경로로 처리하거나 거부
		if strings.Contains(input, ":") {
			return "" // 알 수 없는 프로토콜
		}
	}

	return input
}

// SanitizeFilename 파일명 살균화
func (s *Sanitizer) SanitizeFilename(input string) string {
	if input == "" {
		return ""
	}

	// 경로 구분자 제거
	input = strings.ReplaceAll(input, "/", "")
	input = strings.ReplaceAll(input, "\\", "")
	input = strings.ReplaceAll(input, "..", "")

	// 널 바이트 제거
	input = strings.ReplaceAll(input, "\x00", "")

	// 특수문자 제거 (허용: 알파벳, 숫자, 한글, 하이픈, 언더스코어, 점)
	re := regexp.MustCompile(`[^a-zA-Z0-9가-힣\-_.]`)
	input = re.ReplaceAllString(input, "_")

	// 연속된 점 제거
	re = regexp.MustCompile(`\.+`)
	input = re.ReplaceAllString(input, ".")

	// 앞뒤 점/공백 제거
	input = strings.Trim(input, ". ")

	return input
}
