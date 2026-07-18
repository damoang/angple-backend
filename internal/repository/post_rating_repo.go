package repository

import (
	"errors"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PostRatingRepository handles post star rating data access.
type PostRatingRepository interface {
	AutoMigrate() error
	Upsert(boTable string, wrID int, mbID string, rating int) error
	Aggregate(boTable string, wrID int) (avg float64, count int64, err error)
	FindMyRating(boTable string, wrID int, mbID string) (int, error)
}

type postRatingRepository struct {
	db *gorm.DB
}

// NewPostRatingRepository creates a new PostRatingRepository.
func NewPostRatingRepository(db *gorm.DB) PostRatingRepository {
	return &postRatingRepository{db: db}
}

// AutoMigrate creates the angple_post_ratings table.
// ⛔ prod 는 수동 DDL 선행 원칙 — migration/012_post_ratings.up.sql 참고.
func (r *postRatingRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&domain.AnglePostRating{})
}

// Upsert inserts a rating or updates it on PK conflict (회원당 1표, 재투표=UPDATE).
func (r *postRatingRepository) Upsert(boTable string, wrID int, mbID string, rating int) error {
	record := domain.AnglePostRating{
		BoTable: boTable,
		WrID:    wrID,
		MbID:    mbID,
		Rating:  rating,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "bo_table"}, {Name: "wr_id"}, {Name: "mb_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"rating", "updated_at"}),
	}).Create(&record).Error
}

// Aggregate returns the average rating and vote count for a post.
func (r *postRatingRepository) Aggregate(boTable string, wrID int) (float64, int64, error) {
	var result struct {
		Avg   float64
		Count int64
	}
	err := r.db.Model(&domain.AnglePostRating{}).
		Select("COALESCE(AVG(rating), 0) as avg, COUNT(*) as count").
		Where("bo_table = ? AND wr_id = ?", boTable, wrID).
		Scan(&result).Error
	if err != nil {
		return 0, 0, err
	}
	return result.Avg, result.Count, nil
}

// FindMyRating returns the member's rating for a post (0 if not voted).
func (r *postRatingRepository) FindMyRating(boTable string, wrID int, mbID string) (int, error) {
	var record domain.AnglePostRating
	err := r.db.Where("bo_table = ? AND wr_id = ? AND mb_id = ?", boTable, wrID, mbID).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return record.Rating, nil
}
