//go:build ignore

package handlers

import (
	"errors"
	"time"

	"github.com/angple/core/plugin"
)

// ========== Domain Models ==========

// Advertiser 광고주 모델
type Advertiser struct {
	ID        int64      `json:"id" gorm:"primaryKey"`
	MemberID  string     `json:"member_id" gorm:"column:member_id"`
	Name      string     `json:"name"`
	PostCount int        `json:"post_count" gorm:"column:post_count"`
	StartDate *time.Time `json:"start_date" gorm:"column:start_date"`
	EndDate   *time.Time `json:"end_date" gorm:"column:end_date"`
	IsPinned  bool       `json:"is_pinned" gorm:"column:is_pinned"`
	IsActive  bool       `json:"is_active" gorm:"column:is_active"`
	Memo      string     `json:"memo,omitempty"`
	CreatedAt time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"column:updated_at"`
}

func (Advertiser) TableName() string {
	return "promotion_advertisers"
}

// PromotionPost 직홍게 글 모델
type PromotionPost struct {
	ID           int64     `json:"id" gorm:"primaryKey"`
	AdvertiserID int64     `json:"advertiser_id" gorm:"column:advertiser_id"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	LinkURL      string    `json:"link_url,omitempty" gorm:"column:link_url"`
	ImageURL     string    `json:"image_url,omitempty" gorm:"column:image_url"`
	Views        int       `json:"views"`
	Likes        int       `json:"likes"`
	CommentCount int       `json:"comment_count" gorm:"column:comment_count"`
	IsActive     bool      `json:"is_active" gorm:"column:is_active"`
	CreatedAt    time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"column:updated_at"`

	// Joined fields
	AuthorName  string `json:"author_name,omitempty" gorm:"-"`
	IsPinned    bool   `json:"is_pinned,omitempty" gorm:"-"`
	IsPromotion bool   `json:"is_promotion" gorm:"-"`
}

func (PromotionPost) TableName() string {
	return "promotion_posts"
}

// ========== Request/Response DTOs ==========

type CreateAdvertiserRequest struct {
	MemberID  string     `json:"member_id" binding:"required"`
	Name      string     `json:"name" binding:"required"`
	PostCount int        `json:"post_count"`
	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
	IsPinned  bool       `json:"is_pinned"`
	IsActive  bool       `json:"is_active"`
	Memo      string     `json:"memo"`
}

type UpdateAdvertiserRequest struct {
	Name      string     `json:"name"`
	PostCount int        `json:"post_count"`
	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
	IsPinned  bool       `json:"is_pinned"`
	IsActive  bool       `json:"is_active"`
	Memo      string     `json:"memo"`
}

type CreatePostRequest struct {
	Title    string `json:"title" binding:"required"`
	Content  string `json:"content"`
	LinkURL  string `json:"link_url"`
	ImageURL string `json:"image_url"`
}

type UpdatePostRequest struct {
	Title    string `json:"title"`
	Content  string `json:"content"`
	LinkURL  string `json:"link_url"`
	ImageURL string `json:"image_url"`
	IsActive bool   `json:"is_active"`
}

// ========== Public API Handlers ==========

// ListPosts 직홍게 글 목록 조회
func ListPosts(ctx *plugin.Context) error {
	page := ctx.QueryInt("page", 1)
	limit := ctx.QueryInt("limit", 20)
	offset := (page - 1) * limit

	db := ctx.DB()

	query := `
		WITH active_advertisers AS (
			SELECT id, member_id, name, post_count, is_pinned
			FROM promotion_advertisers
			WHERE is_active = TRUE
			  AND (start_date IS NULL OR start_date <= CURDATE())
			  AND (end_date IS NULL OR end_date >= CURDATE())
		),
		ranked_posts AS (
			SELECT p.*,
				   a.is_pinned,
				   a.name as author_name,
				   ROW_NUMBER() OVER (PARTITION BY p.advertiser_id ORDER BY p.created_at DESC) as rn,
				   a.post_count
			FROM promotion_posts p
			JOIN active_advertisers a ON p.advertiser_id = a.id
			WHERE p.is_active = TRUE
		)
		SELECT id, advertiser_id, title, content, link_url, image_url,
		       views, likes, comment_count, is_active, created_at, updated_at,
		       author_name, is_pinned
		FROM ranked_posts
		WHERE rn <= post_count
		ORDER BY is_pinned DESC, created_at DESC
		LIMIT ? OFFSET ?
	`

	var posts []PromotionPost
	if err := db.Raw(query, limit, offset).Scan(&posts).Error; err != nil {
		return ctx.InternalError(err)
	}

	// Mark as promotion posts
	for i := range posts {
		posts[i].IsPromotion = true
	}

	// Get total count
	var total int64
	countQuery := `
		WITH active_advertisers AS (
			SELECT id, post_count
			FROM promotion_advertisers
			WHERE is_active = TRUE
			  AND (start_date IS NULL OR start_date <= CURDATE())
			  AND (end_date IS NULL OR end_date >= CURDATE())
		),
		ranked_posts AS (
			SELECT p.id,
				   ROW_NUMBER() OVER (PARTITION BY p.advertiser_id ORDER BY p.created_at DESC) as rn,
				   a.post_count
			FROM promotion_posts p
			JOIN active_advertisers a ON p.advertiser_id = a.id
			WHERE p.is_active = TRUE
		)
		SELECT COUNT(*) FROM ranked_posts WHERE rn <= post_count
	`
	db.Raw(countQuery).Scan(&total)

	return ctx.JSON(map[string]interface{}{
		"posts": posts,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetPostsForInsert 사잇광고용 글 조회
func GetPostsForInsert(ctx *plugin.Context) error {
	count := ctx.QueryInt("count", 3)
	db := ctx.DB()

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
		SELECT id, advertiser_id, title, content, link_url, image_url,
		       views, likes, comment_count, is_active, created_at, updated_at,
		       author_name, is_pinned
		FROM latest_posts
		WHERE rn = 1
		ORDER BY is_pinned DESC, RAND()
		LIMIT ?
	`

	var posts []PromotionPost
	if err := db.Raw(query, count).Scan(&posts).Error; err != nil {
		return ctx.InternalError(err)
	}

	for i := range posts {
		posts[i].IsPromotion = true
	}

	return ctx.JSON(posts)
}

