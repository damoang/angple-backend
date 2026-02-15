package repository

import (
	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"gorm.io/gorm"
)

// MarketplaceRepository 마켓플레이스 데이터 접근
type MarketplaceRepository struct {
	db *gorm.DB
}

// NewMarketplaceRepository 생성자
func NewMarketplaceRepository(db *gorm.DB) *MarketplaceRepository {
	return &MarketplaceRepository{db: db}
}

// AutoMigrate 마켓플레이스 테이블 생성
func (r *MarketplaceRepository) AutoMigrate() error {
	return r.db.AutoMigrate(
		&domain.PluginDeveloper{},
		&domain.PluginSubmission{},
		&domain.PluginReview{},
		&domain.PluginDownload{},
	)
}

// === Developer ===

func (r *MarketplaceRepository) FindDeveloperByUserID(userID uint64) (*domain.PluginDeveloper, error) {
	var dev domain.PluginDeveloper
	err := r.db.Where("user_id = ?", userID).First(&dev).Error
	return &dev, err
}

func (r *MarketplaceRepository) FindDeveloperByID(id uint64) (*domain.PluginDeveloper, error) {
	var dev domain.PluginDeveloper
	err := r.db.First(&dev, id).Error
	return &dev, err
}

func (r *MarketplaceRepository) CreateDeveloper(dev *domain.PluginDeveloper) error {
	return r.db.Create(dev).Error
}

func (r *MarketplaceRepository) UpdateDeveloper(dev *domain.PluginDeveloper) error {
	return r.db.Save(dev).Error
}

// === Submission ===

func (r *MarketplaceRepository) CreateSubmission(sub *domain.PluginSubmission) error {
	return r.db.Create(sub).Error
}

func (r *MarketplaceRepository) FindSubmissionByID(id uint64) (*domain.PluginSubmission, error) {
	var sub domain.PluginSubmission
	err := r.db.First(&sub, id).Error
	return &sub, err
}

func (r *MarketplaceRepository) FindSubmissionsByDeveloper(devID uint64, page, perPage int) ([]domain.PluginSubmission, int64, error) {
	var subs []domain.PluginSubmission
	var total int64
	query := r.db.Where("developer_id = ?", devID)
	query.Model(&domain.PluginSubmission{}).Count(&total)
	err := query.Order("created_at DESC").Offset((page - 1) * perPage).Limit(perPage).Find(&subs).Error
	return subs, total, err
}

func (r *MarketplaceRepository) FindApprovedSubmissions(page, perPage int, category, keyword string) ([]domain.PluginSubmission, int64, error) {
	var subs []domain.PluginSubmission
	var total int64
	query := r.db.Where("status = ?", "approved")
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("title LIKE ? OR description LIKE ? OR plugin_name LIKE ?", like, like, like)
	}
	query.Model(&domain.PluginSubmission{}).Count(&total)
	err := query.Order("download_count DESC").Offset((page - 1) * perPage).Limit(perPage).Find(&subs).Error
	return subs, total, err
}

func (r *MarketplaceRepository) FindApprovedByName(pluginName string) (*domain.PluginSubmission, error) {
	var sub domain.PluginSubmission
	err := r.db.Where("plugin_name = ? AND status = ?", pluginName, "approved").
		Order("created_at DESC").First(&sub).Error
	return &sub, err
}

func (r *MarketplaceRepository) UpdateSubmission(sub *domain.PluginSubmission) error {
	return r.db.Save(sub).Error
}

// FindPendingSubmissions 관리자용: 대기 중인 제출 목록
func (r *MarketplaceRepository) FindPendingSubmissions(page, perPage int) ([]domain.PluginSubmission, int64, error) {
	var subs []domain.PluginSubmission
	var total int64
	query := r.db.Where("status = ?", "pending")
	query.Model(&domain.PluginSubmission{}).Count(&total)
	err := query.Order("created_at ASC").Offset((page - 1) * perPage).Limit(perPage).Find(&subs).Error
	return subs, total, err
}

// === Review ===

func (r *MarketplaceRepository) CreateReview(rev *domain.PluginReview) error {
	return r.db.Create(rev).Error
}

func (r *MarketplaceRepository) FindReviewsByPlugin(pluginName string, page, perPage int) ([]domain.PluginReview, int64, error) {
	var reviews []domain.PluginReview
	var total int64
	query := r.db.Where("plugin_name = ?", pluginName)
	query.Model(&domain.PluginReview{}).Count(&total)
	err := query.Order("created_at DESC").Offset((page - 1) * perPage).Limit(perPage).Find(&reviews).Error
	return reviews, total, err
}

func (r *MarketplaceRepository) GetAverageRating(pluginName string) (float64, int64, error) {
	var result struct {
		AvgRating float64
		Count     int64
	}
	err := r.db.Model(&domain.PluginReview{}).
		Select("COALESCE(AVG(rating), 0) as avg_rating, COUNT(*) as count").
		Where("plugin_name = ?", pluginName).
		Scan(&result).Error
	return result.AvgRating, result.Count, err
}

func (r *MarketplaceRepository) HasUserReviewed(pluginName string, userID uint64) (bool, error) {
	var count int64
	err := r.db.Model(&domain.PluginReview{}).
		Where("plugin_name = ? AND user_id = ?", pluginName, userID).Count(&count).Error
	return count > 0, err
}

// === Download ===

func (r *MarketplaceRepository) CreateDownload(dl *domain.PluginDownload) error {
	return r.db.Create(dl).Error
}

func (r *MarketplaceRepository) IncrementDownloadCount(pluginName string) error {
	return r.db.Model(&domain.PluginSubmission{}).
		Where("plugin_name = ? AND status = ?", pluginName, "approved").
		UpdateColumn("download_count", gorm.Expr("download_count + 1")).Error
}
