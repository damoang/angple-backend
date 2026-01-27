//go:build ignore

// 배너 플러그인 Hook 구현
package hooks

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"angple-backend/pkg/plugin"

	"gorm.io/gorm"
)

var db *gorm.DB
var settings map[string]interface{}

// BannerItem 배너 아이템 모델
type BannerItem struct {
	ID         int64     `gorm:"primaryKey" json:"id"`
	Title      string    `json:"title"`
	ImageURL   string    `json:"image_url"`
	LinkURL    string    `json:"link_url"`
	Position   string    `json:"position"` // header, sidebar, content, footer
	StartDate  *time.Time `json:"start_date"`
	EndDate    *time.Time `json:"end_date"`
	Priority   int       `json:"priority"`
	IsActive   bool      `json:"is_active"`
	ClickCount int       `json:"click_count"`
	ViewCount  int       `json:"view_count"`
	AltText    string    `json:"alt_text"`
	Target     string    `json:"target"` // _self, _blank
	Memo       string    `json:"memo"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (BannerItem) TableName() string {
	return "banner_items"
}

// Register Hook 등록
func Register(ctx *plugin.Context) {
	db = ctx.DB
	settings = ctx.Settings
}

// GetActiveBanners 위치별 활성 배너 조회
func GetActiveBanners(position string, limit int) ([]BannerItem, error) {
	var banners []BannerItem
	now := time.Now()

	query := db.Model(&BannerItem{}).
		Where("position = ?", position).
		Where("is_active = ?", true).
		Where("(start_date IS NULL OR start_date <= ?)", now).
		Where("(end_date IS NULL OR end_date >= ?)", now).
		Order("priority DESC, created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&banners).Error; err != nil {
		return nil, err
	}

	return banners, nil
}

// RenderBannerHTML 배너 HTML 생성
func RenderBannerHTML(banners []BannerItem, containerClass string) template.HTML {
	if len(banners) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<div class="%s">`, containerClass))

	for _, banner := range banners {
		altText := banner.AltText
		if altText == "" {
			altText = banner.Title
		}

		target := banner.Target
		if target == "" {
			target = "_blank"
		}

		// 클릭 트래킹 URL
		trackingURL := fmt.Sprintf("/api/plugins/banner/%d/click", banner.ID)

		sb.WriteString(`<div class="banner-item">`)
		sb.WriteString(fmt.Sprintf(`<a href="%s" target="%s" rel="noopener noreferrer" data-banner-id="%d">`,
			trackingURL, target, banner.ID))

		if banner.ImageURL != "" {
			sb.WriteString(fmt.Sprintf(`<img src="%s" alt="%s" loading="lazy" />`,
				template.HTMLEscapeString(banner.ImageURL),
				template.HTMLEscapeString(altText)))
		}

		sb.WriteString(`</a></div>`)
	}

	sb.WriteString(`</div>`)
	return template.HTML(sb.String())
}

// AddHeaderBanner 헤더 배너 삽입
func AddHeaderBanner(ctx *plugin.HookContext) error {
	enabled, ok := settings["header_enabled"].(bool)
	if !ok || !enabled {
		return nil
	}

	banners, err := GetActiveBanners("header", 1)
	if err != nil {
		return err
	}

	html := RenderBannerHTML(banners, "banner-header")
	ctx.SetOutput("html", string(html))
	return nil
}

// AddSidebarBanner 사이드바 배너 삽입
func AddSidebarBanner(ctx *plugin.HookContext) error {
	enabled, ok := settings["sidebar_enabled"].(bool)
	if !ok || !enabled {
		return nil
	}

	banners, err := GetActiveBanners("sidebar", 5)
	if err != nil {
		return err
	}

	html := RenderBannerHTML(banners, "banner-sidebar")
	ctx.SetOutput("html", string(html))
	return nil
}

// AddFooterBanner 푸터 배너 삽입
func AddFooterBanner(ctx *plugin.HookContext) error {
	enabled, ok := settings["footer_enabled"].(bool)
	if !ok || !enabled {
		return nil
	}

	banners, err := GetActiveBanners("footer", 3)
	if err != nil {
		return err
	}

	html := RenderBannerHTML(banners, "banner-footer")
	ctx.SetOutput("html", string(html))
	return nil
}

// InsertContentBanner 콘텐츠 중간 배너 삽입
func InsertContentBanner(ctx *plugin.HookContext) error {
	enabled, ok := settings["content_enabled"].(bool)
	if !ok || !enabled {
		return nil
	}

	content, ok := ctx.Input["content"].(string)
	if !ok || content == "" {
		return nil
	}

	banners, err := GetActiveBanners("content", 1)
	if err != nil {
		return err
	}

	if len(banners) == 0 {
		return nil
	}

	// 삽입 위치 설정 (기본: 3번째 문단 뒤)
	insertAfter := 3
	if pos, ok := settings["content_insert_after_paragraph"].(int); ok {
		insertAfter = pos
	}

	bannerHTML := RenderBannerHTML(banners, "banner-content-inline")

	// 문단(<p>) 태그 기준으로 삽입
	paragraphs := strings.Split(content, "</p>")
	if len(paragraphs) > insertAfter {
		result := strings.Join(paragraphs[:insertAfter], "</p>") + "</p>"
		result += string(bannerHTML)
		result += strings.Join(paragraphs[insertAfter:], "</p>")
		ctx.SetOutput("content", result)
	}

	return nil
}

// AddAdminMenu 관리자 메뉴 추가
func AddAdminMenu(ctx *plugin.HookContext) error {
	menus, ok := ctx.Input["menus"].([]map[string]interface{})
	if !ok {
		menus = []map[string]interface{}{}
	}

	menus = append(menus, map[string]interface{}{
		"id":       "banner",
		"title":    "배너 광고",
		"icon":     "image",
		"path":     "/admin/plugins/banner",
		"priority": 50,
	})

	ctx.SetOutput("menus", menus)
	return nil
}
