package gnuboard

import (
	"sync"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

var boardByIDCache sync.Map

type cachedBoard struct {
	board     *gnuboard.G5Board
	expiresAt time.Time
}

const boardByIDCacheTTL = 60 * time.Second

// BoardRepository provides access to g5_board table
type BoardRepository interface {
	FindByID(boardID string) (*gnuboard.G5Board, error)
	FindAll() ([]*gnuboard.G5Board, error)
	FindByGroupID(groupID string) ([]*gnuboard.G5Board, error)
	Exists(boardID string) bool
	Create(board *gnuboard.G5Board) error
	Update(board *gnuboard.G5Board) error
	Delete(boardID string) error
	GetDB() *gorm.DB
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
	now := time.Now()
	if cached, ok := boardByIDCache.Load(boardID); ok {
		if entry, valid := cached.(*cachedBoard); valid && now.Before(entry.expiresAt) && entry.board != nil {
			boardCopy := *entry.board
			return &boardCopy, nil
		}
		boardByIDCache.Delete(boardID)
	}

	var board gnuboard.G5Board
	err := r.db.Where("bo_table = ?", boardID).First(&board).Error
	if err != nil {
		return nil, err
	}

	boardCopy := board
	boardByIDCache.Store(boardID, &cachedBoard{
		board:     &boardCopy,
		expiresAt: now.Add(boardByIDCacheTTL),
	})

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

// Create creates a new board in g5_board
func (r *boardRepository) Create(board *gnuboard.G5Board) error {
	if err := r.db.Create(board).Error; err != nil {
		return err
	}
	boardByIDCache.Delete(board.BoTable)
	return nil
}

// Update updates a board in g5_board
func (r *boardRepository) Update(board *gnuboard.G5Board) error {
	if err := r.db.Save(board).Error; err != nil {
		return err
	}
	boardByIDCache.Delete(board.BoTable)
	return nil
}

// Delete deletes a board from g5_board
func (r *boardRepository) Delete(boardID string) error {
	if err := r.db.Where("bo_table = ?", boardID).Delete(&gnuboard.G5Board{}).Error; err != nil {
		return err
	}
	boardByIDCache.Delete(boardID)
	return nil
}

// GetDB returns the underlying DB instance for raw operations
func (r *boardRepository) GetDB() *gorm.DB {
	return r.db
}
