package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// CommentRepository v2 comment data access
type CommentRepository interface {
	FindByID(id uint64) (*v2.V2Comment, error)
	FindByPost(postID uint64, page, limit int) ([]*v2.V2Comment, int64, error)
	Create(comment *v2.V2Comment) error
	Update(comment *v2.V2Comment) error
	Delete(id uint64) error
	Count() (int64, error)
}

type commentRepository struct {
	db *gorm.DB
}

// NewCommentRepository creates a new v2 CommentRepository
func NewCommentRepository(db *gorm.DB) CommentRepository {
	return &commentRepository{db: db}
}

func (r *commentRepository) FindByID(id uint64) (*v2.V2Comment, error) {
	var comment v2.V2Comment
	err := r.db.Where("id = ? AND status = 'active'", id).First(&comment).Error
	return &comment, err
}

func (r *commentRepository) FindByPost(postID uint64, page, limit int) ([]*v2.V2Comment, int64, error) {
	var comments []*v2.V2Comment
	var total int64

	query := r.db.Model(&v2.V2Comment{}).Where("post_id = ? AND status = 'active'", postID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	if err := query.Order("id ASC").Offset(offset).Limit(limit).Find(&comments).Error; err != nil {
		return nil, 0, err
	}
	return comments, total, nil
}

func (r *commentRepository) Create(comment *v2.V2Comment) error {
	return r.db.Create(comment).Error
}

func (r *commentRepository) Update(comment *v2.V2Comment) error {
	return r.db.Save(comment).Error
}

func (r *commentRepository) Delete(id uint64) error {
	return r.db.Model(&v2.V2Comment{}).Where("id = ?", id).Update("status", "deleted").Error
}

func (r *commentRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&v2.V2Comment{}).Where("status = 'active'").Count(&count).Error
	return count, err
}
