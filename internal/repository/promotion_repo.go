package repository

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// PromotionRepository defines the interface for promotion data access
type PromotionRepository interface {
	// Advertiser methods
	GetAllAdvertisers() ([]*domain.Advertiser, error)
	GetActiveAdvertisers() ([]*domain.Advertiser, error)
	FindAdvertiserByID(id int64) (*domain.Advertiser, error)
	FindAdvertiserByMemberID(memberID string) (*domain.Advertiser, error)
	CreateAdvertiser(advertiser *domain.Advertiser) error
	UpdateAdvertiser(advertiser *domain.Advertiser) error
	DeleteAdvertiser(id int64) error

	// Promotion Post methods
	GetPromotionPosts(page, limit int) ([]*domain.PromotionPost, int64, error)
	GetPromotionPostsForInsert(count int) ([]*domain.PromotionPost, error)
	FindPromotionPostByID(id int64) (*domain.PromotionPost, error)
	CreatePromotionPost(post *domain.PromotionPost) error
	UpdatePromotionPost(post *domain.PromotionPost) error
	DeletePromotionPost(id int64) error
	IncrementViews(id int64) error

	// Stats methods
	GetPostStatsByAdvertiser(advertiserID int64) (totalViews int, totalLikes int, postCount int, err error)
}

// promotionRepository implements PromotionRepository with GORM
type promotionRepository struct {
	db *gorm.DB
}

// NewPromotionRepository creates a new PromotionRepository
func NewPromotionRepository(db *gorm.DB) PromotionRepository {
	return &promotionRepository{db: db}
}

// GetAllAdvertisers retrieves all advertisers
func (r *promotionRepository) GetAllAdvertisers() ([]*domain.Advertiser, error) {
	var advertisers []*domain.Advertiser

	err := r.db.
		Order("created_at DESC").
		Find(&advertisers).Error

	if err != nil {
		return nil, err
	}

	return advertisers, nil
}

// GetActiveAdvertisers retrieves active advertisers within valid date range
func (r *promotionRepository) GetActiveAdvertisers() ([]*domain.Advertiser, error) {
	var advertisers []*domain.Advertiser

	err := r.db.
		Where("is_active = ?", true).
		Where("(start_date IS NULL OR start_date <= CURDATE())").
		Where("(end_date IS NULL OR end_date >= CURDATE())").
		Order("is_pinned DESC, created_at DESC").
		Find(&advertisers).Error

	if err != nil {
		return nil, err
	}

	return advertisers, nil
}

// FindAdvertiserByID finds an advertiser by ID
func (r *promotionRepository) FindAdvertiserByID(id int64) (*domain.Advertiser, error) {
	var advertiser domain.Advertiser

	err := r.db.
		Where("id = ?", id).
		First(&advertiser).Error

	if err != nil {
		return nil, err
	}

	return &advertiser, nil
}

// FindAdvertiserByMemberID finds an advertiser by member ID
func (r *promotionRepository) FindAdvertiserByMemberID(memberID string) (*domain.Advertiser, error) {
	var advertiser domain.Advertiser

	err := r.db.
		Where("member_id = ?", memberID).
		First(&advertiser).Error

	if err != nil {
		return nil, err
	}

	return &advertiser, nil
}

// CreateAdvertiser creates a new advertiser
func (r *promotionRepository) CreateAdvertiser(advertiser *domain.Advertiser) error {
	return r.db.Create(advertiser).Error
}

// UpdateAdvertiser updates an advertiser
func (r *promotionRepository) UpdateAdvertiser(advertiser *domain.Advertiser) error {
	return r.db.Save(advertiser).Error
}

// DeleteAdvertiser deletes an advertiser by ID
func (r *promotionRepository) DeleteAdvertiser(id int64) error {
	return r.db.Delete(&domain.Advertiser{}, id).Error
}

// GetPromotionPosts retrieves promotion posts with pagination
// Uses Window Function to get configured number of posts per advertiser
func (r *promotionRepository) GetPromotionPosts(page, limit int) ([]*domain.PromotionPost, int64, error) {
	var posts []*domain.PromotionPost
	var total int64

	offset := (page - 1) * limit

	// Use raw SQL with Window Function for per-advertiser post count limit
	query := `
		WITH active_advertisers AS (
			SELECT id, member_id, name, post_count, is_pinned
			FROM advertisers
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

	err := r.db.Raw(query, limit, offset).Scan(&posts).Error
	if err != nil {
		return nil, 0, err
	}

	// Count total posts
	countQuery := `
		WITH active_advertisers AS (
			SELECT id, post_count
			FROM advertisers
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

	r.db.Raw(countQuery).Scan(&total)

	return posts, total, nil
}

// GetPromotionPostsForInsert retrieves promotion posts for inserting into other boards
func (r *promotionRepository) GetPromotionPostsForInsert(count int) ([]*domain.PromotionPost, error) {
	var posts []*domain.PromotionPost

	// Get random posts from active advertisers
	query := `
		WITH active_advertisers AS (
			SELECT id, member_id, name, is_pinned
			FROM advertisers
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

	err := r.db.Raw(query, count).Scan(&posts).Error
	if err != nil {
		return nil, err
	}

	return posts, nil
}

// FindPromotionPostByID finds a promotion post by ID
func (r *promotionRepository) FindPromotionPostByID(id int64) (*domain.PromotionPost, error) {
	var post domain.PromotionPost

	// Join with advertiser to get author name
	query := `
		SELECT p.*, a.name as author_name, a.is_pinned
		FROM promotion_posts p
		JOIN advertisers a ON p.advertiser_id = a.id
		WHERE p.id = ?
	`

	err := r.db.Raw(query, id).Scan(&post).Error
	if err != nil {
		return nil, err
	}

	return &post, nil
}

// CreatePromotionPost creates a new promotion post
func (r *promotionRepository) CreatePromotionPost(post *domain.PromotionPost) error {
	return r.db.Create(post).Error
}

// UpdatePromotionPost updates a promotion post
func (r *promotionRepository) UpdatePromotionPost(post *domain.PromotionPost) error {
	return r.db.Save(post).Error
}

// DeletePromotionPost deletes a promotion post by ID
func (r *promotionRepository) DeletePromotionPost(id int64) error {
	return r.db.Delete(&domain.PromotionPost{}, id).Error
}

// IncrementViews increments the view count of a promotion post
func (r *promotionRepository) IncrementViews(id int64) error {
	return r.db.Model(&domain.PromotionPost{}).
		Where("id = ?", id).
		UpdateColumn("views", gorm.Expr("views + 1")).Error
}

// GetPostStatsByAdvertiser returns aggregated stats for an advertiser's posts
func (r *promotionRepository) GetPostStatsByAdvertiser(advertiserID int64) (totalViews int, totalLikes int, postCount int, err error) {
	var result struct {
		TotalViews int
		TotalLikes int
		PostCount  int
	}
	err = r.db.Model(&domain.PromotionPost{}).
		Select("COALESCE(SUM(views), 0) as total_views, COALESCE(SUM(likes), 0) as total_likes, COUNT(*) as post_count").
		Where("advertiser_id = ? AND is_active = ?", advertiserID, true).
		Scan(&result).Error
	return result.TotalViews, result.TotalLikes, result.PostCount, err
}