// GetPost 직홍게 글 상세 조회
func GetPost(ctx *plugin.Context) error {
	id := ctx.ParamInt64("id")
	db := ctx.DB()

	// Increment views
	db.Exec("UPDATE promotion_posts SET views = views + 1 WHERE id = ?", id)

	query := `
		SELECT p.*, a.name as author_name, a.is_pinned
		FROM promotion_posts p
		JOIN promotion_advertisers a ON p.advertiser_id = a.id
		WHERE p.id = ?
	`

	var post PromotionPost
	if err := db.Raw(query, id).Scan(&post).Error; err != nil {
		return ctx.NotFound("게시글을 찾을 수 없습니다")
	}

	post.IsPromotion = true
	return ctx.JSON(post)
}

// ========== Advertiser API Handlers ==========

// CreatePost 직홍게 글 작성 (광고주만)
func CreatePost(ctx *plugin.Context) error {
	userID := ctx.User().ID

	// 광고주 확인
	db := ctx.DB()
	var advertiser Advertiser
	if err := db.Where("member_id = ? AND is_active = TRUE", userID).First(&advertiser).Error; err != nil {
		return ctx.Forbidden("광고주만 글을 작성할 수 있습니다")
	}

	var req CreatePostRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.BadRequest(err)
	}

	post := PromotionPost{
		AdvertiserID: advertiser.ID,
		Title:        req.Title,
		Content:      req.Content,
		LinkURL:      req.LinkURL,
		ImageURL:     req.ImageURL,
		IsActive:     true,
	}

	if err := db.Create(&post).Error; err != nil {
		return ctx.InternalError(err)
	}

	post.AuthorName = advertiser.Name
	post.IsPinned = advertiser.IsPinned
	post.IsPromotion = true

	return ctx.JSON(post)
}

// UpdatePost 직홍게 글 수정
func UpdatePost(ctx *plugin.Context) error {
	id := ctx.ParamInt64("id")
	userID := ctx.User().ID
	db := ctx.DB()

	// 광고주 확인
	var advertiser Advertiser
	if err := db.Where("member_id = ? AND is_active = TRUE", userID).First(&advertiser).Error; err != nil {
		return ctx.Forbidden("광고주만 글을 수정할 수 있습니다")
	}

	// 글 조회 및 소유권 확인
	var post PromotionPost
	if err := db.First(&post, id).Error; err != nil {
		return ctx.NotFound("게시글을 찾을 수 없습니다")
	}

	if post.AdvertiserID != advertiser.ID {
		return ctx.Forbidden("본인의 글만 수정할 수 있습니다")
	}

	var req UpdatePostRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.BadRequest(err)
	}

	updates := map[string]interface{}{}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Content != "" {
		updates["content"] = req.Content
	}
	updates["link_url"] = req.LinkURL
	updates["image_url"] = req.ImageURL
	updates["is_active"] = req.IsActive

	if err := db.Model(&post).Updates(updates).Error; err != nil {
		return ctx.InternalError(err)
	}

	return ctx.JSON(post)
}

