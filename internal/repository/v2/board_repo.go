package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// BoardRepository v2 board data access
type BoardRepository interface {
	FindByID(id uint64) (*v2.V2Board, error)
	FindBySlug(slug string) (*v2.V2Board, error)
	FindAll() ([]*v2.V2Board, error)
	Create(board *v2.V2Board) error
	Update(board *v2.V2Board) error
	Delete(id uint64) error
}

type boardRepository struct {
	db *gorm.DB
}

// NewBoardRepository creates a new v2 BoardRepository
func NewBoardRepository(db *gorm.DB) BoardRepository {
	return &boardRepository{db: db}
}

func (r *boardRepository) FindByID(id uint64) (*v2.V2Board, error) {
	var board v2.V2Board
	err := r.db.Where("id = ?", id).First(&board).Error
	return &board, err
}

func (r *boardRepository) FindBySlug(slug string) (*v2.V2Board, error) {
	var board v2.V2Board
	err := r.db.Where("slug = ?", slug).First(&board).Error
	return &board, err
}

func (r *boardRepository) FindAll() ([]*v2.V2Board, error) {
	var boards []*v2.V2Board
	err := r.db.Where("is_active = ?", true).Order("order_num ASC").Find(&boards).Error
	return boards, err
}

func (r *boardRepository) Create(board *v2.V2Board) error {
	return r.db.Create(board).Error
}

func (r *boardRepository) Update(board *v2.V2Board) error {
	return r.db.Save(board).Error
}

func (r *boardRepository) Delete(id uint64) error {
	return r.db.Delete(&v2.V2Board{}, id).Error
}
