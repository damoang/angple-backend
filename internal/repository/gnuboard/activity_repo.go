package gnuboard

import (
	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

// MemberActivityRepository provides read access to the member_activity_feed and stats tables
type MemberActivityRepository interface {
	FindPostsByMember(mbID string, page, limit int) ([]gnuboard.MemberActivityFeed, int64, error)
	FindCommentsByMember(mbID string, page, limit int) ([]gnuboard.MemberActivityFeed, int64, error)
	GetBoardStats(mbID string) ([]gnuboard.MemberActivityStatsRow, error)
	FindPublicActivity(mbID string, limit int) ([]gnuboard.MemberActivityFeed, error)
}

type memberActivityRepository struct {
	db *gorm.DB
}

// NewMemberActivityRepository creates a new MemberActivityRepository
func NewMemberActivityRepository(db *gorm.DB) MemberActivityRepository {
	return &memberActivityRepository{db: db}
}

// FindPostsByMember returns paginated posts for a member from the read model
func (r *memberActivityRepository) FindPostsByMember(mbID string, page, limit int) ([]gnuboard.MemberActivityFeed, int64, error) {
	var total int64
	r.db.Model(&gnuboard.MemberActivityFeed{}).
		Where("member_id = ? AND activity_type = 1 AND is_deleted = 0", mbID).
		Count(&total)

	var items []gnuboard.MemberActivityFeed
	offset := (page - 1) * limit
	err := r.db.
		Where("member_id = ? AND activity_type = 1 AND is_deleted = 0", mbID).
		Order("source_created_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&items).Error

	return items, total, err
}

// FindCommentsByMember returns paginated comments for a member from the read model
func (r *memberActivityRepository) FindCommentsByMember(mbID string, page, limit int) ([]gnuboard.MemberActivityFeed, int64, error) {
	var total int64
	r.db.Model(&gnuboard.MemberActivityFeed{}).
		Where("member_id = ? AND activity_type = 2 AND is_deleted = 0", mbID).
		Count(&total)

	var items []gnuboard.MemberActivityFeed
	offset := (page - 1) * limit
	err := r.db.
		Where("member_id = ? AND activity_type = 2 AND is_deleted = 0", mbID).
		Order("source_created_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&items).Error

	return items, total, err
}

// GetBoardStats returns per-board activity counts for a member
func (r *memberActivityRepository) GetBoardStats(mbID string) ([]gnuboard.MemberActivityStatsRow, error) {
	var stats []gnuboard.MemberActivityStatsRow
	err := r.db.
		Where("member_id = ? AND (post_count > 0 OR comment_count > 0)", mbID).
		Order("post_count + comment_count DESC").
		Find(&stats).Error
	return stats, err
}

// FindPublicActivity returns recent public activity for a member (used in member profile)
func (r *memberActivityRepository) FindPublicActivity(mbID string, limit int) ([]gnuboard.MemberActivityFeed, error) {
	var items []gnuboard.MemberActivityFeed
	err := r.db.
		Where("member_id = ? AND is_public = 1 AND is_deleted = 0", mbID).
		Order("source_created_at DESC, id DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}
