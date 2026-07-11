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
	FindVisible(memberLevel int) ([]*v2.V2Board, error)
	FindAllIncludingInactive() ([]*v2.V2Board, error)
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

// FindVisible returns active boards a member of the given level may LIST.
// list_level 이 회원 레벨보다 높은 게시판(관리/비공개)은 목록에서 제외한다.
// memberLevel 0(비회원)이면 list_level 0 인 완전 공개 게시판만 반환.
func (r *boardRepository) FindVisible(memberLevel int) ([]*v2.V2Board, error) {
	var boards []*v2.V2Board
	err := r.db.Where("is_active = ? AND list_level <= ?", true, memberLevel).
		Order("order_num ASC").Find(&boards).Error
	return boards, err
}

func (r *boardRepository) FindAllIncludingInactive() ([]*v2.V2Board, error) {
	var boards []*v2.V2Board
	err := r.db.Order("order_num ASC").Find(&boards).Error
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
