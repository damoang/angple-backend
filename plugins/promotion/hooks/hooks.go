//go:build ignore

package hooks

import (
	"context"
	"strings"

	"github.com/angple/core/hook"
	"github.com/angple/core/model"
	"github.com/angple/core/plugin"
)

var pluginCtx *plugin.Context

// Register 플러그인 Hook 등록
func Register(ctx *plugin.Context) {
	pluginCtx = ctx

	// 게시판 목록에 사잇광고 삽입
	hook.RegisterFilter("post.list", InsertPromotionPosts, 10)

	// 관리자 메뉴 추가
	hook.RegisterFilter("admin.menu", AddAdminMenu, 50)

	// 사이드바 위젯 추가
	hook.Register("template.sidebar", AddSidebarWidget, 10)
}

// InsertPromotionPosts 게시판 목록에 사잇광고 삽입
func InsertPromotionPosts(ctx context.Context, data *model.PostListData) *model.PostListData {
	// 제외 게시판 확인
	excludeBoards := pluginCtx.GetSetting("exclude_boards", "promotion,promotion_my,notice")
	excludeList := strings.Split(excludeBoards, ",")

	for _, board := range excludeList {
		if strings.TrimSpace(board) == data.BoardID {
			return data
		}
	}

	// 사잇광고 설정 가져오기
	insertPosition := pluginCtx.GetSettingInt("insert_position", 3)
	insertCount := pluginCtx.GetSettingInt("insert_count", 1)

	// 사잇광고 글 조회
	promotionPosts, err := getPromotionPostsForInsert(ctx, insertCount)
	if err != nil || len(promotionPosts) == 0 {
		return data
	}

	// 지정 위치에 삽입
	if insertPosition > len(data.Posts) {
		insertPosition = len(data.Posts)
	}

	// 새 슬라이스 생성하여 삽입
	newPosts := make([]model.Post, 0, len(data.Posts)+len(promotionPosts))
	newPosts = append(newPosts, data.Posts[:insertPosition]...)

	for _, pp := range promotionPosts {
		newPosts = append(newPosts, model.Post{
			ID:          pp.ID,
			Title:       pp.Title,
			Content:     pp.Content,
			AuthorName:  pp.AuthorName,
			Views:       pp.Views,
			CreatedAt:   pp.CreatedAt,
			IsPromotion: true,
			IsPinned:    pp.IsPinned,
		})
	}

	newPosts = append(newPosts, data.Posts[insertPosition:]...)
	data.Posts = newPosts

	return data
}

// AddAdminMenu 관리자 메뉴에 직홍게 관리 추가
func AddAdminMenu(ctx context.Context, menu *model.AdminMenu) *model.AdminMenu {
	menu.Items = append(menu.Items, model.MenuItem{
		ID:    "promotion",
		Label: "직접홍보 관리",
		Icon:  "megaphone",
		Path:  "/admin/plugins/promotion",
		Children: []model.MenuItem{
			{
				ID:    "promotion-advertisers",
				Label: "광고주 관리",
				Path:  "/admin/plugins/promotion/advertisers",
			},
			{
				ID:    "promotion-posts",
				Label: "직홍게 글 관리",
				Path:  "/admin/plugins/promotion/posts",
			},
			{
				ID:    "promotion-settings",
				Label: "설정",
				Path:  "/admin/plugins/promotion/settings",
			},
		},
	})

	return menu
}

// AddSidebarWidget 사이드바에 직홍게 최신글 위젯 추가
func AddSidebarWidget(ctx context.Context) {
	widgetCount := pluginCtx.GetSettingInt("sidebar_widget_count", 5)

	posts, err := getLatestPromotionPosts(ctx, widgetCount)
	if err != nil {
		return
	}

	// 위젯 HTML 생성 및 출력
	hook.Do("template.sidebar.widget", ctx, map[string]interface{}{
		"id":    "promotion-latest",
		"title": "직접홍보 최신글",
		"posts": posts,
	})
}

// PromotionPost 직홍게 글 구조체
type PromotionPost struct {
	ID         int64
	Title      string
	Content    string
	AuthorName string
	Views      int
	IsPinned   bool
	CreatedAt  string
}

// getPromotionPostsForInsert 사잇광고용 글 조회
func getPromotionPostsForInsert(ctx context.Context, count int) ([]PromotionPost, error) {
	db := pluginCtx.DB()

	query := `
		WITH active_advertisers AS (
			SELECT id, member_id, name, is_pinned
			FROM promotion_advertisers
			WHERE is_active = TRUE
			  AND (start_date IS NULL OR start_date <= CURDATE())
			  AND (end_date IS NULL OR end_date >= CURDATE())
		),
		latest_posts AS (
			SELECT p.*,
				   a.is_pinned,
				   a.name as author_name,
				   ROW_NUMBER() OVER (PARTITION BY p.advertiser_id ORDER BY p.created_at DESC) as rn
			FROM promotion_posts p
			JOIN active_advertisers a ON p.advertiser_id = a.id
			WHERE p.is_active = TRUE
		)
		SELECT id, title, content, author_name, views, is_pinned, created_at
		FROM latest_posts
		WHERE rn = 1
		ORDER BY is_pinned DESC, RAND()
		LIMIT ?
	`

	var posts []PromotionPost
	err := db.Raw(query, count).Scan(&posts).Error

	return posts, err
}

// getLatestPromotionPosts 최신 직홍게 글 조회
func getLatestPromotionPosts(ctx context.Context, count int) ([]PromotionPost, error) {
	db := pluginCtx.DB()

	query := `
		SELECT p.id, p.title, a.name as author_name, p.views, a.is_pinned, p.created_at
		FROM promotion_posts p
		JOIN promotion_advertisers a ON p.advertiser_id = a.id
		WHERE p.is_active = TRUE
		  AND a.is_active = TRUE
		  AND (a.start_date IS NULL OR a.start_date <= CURDATE())
		  AND (a.end_date IS NULL OR a.end_date >= CURDATE())
		ORDER BY a.is_pinned DESC, p.created_at DESC
		LIMIT ?
	`

	var posts []PromotionPost
	err := db.Raw(query, count).Scan(&posts).Error

	return posts, err
}
