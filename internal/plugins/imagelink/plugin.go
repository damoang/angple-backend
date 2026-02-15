package imagelink

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// init 플러그인 팩토리 자동 등록
func init() {
	plugin.RegisterFactory("imagelink", func() plugin.Plugin {
		return New()
	}, Manifest)
}

// Manifest 플러그인 매니페스트
var Manifest = &plugin.PluginManifest{
	Name:        "imagelink",
	Version:     "1.0.0",
	Title:       "이미지 링크 변환",
	Description: "게시글/댓글에서 [이미지URL] 형식을 <img> 태그로 변환합니다.",
	Author:      "Damoang Team",
	Requires: plugin.Requires{
		Angple: ">=1.0.0",
	},
	Settings: []plugin.SettingConfig{
		{
			Key:     "allowed_domains",
			Type:    "string",
			Default: "s3.damoang.net,damoang.net,damoang.com",
			Label:   "허용 도메인 (쉼표 구분, 빈 값이면 모든 도메인 허용)",
		},
		{
			Key:     "max_width",
			Type:    "number",
			Default: 0,
			Label:   "최대 너비 (px, 0이면 제한 없음)",
		},
		{
			Key:     "lazy_loading",
			Type:    "boolean",
			Default: true,
			Label:   "지연 로딩 (loading=lazy)",
		},
		{
			Key:     "link_wrapper",
			Type:    "boolean",
			Default: true,
			Label:   "이미지 클릭 시 원본 열기",
		},
	},
}

// Plugin 이미지 링크 변환 플러그인
type Plugin struct {
	allowedDomains map[string]bool
	maxWidth       int
	lazyLoading    bool
	linkWrapper    bool
	pattern        *regexp.Regexp
}

// New 플러그인 인스턴스 생성
func New() *Plugin {
	p := &Plugin{
		allowedDomains: make(map[string]bool),
		lazyLoading:    true,
		linkWrapper:    true,
	}

	// 기본 허용 도메인
	for _, domain := range []string{"s3.damoang.net", "damoang.net", "damoang.com"} {
		p.allowedDomains[domain] = true
	}

	// 정규식 컴파일 - [https://url.com/image.jpg] 패턴 매칭
	p.pattern = regexp.MustCompile(`(?i)\[(https?://[^\]\s]+\.(?:jpg|jpeg|png|gif|webp|bmp|svg|avif)(?:\?[^\]]*)?)\]`)

	return p
}

// Name 플러그인 이름 반환
func (p *Plugin) Name() string {
	return "imagelink"
}

// Initialize 플러그인 초기화
func (p *Plugin) Initialize(ctx *plugin.PluginContext) error {
	// 설정 로드
	if domains, ok := ctx.Config["allowed_domains"].(string); ok && domains != "" {
		p.allowedDomains = make(map[string]bool)
		for _, domain := range strings.Split(domains, ",") {
			domain = strings.TrimSpace(strings.ToLower(domain))
			if domain != "" {
				p.allowedDomains[domain] = true
			}
		}
	}

	if maxWidth, ok := ctx.Config["max_width"].(float64); ok {
		p.maxWidth = int(maxWidth)
	}

	if lazy, ok := ctx.Config["lazy_loading"].(bool); ok {
		p.lazyLoading = lazy
	}

	if wrapper, ok := ctx.Config["link_wrapper"].(bool); ok {
		p.linkWrapper = wrapper
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
func (p *Plugin) Migrate(_ *gorm.DB) error {
	return nil
}

// RegisterRoutes 라우트 등록 (필요 없음)
func (p *Plugin) RegisterRoutes(_ gin.IRouter) {
}

// RegisterHooks Hook 등록 (HookAware 인터페이스)
func (p *Plugin) RegisterHooks(hm *plugin.HookManager) {
	// 게시글 본문 필터
	hm.RegisterFilter("post.content", "imagelink", p.filterContent, 10)

	// 댓글 본문 필터
	hm.RegisterFilter("comment.content", "imagelink", p.filterContent, 10)
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

// Transform 콘텐츠 내 이미지 링크를 img 태그로 변환
func (p *Plugin) Transform(content string) string {
	return p.pattern.ReplaceAllStringFunc(content, func(match string) string {
		// [url] 에서 url 추출
		imageURL := match[1 : len(match)-1]

		// 도메인 검증
		if !p.isAllowedDomain(imageURL) {
			return match // 허용되지 않은 도메인은 변환하지 않음
		}

		// img 태그 생성
		return p.buildImgTag(imageURL)
	})
}

// isAllowedDomain 허용된 도메인인지 확인
func (p *Plugin) isAllowedDomain(imageURL string) bool {
	// 허용 도메인이 비어있으면 모든 도메인 허용
	if len(p.allowedDomains) == 0 {
		return true
	}

	parsed, err := url.Parse(imageURL)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Host)

	// 정확히 일치하거나 서브도메인인 경우 허용
	for domain := range p.allowedDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}

	return false
}

// buildImgTag img 태그 생성
func (p *Plugin) buildImgTag(imageURL string) string {
	var attrs []string

	// src 속성
	attrs = append(attrs, fmt.Sprintf(`src="%s"`, escapeHTML(imageURL)))

	// max-width 스타일
	if p.maxWidth > 0 {
		attrs = append(attrs, fmt.Sprintf(`style="max-width: %dpx"`, p.maxWidth))
	}

	// lazy loading
	if p.lazyLoading {
		attrs = append(attrs, `loading="lazy"`)
	}

	// alt 속성
	attrs = append(attrs, `alt="image"`)

	imgTag := fmt.Sprintf(`<img %s>`, strings.Join(attrs, " "))

	// 링크 감싸기
	if p.linkWrapper {
		return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener">%s</a>`, escapeHTML(imageURL), imgTag)
	}

	return imgTag
}

// escapeHTML HTML 특수문자 이스케이프
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
