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

	// IncrementLikes increases the like count by 1
	IncrementLikes(boardID string, id int) error

	// DecrementLikes decreases the like count by 1
	DecrementLikes(boardID string, id int) error

	// IncrementDislikes increases the dislike count by 1
	IncrementDislikes(boardID string, id int) error

	// DecrementDislikes decreases the dislike count by 1
	DecrementDislikes(boardID string, id int) error

	// GetNextCommentReply returns the next wr_comment_reply value for a reply
	GetNextCommentReply(boardID string, postID int, parentCommentReply string) (string, error)
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
// wr_comment_reply 순서로 정렬하여 대댓글 트리 구조 유지
func (r *commentRepository) ListByPost(boardID string, postID int) ([]*domain.Comment, error) {
	tableName := r.getTableName(boardID)
	var comments []*domain.Comment

	err := r.db.Table(tableName).
		Where("wr_parent = ?", postID).
		Where("wr_is_comment = ?", 1).
		Order("wr_comment_reply ASC, wr_id ASC").
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
	// CommentCount(depth)는 서비스에서 설정하므로 덮어쓰지 않음
	// 그누보드 호환: depth 0 = 원댓글, depth >= 1 = 대댓글
	comment.Views = 0
	comment.Likes = 0
	comment.Dislikes = 0

	// Required fields - empty strings
	if comment.Reply == "" {
		comment.Reply = ""
	}
	// CommentReply는 서비스에서 설정하므로 기본값 설정하지 않음
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

	// Num (wr_num)은 서비스에서 부모 댓글 ID로 설정하므로 덮어쓰지 않음

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

// IncrementLikes increases the like count by 1
func (r *commentRepository) IncrementLikes(boardID string, id int) error {
	tableName := r.getTableName(boardID)

	return r.db.Table(tableName).
		Where("wr_id = ?", id).
		Where("wr_is_comment = ?", 1).
		UpdateColumn("wr_good", gorm.Expr("wr_good + 1")).Error
}

// DecrementLikes decreases the like count by 1
func (r *commentRepository) DecrementLikes(boardID string, id int) error {
	tableName := r.getTableName(boardID)

	return r.db.Table(tableName).
		Where("wr_id = ?", id).
		Where("wr_is_comment = ?", 1).
		UpdateColumn("wr_good", gorm.Expr("GREATEST(wr_good - 1, 0)")).Error
}

// IncrementDislikes increases the dislike count by 1
func (r *commentRepository) IncrementDislikes(boardID string, id int) error {
	tableName := r.getTableName(boardID)

	return r.db.Table(tableName).
		Where("wr_id = ?", id).
		Where("wr_is_comment = ?", 1).
		UpdateColumn("wr_nogood", gorm.Expr("wr_nogood + 1")).Error
}

// DecrementDislikes decreases the dislike count by 1
func (r *commentRepository) DecrementDislikes(boardID string, id int) error {
	tableName := r.getTableName(boardID)

	return r.db.Table(tableName).
		Where("wr_id = ?", id).
		Where("wr_is_comment = ?", 1).
		UpdateColumn("wr_nogood", gorm.Expr("GREATEST(wr_nogood - 1, 0)")).Error
}

// GetNextCommentReply returns the next wr_comment_reply value for a reply
// 그누보드 대댓글 정렬 키 생성
// - 원댓글: wr_comment_reply = ""
// - 대댓글: wr_comment_reply = 부모의 wr_comment_reply + 2자리 숫자
// 예: "", "01", "02", "0101", "0102"
func (r *commentRepository) GetNextCommentReply(boardID string, postID int, parentCommentReply string) (string, error) {
	tableName := r.getTableName(boardID)

	// 부모의 wr_comment_reply 접두사로 시작하고, 길이가 부모+2인 댓글 중 가장 큰 값을 찾음
	targetLen := len(parentCommentReply) + 2
	var maxReply string

	err := r.db.Table(tableName).
		Select("MAX(wr_comment_reply)").
		Where("wr_parent = ?", postID).
		Where("wr_is_comment = ?", 1).
		Where("wr_comment_reply LIKE ?", parentCommentReply+"%").
		Where("LENGTH(wr_comment_reply) = ?", targetLen).
		Row().Scan(&maxReply)

	if err != nil || maxReply == "" {
		// 첫 대댓글
		return parentCommentReply + "01", nil
	}

	// 마지막 2자리를 추출하여 +1
	suffix := maxReply[len(maxReply)-2:]
	num := 0
	if _, err := fmt.Sscanf(suffix, "%02d", &num); err != nil {
		return parentCommentReply + "01", nil
	}
	num++

	return fmt.Sprintf("%s%02d", parentCommentReply, num), nil
}
