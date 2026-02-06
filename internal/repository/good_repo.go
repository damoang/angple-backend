package repository

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// GoodRepository defines the interface for recommend/downvote operations
type GoodRepository interface {
	HasGood(boTable string, wrID int, mbID string, flag string) (bool, error)
	AddGood(boTable string, wrID int, mbID string, flag string, ip string) error
	RemoveGood(boTable string, wrID int, mbID string, flag string) error
	GetGoodCount(boTable string, wrID int) (good int, nogood int, err error)
	GetWriteAuthorID(boTable string, wrID int) (string, error)
	GetLikers(boTable string, wrID int, page, limit int) (*domain.LikersResponse, error)
}

type goodRepository struct {
	db *gorm.DB
}

// NewGoodRepository creates a new GoodRepository
func NewGoodRepository(db *gorm.DB) GoodRepository {
	return &goodRepository{db: db}
}

func (r *goodRepository) getWriteTableName(boTable string) string {
	return fmt.Sprintf("g5_write_%s", boTable)
}

// HasGood checks if a user has already recommended/downvoted
func (r *goodRepository) HasGood(boTable string, wrID int, mbID string, flag string) (bool, error) {
	var count int64
	err := r.db.Model(&domain.BoardGood{}).
		Where("bo_table = ? AND wr_id = ? AND mb_id = ? AND bg_flag = ?", boTable, wrID, mbID, flag).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddGood adds a recommend/downvote record and increments wr_good/wr_nogood in a transaction
func (r *goodRepository) AddGood(boTable string, wrID int, mbID string, flag string, ip string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Insert into g5_board_good
		good := &domain.BoardGood{
			BoTable:    boTable,
			WrID:       wrID,
			MbID:       mbID,
			BgFlag:     flag,
			BgDatetime: time.Now(),
			BgIP:       ip,
		}
		if err := tx.Create(good).Error; err != nil {
			return err
		}

		// Increment wr_good or wr_nogood in the write table
		column := "wr_good"
		if flag == "nogood" {
			column = "wr_nogood"
		}
		tableName := r.getWriteTableName(boTable)
		return tx.Table(tableName).
			Where("wr_id = ?", wrID).
			UpdateColumn(column, gorm.Expr(column+" + 1")).Error
	})
}

// RemoveGood removes a recommend/downvote record and decrements wr_good/wr_nogood in a transaction
func (r *goodRepository) RemoveGood(boTable string, wrID int, mbID string, flag string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete from g5_board_good
		result := tx.Where("bo_table = ? AND wr_id = ? AND mb_id = ? AND bg_flag = ?", boTable, wrID, mbID, flag).
			Delete(&domain.BoardGood{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}

		// Decrement wr_good or wr_nogood in the write table
		column := "wr_good"
		if flag == "nogood" {
			column = "wr_nogood"
		}
		tableName := r.getWriteTableName(boTable)
		return tx.Table(tableName).
			Where("wr_id = ?", wrID).
			UpdateColumn(column, gorm.Expr("GREATEST("+column+" - 1, 0)")).Error
	})
}

// GetGoodCount returns the current good and nogood counts from the write table
func (r *goodRepository) GetGoodCount(boTable string, wrID int) (good int, nogood int, err error) {
	tableName := r.getWriteTableName(boTable)
	var result struct {
		Good   int `gorm:"column:wr_good"`
		Nogood int `gorm:"column:wr_nogood"`
	}
	err = r.db.Table(tableName).
		Select("wr_good, wr_nogood").
		Where("wr_id = ?", wrID).
		Scan(&result).Error
	if err != nil {
		return 0, 0, err
	}
	return result.Good, result.Nogood, nil
}

// GetWriteAuthorID returns the author ID (mb_id) of a write record (post or comment)
func (r *goodRepository) GetWriteAuthorID(boTable string, wrID int) (string, error) {
	tableName := r.getWriteTableName(boTable)
	var result struct {
		AuthorID string `gorm:"column:mb_id"`
	}
	err := r.db.Table(tableName).
		Select("mb_id").
		Where("wr_id = ?", wrID).
		Scan(&result).Error
	if err != nil {
		return "", err
	}
	return result.AuthorID, nil
}

// GetLikers returns the list of users who liked a post
func (r *goodRepository) GetLikers(boTable string, wrID int, page, limit int) (*domain.LikersResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Get total count
	var total int64
	if err := r.db.Model(&domain.BoardGood{}).
		Where("bo_table = ? AND wr_id = ? AND bg_flag = ?", boTable, wrID, "good").
		Count(&total).Error; err != nil {
		return nil, err
	}

	// Get likers with member info
	var likers []struct {
		MbID       string    `gorm:"column:mb_id"`
		MbName     string    `gorm:"column:mb_name"`
		MbNick     string    `gorm:"column:mb_nick"`
		BgIP       string    `gorm:"column:bg_ip"`
		BgDatetime time.Time `gorm:"column:bg_datetime"`
	}

	err := r.db.Table("g5_board_good AS bg").
		Select("bg.mb_id, COALESCE(m.mb_name, bg.mb_id) AS mb_name, COALESCE(m.mb_nick, m.mb_name, bg.mb_id) AS mb_nick, bg.bg_ip, bg.bg_datetime").
		Joins("LEFT JOIN g5_member AS m ON bg.mb_id = m.mb_id").
		Where("bg.bo_table = ? AND bg.wr_id = ? AND bg.bg_flag = ?", boTable, wrID, "good").
		Order("bg.bg_datetime DESC").
		Offset(offset).
		Limit(limit).
		Scan(&likers).Error
	if err != nil {
		return nil, err
	}

	result := &domain.LikersResponse{
		Likers: make([]domain.LikerInfo, len(likers)),
		Total:  int(total),
	}

	for i, liker := range likers {
		result.Likers[i] = domain.LikerInfo{
			MbID:    liker.MbID,
			MbName:  liker.MbName,
			MbNick:  liker.MbNick,
			BgIP:    maskIP(liker.BgIP),
			LikedAt: liker.BgDatetime.Format("2006-01-02 15:04:05"),
		}
	}

	return result, nil
}
