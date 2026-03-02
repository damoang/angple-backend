package gnuboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

// coreColumns are the columns that exist in all g5_write_* tables
var coreColumns = []string{
	"wr_id", "wr_num", "wr_reply", "wr_parent", "wr_is_comment",
	"wr_comment", "wr_comment_reply", "ca_name", "wr_option",
	"wr_subject", "wr_content", "wr_link1", "wr_link2",
	"wr_link1_hit", "wr_link2_hit", "wr_hit", "wr_good", "wr_nogood",
	"mb_id", "wr_password", "wr_name", "wr_email", "wr_homepage",
	"wr_datetime", "wr_file", "wr_last", "wr_ip",
	"wr_10", // 이미지 URL (갤러리/메시지 썸네일)
	"wr_deleted_at", "wr_deleted_by", // Soft delete columns (마이그레이션된 테이블만)
}

// WriteRepository provides access to g5_write_* dynamic tables
type WriteRepository interface {
	// Posts
	FindPosts(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error)
	FindPostByID(boardID string, wrID int) (*gnuboard.G5Write, error)
	FindPostByIDIncludeDeleted(boardID string, wrID int) (*gnuboard.G5Write, error)
	FindNotices(boardID string, noticeIDs []int) ([]*gnuboard.G5Write, error)
	FindDeletedPosts(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error)
	CreatePost(boardID string, post *gnuboard.G5Write) error
	UpdatePost(boardID string, post *gnuboard.G5Write) error
	DeletePost(boardID string, wrID int) error
	SoftDeletePost(boardID string, wrID int, deletedBy string) error
	RestorePost(boardID string, wrID int) error
	IncrementHit(boardID string, wrID int) error

	// Comments
	FindComments(boardID string, parentID int) ([]*gnuboard.G5Write, error)
	FindCommentsIncludeDeleted(boardID string, parentID int) ([]*gnuboard.G5Write, error)
	FindCommentByID(boardID string, wrID int) (*gnuboard.G5Write, error)
	CreateComment(boardID string, comment *gnuboard.G5Write) error
	DeleteComment(boardID string, wrID int) error
	SoftDeleteComment(boardID string, wrID int, deletedBy string) error
	RestoreComment(boardID string, wrID int) error

	// Utility
	TableExists(boardID string) bool
	GetNextWrNum(boardID string) (int, error)
}

type writeRepository struct {
	db *gorm.DB
}

// NewWriteRepository creates a new Gnuboard WriteRepository
func NewWriteRepository(db *gorm.DB) WriteRepository {
	return &writeRepository{db: db}
}

// tableName generates the dynamic table name for a board
func tableName(boardID string) string {
	return fmt.Sprintf("g5_write_%s", boardID)
}

// FindPosts retrieves posts (not comments, not deleted) from a board with pagination
func (r *writeRepository) FindPosts(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error) {
	var posts []*gnuboard.G5Write
	var total int64

	offset := (page - 1) * limit
	table := tableName(boardID)

	// Posts only (wr_is_comment = 0), exclude soft deleted
	countQuery := r.db.Table(table).Where("wr_is_comment = 0 AND wr_deleted_at IS NULL")
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Select only core columns to avoid errors with missing columns
	// Order by wr_num (descending for latest first), then wr_reply for threaded replies
	err := r.db.Table(table).
		Select(coreColumns).
		Where("wr_is_comment = 0 AND wr_deleted_at IS NULL").
		Order("wr_num, wr_reply").
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	return posts, total, err
}

// FindPostByID retrieves a single post by ID (excludes soft deleted)
func (r *writeRepository) FindPostByID(boardID string, wrID int) (*gnuboard.G5Write, error) {
	var post gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL", wrID).
		First(&post).Error
	return &post, err
}

// FindPostByIDIncludeDeleted retrieves a single post by ID including soft deleted posts
func (r *writeRepository) FindPostByIDIncludeDeleted(boardID string, wrID int) (*gnuboard.G5Write, error) {
	var post gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_id = ? AND wr_is_comment = 0", wrID).
		First(&post).Error
	return &post, err
}

// FindNotices retrieves notice posts by their IDs (excludes soft deleted)
func (r *writeRepository) FindNotices(boardID string, noticeIDs []int) ([]*gnuboard.G5Write, error) {
	if len(noticeIDs) == 0 {
		return []*gnuboard.G5Write{}, nil
	}

	var notices []*gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_id IN ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL", noticeIDs).
		Order("wr_num, wr_reply").
		Find(&notices).Error
	return notices, err
}

