package gnuboard

import (
	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

// BoardRepository provides access to g5_board table
type BoardRepository interface {
	FindByID(boardID string) (*gnuboard.G5Board, error)
	FindAll() ([]*gnuboard.G5Board, error)
	FindByGroupID(groupID string) ([]*gnuboard.G5Board, error)
	Exists(boardID string) bool
}

type boardRepository struct {
	db *gorm.DB
}

// NewBoardRepository creates a new Gnuboard BoardRepository
func NewBoardRepository(db *gorm.DB) BoardRepository {
	return &boardRepository{db: db}
}

// FindByID finds a board by its table name (bo_table)
func (r *boardRepository) FindByID(boardID string) (*gnuboard.G5Board, error) {
	var board gnuboard.G5Board
	err := r.db.Where("bo_table = ?", boardID).First(&board).Error
	if err != nil {
		return nil, err
	}
	return &board, nil
}

// FindAll returns all boards ordered by bo_order
func (r *boardRepository) FindAll() ([]*gnuboard.G5Board, error) {
	var boards []*gnuboard.G5Board
	err := r.db.Order("bo_order ASC, bo_table ASC").Find(&boards).Error
	return boards, err
}

// FindByGroupID returns all boards in a specific group
func (r *boardRepository) FindByGroupID(groupID string) ([]*gnuboard.G5Board, error) {
	var boards []*gnuboard.G5Board
	err := r.db.Where("gr_id = ?", groupID).Order("bo_order ASC, bo_table ASC").Find(&boards).Error
	return boards, err
}

// Exists checks if a board table exists
func (r *boardRepository) Exists(boardID string) bool {
	var count int64
	r.db.Model(&gnuboard.G5Board{}).Where("bo_table = ?", boardID).Count(&count)
	return count > 0
}
