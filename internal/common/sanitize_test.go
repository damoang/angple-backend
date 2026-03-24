package common

import (
	"strings"
	"testing"
)

func TestSanitizePostContent_RemovesScript(t *testing.T) {
	input := `<p>Hello</p><script>alert('xss')</script><p>World</p>`
	result := SanitizePostContent(input)
	if strings.Contains(result, "<script") {
		t.Errorf("script tag not removed: %s", result)
	}
	if !strings.Contains(result, "<p>Hello</p>") {
		t.Errorf("safe p tag removed: %s", result)
	}
}

func TestSanitizePostContent_PreservesSafeTags(t *testing.T) {
	input := `<p>text</p><strong>bold</strong><em>italic</em><a href="https://example.com">link</a>`
	result := SanitizePostContent(input)
	if !strings.Contains(result, "<p>") {
		t.Errorf("p tag removed: %s", result)
	}
	if !strings.Contains(result, "<strong>") {
		t.Errorf("strong tag removed: %s", result)
	}
	if !strings.Contains(result, "<a ") {
		t.Errorf("a tag removed: %s", result)
	}
}

func TestSanitizePostContent_PreservesImg(t *testing.T) {
	input := `<img src="https://example.com/photo.jpg" alt="photo" loading="lazy">`
	result := SanitizePostContent(input)
	if !strings.Contains(result, "<img") {
		t.Errorf("img tag removed: %s", result)
	}
	if !strings.Contains(result, `alt="photo"`) {
		t.Errorf("alt attribute removed: %s", result)
	}
}

func TestSanitizePostContent_YouTubeIframePreserved(t *testing.T) {
	input := `<iframe src="https://www.youtube.com/embed/abc123" width="560" height="315" frameborder="0" allowfullscreen></iframe>`
	result := SanitizePostContent(input)
	if !strings.Contains(result, "youtube.com/embed/abc123") {
		t.Errorf("YouTube iframe removed: %s", result)
	}
}

func TestSanitizePostContent_YouTubeNoCookiePreserved(t *testing.T) {
	input := `<iframe src="https://www.youtube-nocookie.com/embed/abc123" width="560" height="315"></iframe>`
	result := SanitizePostContent(input)
	if !strings.Contains(result, "youtube-nocookie.com") {
		t.Errorf("YouTube nocookie iframe removed: %s", result)
	}
}

func TestSanitizePostContent_NonYoutubeIframeRemoved(t *testing.T) {
	input := `<iframe src="https://evil.com/phish" width="100%" height="500"></iframe>`
	result := SanitizePostContent(input)
	if strings.Contains(result, "evil.com") {
		t.Errorf("non-YouTube iframe not removed: %s", result)
	}
}

func TestSanitizePostContent_DangerousCSSRemoved(t *testing.T) {
	input := `<div style="position: fixed; z-index: 9999; top: 0; left: 0; width: 100vw;">overlay</div>`
	result := SanitizePostContent(input)
	if strings.Contains(result, "position") {
		t.Errorf("dangerous CSS 'position' not removed: %s", result)
	}
	if strings.Contains(result, "z-index") {
		t.Errorf("dangerous CSS 'z-index' not removed: %s", result)
	}
}

func TestSanitizePostContent_SafeCSSPreserved(t *testing.T) {
	input := `<p style="color: red; font-size: 16px;">styled</p>`
	result := SanitizePostContent(input)
	if !strings.Contains(result, "color") {
		t.Errorf("safe CSS 'color' removed: %s", result)
	}
	if !strings.Contains(result, "font-size") {
		t.Errorf("safe CSS 'font-size' removed: %s", result)
	}
}

func TestSanitizePostContent_JavascriptHrefBlocked(t *testing.T) {
	input := `<a href="javascript:alert('xss')">click</a>`
	result := SanitizePostContent(input)
	if strings.Contains(result, "javascript:") {
		t.Errorf("javascript: href not blocked: %s", result)
	}
}

func TestSanitizePostContent_OnEventHandlersRemoved(t *testing.T) {
	input := `<img src="x" onerror="alert('xss')">`
	result := SanitizePostContent(input)
	if strings.Contains(result, "onerror") {
		t.Errorf("onerror attribute not removed: %s", result)
	}
}

func TestSanitizeComment_BasicFormatting(t *testing.T) {
	input := `<p>Hello <strong>bold</strong> <em>italic</em></p>`
	result := SanitizeComment(input)
	if !strings.Contains(result, "<strong>") {
		t.Errorf("strong tag removed from comment: %s", result)
	}
}

