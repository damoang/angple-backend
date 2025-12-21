package repository

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

type CommentRepository interface {
	// List comments for a post
	ListByPost(boardID string, postID int) ([]*domain.Comment, error)

	// Find comment by ID
	FindByID(boardID string, id int) (*domain.Comment, error)

	// Create new comment
	Create(boardID string, comment *domain.Comment) error

	// Update comment
	Update(boardID string, id int, comment *domain.Comment) error

	// Delete comment
	Delete(boardID string, id int) error
}

type commentRepository struct {
	db *gorm.DB
}

func NewCommentRepository(db *gorm.DB) CommentRepository {
	return &commentRepository{db: db}
}

func (r *commentRepository) getTableName(boardID string) string {
	return fmt.Sprintf("g5_write_%s", boardID)
}

// ListByPost retrieves all comments for a post
func (r *commentRepository) ListByPost(boardID string, postID int) ([]*domain.Comment, error) {
	tableName := r.getTableName(boardID)
	var comments []*domain.Comment

	err := r.db.Table(tableName).
		Where("wr_parent = ?", postID).
		Where("wr_is_comment = ?", 1).
		Order("wr_id ASC").
		Find(&comments).Error

	return comments, err
}

// FindByID retrieves a comment by ID
func (r *commentRepository) FindByID(boardID string, id int) (*domain.Comment, error) {
	tableName := r.getTableName(boardID)
	var comment domain.Comment

	err := r.db.Table(tableName).
		Where("wr_id = ?", id).
		Where("wr_is_comment = ?", 1).
		First(&comment).Error

	return &comment, err
}

// Create creates a new comment
func (r *commentRepository) Create(boardID string, comment *domain.Comment) error {
	tableName := r.getTableName(boardID)

	// Set default values
	comment.CreatedAt = time.Now()
	comment.IsComment = 1 // Mark as comment
	comment.CommentCount = 0
	comment.Views = 0
	comment.Likes = 0
	comment.Dislikes = 0

	// Required fields - empty strings
	if comment.Reply == "" {
		comment.Reply = ""
	}
	if comment.CommentReply == "" {
		comment.CommentReply = ""
	}
	if comment.Option == "" {
		comment.Option = ""
	}
	if comment.Link1 == "" {
		comment.Link1 = ""
	}
	if comment.Link2 == "" {
		comment.Link2 = ""
	}
	if comment.Email == "" {
		comment.Email = ""
	}
	if comment.Homepage == "" {
		comment.Homepage = ""
	}
	if comment.LastUpdated == "" {
		comment.LastUpdated = ""
	}
	if comment.IP == "" {
		comment.IP = ""
	}
	if comment.FacebookUser == "" {
		comment.FacebookUser = ""
	}
	if comment.TwitterUser == "" {
		comment.TwitterUser = ""
	}

	// Extra fields
	comment.Extra1 = ""
	comment.Extra2 = ""
	comment.Extra3 = ""
	comment.Extra4 = ""
	comment.Extra5 = ""
	comment.Extra6 = ""
	comment.Extra7 = ""
	comment.Extra8 = ""
	comment.Extra9 = ""
	comment.Extra10 = ""

	// Not used for comments
	comment.Title = ""
	comment.Category = ""
	comment.SEOTitle = ""

	// wr_num is not important for comments
	comment.Num = 0

	return r.db.Table(tableName).
		Select("*").
		Create(comment).Error
}

// Update updates a comment
func (r *commentRepository) Update(boardID string, id int, comment *domain.Comment) error {
	tableName := r.getTableName(boardID)

	updates := map[string]interface{}{
		"wr_content": comment.Content,
	}

	return r.db.Table(tableName).
		Where("wr_id = ?", id).
		Where("wr_is_comment = ?", 1).
		Updates(updates).Error
}

// Delete deletes a comment
func (r *commentRepository) Delete(boardID string, id int) error {
	tableName := r.getTableName(boardID)

	return r.db.Table(tableName).
		Where("wr_id = ?", id).
		Where("wr_is_comment = ?", 1).
		Delete(&domain.Comment{}).Error
}
