package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// PostRepository v2 post data access
type PostRepository interface {
	FindByID(id uint64) (*v2.V2Post, error)
	FindByBoard(boardID uint64, page, limit int) ([]*v2.V2Post, int64, error)
	Create(post *v2.V2Post) error
	Update(post *v2.V2Post) error
	Delete(id uint64) error
	IncrementViewCount(id uint64) error
}

type postRepository struct {
	db *gorm.DB
}

// NewPostRepository creates a new v2 PostRepository
func NewPostRepository(db *gorm.DB) PostRepository {
	return &postRepository{db: db}
}

func (r *postRepository) FindByID(id uint64) (*v2.V2Post, error) {
	var post v2.V2Post
	err := r.db.Where("id = ? AND status != 'deleted'", id).First(&post).Error
	return &post, err
}

func (r *postRepository) FindByBoard(boardID uint64, page, limit int) ([]*v2.V2Post, int64, error) {
	var posts []*v2.V2Post
	var total int64

	query := r.db.Model(&v2.V2Post{}).Where("board_id = ? AND status = 'published'", boardID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	if err := query.Order("is_notice DESC, id DESC").Offset(offset).Limit(limit).Find(&posts).Error; err != nil {
		return nil, 0, err
	}
	return posts, total, nil
}

func (r *postRepository) Create(post *v2.V2Post) error {
	return r.db.Create(post).Error
}

func (r *postRepository) Update(post *v2.V2Post) error {
	return r.db.Save(post).Error
}

func (r *postRepository) Delete(id uint64) error {
	return r.db.Model(&v2.V2Post{}).Where("id = ?", id).Update("status", "deleted").Error
}

func (r *postRepository) IncrementViewCount(id uint64) error {
	return r.db.Model(&v2.V2Post{}).Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error
}
