package repository

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// ScrapRepository scrap data access interface
type ScrapRepository interface {
	Create(mbID string, boTable string, wrID int) (*domain.Scrap, error)
	Delete(mbID string, boTable string, wrID int) error
	FindByMember(mbID string, page, limit int) ([]*domain.Scrap, int64, error)
	Exists(mbID string, boTable string, wrID int) (bool, error)
	GetPostTitle(boTable string, wrID int) (string, string, error) // title, author
}

type scrapRepository struct {
	db *gorm.DB
}

// NewScrapRepository creates a new ScrapRepository
func NewScrapRepository(db *gorm.DB) ScrapRepository {
	return &scrapRepository{db: db}
}

// Create adds a scrap
func (r *scrapRepository) Create(mbID string, boTable string, wrID int) (*domain.Scrap, error) {
	scrap := &domain.Scrap{
		MbID:     mbID,
		BoTable:  boTable,
		WrID:     wrID,
		DateTime: time.Now(),
	}
	if err := r.db.Create(scrap).Error; err != nil {
		return nil, err
	}

	// mb_scrap_cnt 증가
	r.db.Table("g5_member").Where("mb_id = ?", mbID).
		UpdateColumn("mb_scrap_cnt", gorm.Expr("mb_scrap_cnt + 1"))

	return scrap, nil
}

// Delete removes a scrap
func (r *scrapRepository) Delete(mbID string, boTable string, wrID int) error {
	result := r.db.Where("mb_id = ? AND bo_table = ? AND wr_id = ?", mbID, boTable, wrID).
		Delete(&domain.Scrap{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("스크랩을 찾을 수 없습니다")
	}

	// mb_scrap_cnt 감소
	r.db.Table("g5_member").Where("mb_id = ?", mbID).
		UpdateColumn("mb_scrap_cnt", gorm.Expr("GREATEST(mb_scrap_cnt - 1, 0)"))

	return nil
}

// FindByMember returns scraps for a member with pagination
func (r *scrapRepository) FindByMember(mbID string, page, limit int) ([]*domain.Scrap, int64, error) {
	var scraps []*domain.Scrap
	var total int64

	r.db.Model(&domain.Scrap{}).Where("mb_id = ?", mbID).Count(&total)

	offset := (page - 1) * limit
	err := r.db.Where("mb_id = ?", mbID).
		Order("ms_id DESC").
		Offset(offset).Limit(limit).
		Find(&scraps).Error
	return scraps, total, err
}

// Exists checks if a scrap already exists
func (r *scrapRepository) Exists(mbID string, boTable string, wrID int) (bool, error) {
	var count int64
	err := r.db.Model(&domain.Scrap{}).
		Where("mb_id = ? AND bo_table = ? AND wr_id = ?", mbID, boTable, wrID).
		Count(&count).Error
	return count > 0, err
}

// GetPostTitle returns the title and author from a write table
func (r *scrapRepository) GetPostTitle(boTable string, wrID int) (string, string, error) {
	tableName := fmt.Sprintf("g5_write_%s", boTable)
	var result struct {
		Title  string `gorm:"column:wr_subject"`
		Author string `gorm:"column:wr_name"`
	}
	err := r.db.Table(tableName).Select("wr_subject, wr_name").
		Where("wr_id = ?", wrID).Scan(&result).Error
	if err != nil {
		return "", "", err
	}
	return result.Title, result.Author, nil
}
