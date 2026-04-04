package v2

import "regexp"

var wikiLinkPattern = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// WikiLinkRef 파싱된 위키 링크 참조
type WikiLinkRef struct {
	Text string // [[문서명]]에서 "문서명" 부분
}

// ParseWikiLinks HTML 콘텐츠에서 [[위키링크]]를 추출합니다.
func ParseWikiLinks(content string) []WikiLinkRef {
	matches := wikiLinkPattern.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	var refs []WikiLinkRef

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		text := match[1]
		if seen[text] {
			continue
		}
		seen[text] = true
		refs = append(refs, WikiLinkRef{Text: text})
	}
	return refs
}