// FindDeletedPosts retrieves soft deleted posts from a board with pagination (admin use)
func (r *writeRepository) FindDeletedPosts(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error) {
	var posts []*gnuboard.G5Write
	var total int64

	offset := (page - 1) * limit
	table := tableName(boardID)

	countQuery := r.db.Table(table).Where("wr_is_comment = 0 AND wr_deleted_at IS NOT NULL")
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.Table(table).
		Select(coreColumns).
		Where("wr_is_comment = 0 AND wr_deleted_at IS NOT NULL").
		Order("wr_deleted_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	return posts, total, err
}

// CreatePost creates a new post
func (r *writeRepository) CreatePost(boardID string, post *gnuboard.G5Write) error {
	return r.db.Table(tableName(boardID)).Create(post).Error
}

// UpdatePost updates an existing post
func (r *writeRepository) UpdatePost(boardID string, post *gnuboard.G5Write) error {
	return r.db.Table(tableName(boardID)).Save(post).Error
}

// DeletePost permanently deletes a post and its comments from the database
func (r *writeRepository) DeletePost(boardID string, wrID int) error {
	table := tableName(boardID)
	// Delete comments first
	if err := r.db.Table(table).Where("wr_parent = ?", wrID).Delete(&gnuboard.G5Write{}).Error; err != nil {
		return err
	}
	// Delete the post
	return r.db.Table(table).Where("wr_id = ?", wrID).Delete(&gnuboard.G5Write{}).Error
}

// SoftDeletePost marks a post and its comments as deleted
func (r *writeRepository) SoftDeletePost(boardID string, wrID int, deletedBy string) error {
	table := tableName(boardID)
	now := time.Now()

	// Soft delete the post
	if err := r.db.Table(table).Where("wr_id = ?", wrID).Updates(map[string]interface{}{
		"wr_deleted_at": now,
		"wr_deleted_by": deletedBy,
	}).Error; err != nil {
		return err
	}

	// Soft delete all comments for this post
	return r.db.Table(table).Where("wr_parent = ? AND wr_is_comment = 1", wrID).Updates(map[string]interface{}{
		"wr_deleted_at": now,
		"wr_deleted_by": deletedBy,
	}).Error
}

// RestorePost restores a soft deleted post and its comments
func (r *writeRepository) RestorePost(boardID string, wrID int) error {
	table := tableName(boardID)

	// Restore the post
	if err := r.db.Table(table).Where("wr_id = ?", wrID).Updates(map[string]interface{}{
		"wr_deleted_at": nil,
		"wr_deleted_by": nil,
	}).Error; err != nil {
		return err
	}

	// Restore all comments for this post
	return r.db.Table(table).Where("wr_parent = ?", wrID).Updates(map[string]interface{}{
		"wr_deleted_at": nil,
		"wr_deleted_by": nil,
	}).Error
}

// IncrementHit increments the view count for a post
func (r *writeRepository) IncrementHit(boardID string, wrID int) error {
	return r.db.Table(tableName(boardID)).
		Where("wr_id = ?", wrID).
		UpdateColumn("wr_hit", gorm.Expr("wr_hit + 1")).Error
}

// FindComments retrieves all non-deleted comments for a post
func (r *writeRepository) FindComments(boardID string, parentID int) ([]*gnuboard.G5Write, error) {
	var comments []*gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_parent = ? AND wr_is_comment = 1 AND wr_deleted_at IS NULL", parentID).
		Order("wr_comment, wr_comment_reply").
		Find(&comments).Error
	return comments, err
}

// FindCommentsIncludeDeleted retrieves all comments for a post including soft deleted ones
func (r *writeRepository) FindCommentsIncludeDeleted(boardID string, parentID int) ([]*gnuboard.G5Write, error) {
	var comments []*gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_parent = ? AND wr_is_comment = 1", parentID).
		Order("wr_comment, wr_comment_reply").
		Find(&comments).Error
	return comments, err
}

// FindCommentByID retrieves a single comment by ID
func (r *writeRepository) FindCommentByID(boardID string, wrID int) (*gnuboard.G5Write, error) {
	var comment gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_id = ? AND wr_is_comment = 1", wrID).
		First(&comment).Error
	return &comment, err
}

// CreateComment creates a new comment
func (r *writeRepository) CreateComment(boardID string, comment *gnuboard.G5Write) error {
	return r.db.Table(tableName(boardID)).Create(comment).Error
}

// DeleteComment permanently deletes a comment from the database
func (r *writeRepository) DeleteComment(boardID string, wrID int) error {
	return r.db.Table(tableName(boardID)).
		Where("wr_id = ? AND wr_is_comment = 1", wrID).
		Delete(&gnuboard.G5Write{}).Error
}

// SoftDeleteComment marks a comment as deleted
func (r *writeRepository) SoftDeleteComment(boardID string, wrID int, deletedBy string) error {
	now := time.Now()
	return r.db.Table(tableName(boardID)).
		Where("wr_id = ? AND wr_is_comment = 1", wrID).
		Updates(map[string]interface{}{
			"wr_deleted_at": now,
			"wr_deleted_by": deletedBy,
		}).Error
}

// RestoreComment restores a soft deleted comment
func (r *writeRepository) RestoreComment(boardID string, wrID int) error {
	return r.db.Table(tableName(boardID)).
		Where("wr_id = ? AND wr_is_comment = 1", wrID).
		Updates(map[string]interface{}{
			"wr_deleted_at": nil,
			"wr_deleted_by": nil,
		}).Error
}

// TableExists checks if the write table exists for a board
func (r *writeRepository) TableExists(boardID string) bool {
	table := tableName(boardID)
	var count int64
	// Check if table exists by querying INFORMATION_SCHEMA
	r.db.Raw("SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_NAME = ?", table).Scan(&count)
	return count > 0
}

// GetNextWrNum gets the next wr_num for a new post (negative, as per Gnuboard convention)
func (r *writeRepository) GetNextWrNum(boardID string) (int, error) {
	var minNum int
	err := r.db.Table(tableName(boardID)).
		Select("COALESCE(MIN(wr_num), 0)").
		Scan(&minNum).Error
	if err != nil {
		return 0, err
	}
	return minNum - 1, nil
}

// ParseNoticeIDs parses the bo_notice string into a slice of post IDs
func ParseNoticeIDs(noticeStr string) []int {
	if noticeStr == "" {
		return []int{}
	}

	parts := strings.Split(noticeStr, ",")
	ids := make([]int, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var id int
		if _, err := fmt.Sscanf(part, "%d", &id); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}

	return ids
}
