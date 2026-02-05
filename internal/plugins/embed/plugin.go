package embed

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// init 플러그인 팩토리 자동 등록
func init() {
	plugin.RegisterFactory("embed", func() plugin.Plugin {
		return New()
	}, Manifest)
}

// Manifest 플러그인 매니페스트
var Manifest = &plugin.PluginManifest{
	Name:        "embed",
	Version:     "1.0.0",
	Title:       "미디어 임베드",
	Description: "YouTube, Twitter, Instagram 등의 URL을 임베드 플레이어로 변환합니다.",
	Author:      "Damoang Team",
	Requires: plugin.Requires{
		Angple: ">=1.0.0",
	},
	Settings: []plugin.SettingConfig{
		{
			Key:     "youtube_enabled",
			Type:    "boolean",
			Default: true,
			Label:   "YouTube 임베드 활성화",
		},
		{
			Key:     "twitter_enabled",
			Type:    "boolean",
			Default: true,
			Label:   "Twitter/X 임베드 활성화",
		},
		{
			Key:     "instagram_enabled",
			Type:    "boolean",
			Default: true,
			Label:   "Instagram 임베드 활성화",
		},
		{
			Key:     "max_width",
			Type:    "number",
			Default: 560,
			Label:   "최대 너비 (px)",
		},
		{
			Key:     "aspect_ratio",
			Type:    "string",
			Default: "16:9",
			Label:   "비디오 비율 (16:9, 4:3)",
		},
	},
}

// Plugin 미디어 임베드 플러그인
type Plugin struct {
	youtubeEnabled   bool
	twitterEnabled   bool
	instagramEnabled bool
	maxWidth         int
	aspectRatio      string

	// 정규식 패턴들
	youtubePattern   *regexp.Regexp
	twitterPattern   *regexp.Regexp
	instagramPattern *regexp.Regexp
}

// New 플러그인 인스턴스 생성
func New() *Plugin {
	p := &Plugin{
		youtubeEnabled:   true,
		twitterEnabled:   true,
		instagramEnabled: true,
		maxWidth:         560,
		aspectRatio:      "16:9",
	}

	// YouTube 패턴: youtube.com/watch?v=ID, youtu.be/ID, youtube.com/embed/ID
	p.youtubePattern = regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?(?:youtube\.com/(?:watch\?v=|embed/)|youtu\.be/)([a-zA-Z0-9_-]{11})(?:[?&][^\s]*)?`)

	// Twitter/X 패턴: twitter.com/user/status/ID, x.com/user/status/ID
	p.twitterPattern = regexp.MustCompile(`(?i)https?://(?:www\.)?(?:twitter\.com|x\.com)/([^/]+)/status/(\d+)`)

	// Instagram 패턴: instagram.com/p/ID, instagram.com/reel/ID
	p.instagramPattern = regexp.MustCompile(`(?i)https?://(?:www\.)?instagram\.com/(?:p|reel)/([a-zA-Z0-9_-]+)/?`)

	return p
}

// Name 플러그인 이름 반환
func (p *Plugin) Name() string {
	return "embed"
}

// Initialize 플러그인 초기화
func (p *Plugin) Initialize(ctx *plugin.PluginContext) error {
	if v, ok := ctx.Config["youtube_enabled"].(bool); ok {
		p.youtubeEnabled = v
	}
	if v, ok := ctx.Config["twitter_enabled"].(bool); ok {
		p.twitterEnabled = v
	}
	if v, ok := ctx.Config["instagram_enabled"].(bool); ok {
		p.instagramEnabled = v
	}
	if v, ok := ctx.Config["max_width"].(float64); ok {
		p.maxWidth = int(v)
	}
	if v, ok := ctx.Config["aspect_ratio"].(string); ok {
		p.aspectRatio = v
	}

	return nil
}

// Shutdown 플러그인 종료
func (p *Plugin) Shutdown() error {
	return nil
}

// GetManifest 매니페스트 반환
func (p *Plugin) GetManifest() *plugin.PluginManifest {
	return Manifest
}