// DeletePost 직홍게 글 삭제
func DeletePost(ctx *plugin.Context) error {
	id := ctx.ParamInt64("id")
	userID := ctx.User().ID
	db := ctx.DB()

	// 광고주 확인
	var advertiser Advertiser
	if err := db.Where("member_id = ? AND is_active = TRUE", userID).First(&advertiser).Error; err != nil {
		return ctx.Forbidden("광고주만 글을 삭제할 수 있습니다")
	}

	// 글 조회 및 소유권 확인
	var post PromotionPost
	if err := db.First(&post, id).Error; err != nil {
		return ctx.NotFound("게시글을 찾을 수 없습니다")
	}

	if post.AdvertiserID != advertiser.ID {
		return ctx.Forbidden("본인의 글만 삭제할 수 있습니다")
	}

	if err := db.Delete(&post).Error; err != nil {
		return ctx.InternalError(err)
	}

	return ctx.JSON(map[string]string{"message": "삭제되었습니다"})
}

// ========== Admin API Handlers ==========

// ListAdvertisers 광고주 목록 조회
func ListAdvertisers(ctx *plugin.Context) error {
	// TODO: 관리자 권한 확인
	db := ctx.DB()

	var advertisers []Advertiser
	if err := db.Order("created_at DESC").Find(&advertisers).Error; err != nil {
		return ctx.InternalError(err)
	}

	return ctx.JSON(advertisers)
}

// CreateAdvertiser 광고주 추가
func CreateAdvertiser(ctx *plugin.Context) error {
	// TODO: 관리자 권한 확인
	db := ctx.DB()

	var req CreateAdvertiserRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.BadRequest(err)
	}

	// 중복 확인
	var existing Advertiser
	if err := db.Where("member_id = ?", req.MemberID).First(&existing).Error; err == nil {
		return ctx.Error(409, "CONFLICT", "이미 등록된 광고주입니다")
	}

	postCount := req.PostCount
	if postCount <= 0 {
		postCount = 1
	}

	advertiser := Advertiser{
		MemberID:  req.MemberID,
		Name:      req.Name,
		PostCount: postCount,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		IsPinned:  req.IsPinned,
		IsActive:  req.IsActive,
		Memo:      req.Memo,
	}

	if err := db.Create(&advertiser).Error; err != nil {
		return ctx.InternalError(err)
	}

	return ctx.JSON(advertiser)
}

// UpdateAdvertiser 광고주 수정
func UpdateAdvertiser(ctx *plugin.Context) error {
	// TODO: 관리자 권한 확인
	id := ctx.ParamInt64("id")
	db := ctx.DB()

	var advertiser Advertiser
	if err := db.First(&advertiser, id).Error; err != nil {
		return ctx.NotFound("광고주를 찾을 수 없습니다")
	}

	var req UpdateAdvertiserRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.BadRequest(err)
	}

	updates := map[string]interface{}{
		"is_pinned":  req.IsPinned,
		"is_active":  req.IsActive,
		"memo":       req.Memo,
		"start_date": req.StartDate,
		"end_date":   req.EndDate,
	}

	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.PostCount > 0 {
		updates["post_count"] = req.PostCount
	}

	if err := db.Model(&advertiser).Updates(updates).Error; err != nil {
		return ctx.InternalError(err)
	}

	return ctx.JSON(advertiser)
}

// DeleteAdvertiser 광고주 삭제
func DeleteAdvertiser(ctx *plugin.Context) error {
	// TODO: 관리자 권한 확인
	id := ctx.ParamInt64("id")
	db := ctx.DB()

	if err := db.Delete(&Advertiser{}, id).Error; err != nil {
		return ctx.InternalError(err)
	}

	return ctx.JSON(map[string]string{"message": "삭제되었습니다"})
}