func TestSanitizeComment_AllowsSafeImg(t *testing.T) {
	input := `<p>text</p><img src="https://example.com/img.jpg" alt="첨부" loading="lazy" onerror="alert('xss')"><p>more</p>`
	result := SanitizeComment(input)
	if !strings.Contains(result, "<img") {
		t.Errorf("img tag removed from comment: %s", result)
	}
	if strings.Contains(result, "onerror") {
		t.Errorf("unsafe img attribute not removed from comment: %s", result)
	}
}

func TestSanitizeComment_NoIframeAllowed(t *testing.T) {
	input := `<iframe src="https://youtube.com/embed/abc"></iframe>`
	result := SanitizeComment(input)
	if strings.Contains(result, "<iframe") {
		t.Errorf("iframe should not be allowed in comments: %s", result)
	}
}

func TestSanitizeComment_NofollowAdded(t *testing.T) {
	input := `<a href="https://example.com">link</a>`
	result := SanitizeComment(input)
	if !strings.Contains(result, "nofollow") {
		t.Errorf("nofollow not added to comment link: %s", result)
	}
}

func TestSanitizeMessage_StripsAllHTML(t *testing.T) {
	input := `<p>Hello <strong>World</strong></p><script>alert('x')</script>`
	result := SanitizeMessage(input)
	if strings.Contains(result, "<") {
		t.Errorf("HTML tags not stripped: %s", result)
	}
	if !strings.Contains(result, "Hello") || !strings.Contains(result, "World") {
		t.Errorf("text content lost: %s", result)
	}
}

func TestSanitizePostContent_DataAttributes(t *testing.T) {
	input := `<div data-youtube-video="true" data-platform="youtube">embed</div>`
	result := SanitizePostContent(input)
	if !strings.Contains(result, "data-youtube-video") {
		t.Errorf("data-youtube-video attribute removed: %s", result)
	}
	if !strings.Contains(result, "data-platform") {
		t.Errorf("data-platform attribute removed: %s", result)
	}
}

func TestSanitizePostContent_TwitterIframePreserved(t *testing.T) {
	input := `<iframe src="https://platform.twitter.com/embed/Tweet.html?id=123" width="550" height="300" data-tweet-id="123"></iframe>`
	result := SanitizePostContent(input)
	if !strings.Contains(result, "platform.twitter.com") {
		t.Errorf("Twitter iframe removed: %s", result)
	}
}

func TestSanitizePostContent_BlueskyIframePreserved(t *testing.T) {
	input := `<iframe src="https://embed.bsky.app/embed/post/abc" width="550" height="300"></iframe>`
	result := SanitizePostContent(input)
	if !strings.Contains(result, "embed.bsky.app") {
		t.Errorf("Bluesky iframe removed: %s", result)
	}
}

func TestSanitizePostContent_VimeoIframePreserved(t *testing.T) {
	input := `<iframe src="https://player.vimeo.com/video/123456" width="640" height="360"></iframe>`
	result := SanitizePostContent(input)
	if !strings.Contains(result, "player.vimeo.com") {
		t.Errorf("Vimeo iframe removed: %s", result)
	}
}

func TestSanitizePostContent_SpotifyIframePreserved(t *testing.T) {
	input := `<iframe src="https://open.spotify.com/embed/track/abc" width="300" height="380"></iframe>`
	result := SanitizePostContent(input)
	if !strings.Contains(result, "open.spotify.com") {
		t.Errorf("Spotify iframe removed: %s", result)
	}
}

func TestSanitizePostContent_InstagramIframePreserved(t *testing.T) {
	input := `<iframe src="https://www.instagram.com/p/abc123/embed" width="400" height="500"></iframe>`
	result := SanitizePostContent(input)
	if !strings.Contains(result, "instagram.com") {
		t.Errorf("Instagram iframe removed: %s", result)
	}
}

func TestSanitizePostContent_DetailsTag(t *testing.T) {
	input := `<details open><summary>Title</summary><p>Content</p></details>`
	result := SanitizePostContent(input)
	if !strings.Contains(result, "<details") {
		t.Errorf("details tag removed: %s", result)
	}
	if !strings.Contains(result, "<summary>") {
		t.Errorf("summary tag removed: %s", result)
	}
}