// Migrate DB 마이그레이션 (필요 없음)
func (p *Plugin) Migrate(db *gorm.DB) error {
	return nil
}

// RegisterRoutes 라우트 등록 (필요 없음)
func (p *Plugin) RegisterRoutes(router gin.IRouter) {
}

// RegisterHooks Hook 등록 (HookAware 인터페이스)
func (p *Plugin) RegisterHooks(hm *plugin.HookManager) {
	// 게시글 본문 필터 (imagelink 다음에 실행되도록 priority 20)
	hm.RegisterFilter("post.content", "embed", p.filterContent, 20)

	// 댓글 본문 필터
	hm.RegisterFilter("comment.content", "embed", p.filterContent, 20)
}

// filterContent 콘텐츠 필터 핸들러
func (p *Plugin) filterContent(ctx *plugin.HookContext) error {
	content, ok := ctx.Input["content"].(string)
	if !ok || content == "" {
		return nil
	}

	filtered := p.Transform(content)
	ctx.SetOutput(map[string]interface{}{
		"content": filtered,
	})
	return nil
}

// Transform 콘텐츠 내 URL을 임베드로 변환
func (p *Plugin) Transform(content string) string {
	// YouTube 변환
	if p.youtubeEnabled {
		content = p.youtubePattern.ReplaceAllStringFunc(content, p.embedYouTube)
	}

	// Twitter 변환
	if p.twitterEnabled {
		content = p.twitterPattern.ReplaceAllStringFunc(content, p.embedTwitter)
	}

	// Instagram 변환
	if p.instagramEnabled {
		content = p.instagramPattern.ReplaceAllStringFunc(content, p.embedInstagram)
	}

	return content
}

// embedYouTube YouTube URL을 iframe으로 변환
func (p *Plugin) embedYouTube(match string) string {
	matches := p.youtubePattern.FindStringSubmatch(match)
	if len(matches) < 2 {
		return match
	}

	videoID := matches[1]
	height := p.calculateHeight()

	return fmt.Sprintf(
		`<div class="embed-container youtube" style="max-width:%dpx">
<iframe src="https://www.youtube.com/embed/%s" width="%d" height="%d" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen loading="lazy"></iframe>
</div>`,
		p.maxWidth, videoID, p.maxWidth, height,
	)
}

// embedTwitter Twitter URL을 임베드로 변환
func (p *Plugin) embedTwitter(match string) string {
	matches := p.twitterPattern.FindStringSubmatch(match)
	if len(matches) < 3 {
		return match
	}

	user := matches[1]
	tweetID := matches[2]

	// Twitter oEmbed 방식 (클라이언트 사이드 렌더링)
	return fmt.Sprintf(
		`<div class="embed-container twitter" style="max-width:%dpx">
<blockquote class="twitter-tweet" data-dnt="true"><a href="https://twitter.com/%s/status/%s"></a></blockquote>
<script async src="https://platform.twitter.com/widgets.js" charset="utf-8"></script>
</div>`,
		p.maxWidth, user, tweetID,
	)
}

// embedInstagram Instagram URL을 임베드로 변환
func (p *Plugin) embedInstagram(match string) string {
	matches := p.instagramPattern.FindStringSubmatch(match)
	if len(matches) < 2 {
		return match
	}

	postID := matches[1]

	// Instagram oEmbed 방식
	return fmt.Sprintf(
		`<div class="embed-container instagram" style="max-width:%dpx">
<blockquote class="instagram-media" data-instgrm-permalink="https://www.instagram.com/p/%s/" data-instgrm-version="14"></blockquote>
<script async src="https://www.instagram.com/embed.js"></script>
</div>`,
		p.maxWidth, postID,
	)
}

// calculateHeight 비율에 따른 높이 계산
func (p *Plugin) calculateHeight() int {
	switch p.aspectRatio {
	case "4:3":
		return p.maxWidth * 3 / 4
	case "1:1":
		return p.maxWidth
	default: // 16:9
		return p.maxWidth * 9 / 16
	}
}

// escapeHTML HTML 특수문자 이스케이프
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
