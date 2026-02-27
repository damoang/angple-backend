package gnuboard

import (
	"fmt"
	"strings"

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
}

// WriteRepository provides access to g5_write_* dynamic tables
type WriteRepository interface {
	// Posts
	FindPosts(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error)
	FindPostByID(boardID string, wrID int) (*gnuboard.G5Write, error)
	FindNotices(boardID string, noticeIDs []int) ([]*gnuboard.G5Write, error)
	CreatePost(boardID string, post *gnuboard.G5Write) error
	UpdatePost(boardID string, post *gnuboard.G5Write) error
	DeletePost(boardID string, wrID int) error
	IncrementHit(boardID string, wrID int) error

	// Comments
	FindComments(boardID string, parentID int) ([]*gnuboard.G5Write, error)
	FindCommentByID(boardID string, wrID int) (*gnuboard.G5Write, error)
	CreateComment(boardID string, comment *gnuboard.G5Write) error
	DeleteComment(boardID string, wrID int) error

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

// FindPosts retrieves posts (not comments) from a board with pagination
func (r *writeRepository) FindPosts(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error) {
	var posts []*gnuboard.G5Write
	var total int64

	offset := (page - 1) * limit
	table := tableName(boardID)

	// Posts only (wr_is_comment = 0)
	countQuery := r.db.Table(table).Where("wr_is_comment = 0")
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Select only core columns to avoid errors with missing columns
	// Order by wr_num (descending for latest first), then wr_reply for threaded replies
	err := r.db.Table(table).
		Select(coreColumns).
		Where("wr_is_comment = 0").
		Order("wr_num, wr_reply").
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	return posts, total, err
}

// FindPostByID retrieves a single post by ID
func (r *writeRepository) FindPostByID(boardID string, wrID int) (*gnuboard.G5Write, error) {
	var post gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_id = ? AND wr_is_comment = 0", wrID).
		First(&post).Error
	return &post, err
}

// FindNotices retrieves notice posts by their IDs
func (r *writeRepository) FindNotices(boardID string, noticeIDs []int) ([]*gnuboard.G5Write, error) {
	if len(noticeIDs) == 0 {
		return []*gnuboard.G5Write{}, nil
	}

	var notices []*gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_id IN ? AND wr_is_comment = 0", noticeIDs).
		Order("wr_num, wr_reply").
		Find(&notices).Error
	return notices, err
}

// CreatePost creates a new post
func (r *writeRepository) CreatePost(boardID string, post *gnuboard.G5Write) error {
	return r.db.Table(tableName(boardID)).Create(post).Error
}

// UpdatePost updates an existing post
func (r *writeRepository) UpdatePost(boardID string, post *gnuboard.G5Write) error {
	return r.db.Table(tableName(boardID)).Save(post).Error
}

// DeletePost deletes a post (and potentially its comments)
func (r *writeRepository) DeletePost(boardID string, wrID int) error {
	table := tableName(boardID)
	// Delete comments first
	if err := r.db.Table(table).Where("wr_parent = ?", wrID).Delete(&gnuboard.G5Write{}).Error; err != nil {
		return err
	}
	// Delete the post
	return r.db.Table(table).Where("wr_id = ?", wrID).Delete(&gnuboard.G5Write{}).Error
}

// IncrementHit increments the view count for a post
func (r *writeRepository) IncrementHit(boardID string, wrID int) error {
	return r.db.Table(tableName(boardID)).
		Where("wr_id = ?", wrID).
		UpdateColumn("wr_hit", gorm.Expr("wr_hit + 1")).Error
}

// FindComments retrieves all comments for a post
func (r *writeRepository) FindComments(boardID string, parentID int) ([]*gnuboard.G5Write, error) {
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

// DeleteComment deletes a comment
func (r *writeRepository) DeleteComment(boardID string, wrID int) error {
	return r.db.Table(tableName(boardID)).
		Where("wr_id = ? AND wr_is_comment = 1", wrID).
		Delete(&gnuboard.G5Write{}).Error
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
