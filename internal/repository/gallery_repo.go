package repository

import (
	"fmt"
	"strings"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// GalleryRepository handles gallery and unified search queries
type GalleryRepository struct {
	db *gorm.DB
}

// NewGalleryRepository creates a new GalleryRepository
func NewGalleryRepository(db *gorm.DB) *GalleryRepository {
	return &GalleryRepository{db: db}
}

// FindGalleryPosts returns posts with images from the specified board
func (r *GalleryRepository) FindGalleryPosts(boardID string, page, limit int) ([]*domain.GalleryItem, int64, error) {
	tableName := fmt.Sprintf("g5_write_%s", boardID)
	offset := (page - 1) * limit

	var total int64
	countSQL := fmt.Sprintf(
		"SELECT COUNT(*) FROM `%s` WHERE wr_is_comment = 0 AND wr_parent = wr_id AND wr_file > 0",
		tableName,
	)
	if err := r.db.Raw(countSQL).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	type postRow struct {
		WrID       int    `gorm:"column:wr_id"`
		WrSubject  string `gorm:"column:wr_subject"`
		WrName     string `gorm:"column:wr_name"`
		MbID       string `gorm:"column:mb_id"`
		WrHit      int    `gorm:"column:wr_hit"`
		WrGood     int    `gorm:"column:wr_good"`
		WrComment  int    `gorm:"column:wr_comment"`
		WrDatetime string `gorm:"column:wr_datetime"`
	}

	var rows []postRow
	sql := fmt.Sprintf(
		"SELECT wr_id, wr_subject, wr_name, mb_id, wr_hit, wr_good, wr_comment, wr_datetime "+
			"FROM `%s` WHERE wr_is_comment = 0 AND wr_parent = wr_id AND wr_file > 0 "+
			"ORDER BY wr_datetime DESC LIMIT ? OFFSET ?",
		tableName,
	)
	if err := r.db.Raw(sql, limit, offset).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	items := make([]*domain.GalleryItem, len(rows))
	for i, row := range rows {
		// Get first image file for thumbnail
		thumbnail := r.getFirstImage(boardID, row.WrID)
		items[i] = &domain.GalleryItem{
			BoardID:      boardID,
			PostID:       row.WrID,
			Title:        row.WrSubject,
			Author:       row.WrName,
			AuthorID:     row.MbID,
			ThumbnailURL: thumbnail,
			Views:        row.WrHit,
			Likes:        row.WrGood,
			CommentCount: row.WrComment,
			CreatedAt:    row.WrDatetime,
		}
	}

	return items, total, nil
}

// FindGalleryAll returns image posts across all boards (최신순)
func (r *GalleryRepository) FindGalleryAll(boardIDs []string, page, limit int) ([]*domain.GalleryItem, int64, error) {
	if len(boardIDs) == 0 {
		return nil, 0, nil
	}

	offset := (page - 1) * limit

	// Build UNION ALL for count
	var countParts []string
	for _, bid := range boardIDs {
		countParts = append(countParts, fmt.Sprintf(
			"SELECT wr_id FROM `g5_write_%s` WHERE wr_is_comment = 0 AND wr_parent = wr_id AND wr_file > 0",
			bid,
		))
	}
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS t", strings.Join(countParts, " UNION ALL "))
	var total int64
	if err := r.db.Raw(countSQL).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	// Build UNION ALL for data
	var dataParts []string
	for _, bid := range boardIDs {
		dataParts = append(dataParts, fmt.Sprintf(
			"SELECT '%s' AS bo_table, wr_id, wr_subject, wr_name, mb_id, wr_hit, wr_good, wr_comment, wr_datetime "+
				"FROM `g5_write_%s` WHERE wr_is_comment = 0 AND wr_parent = wr_id AND wr_file > 0",
			bid, bid,
		))
	}
	dataSQL := fmt.Sprintf(
		"SELECT * FROM (%s) AS t ORDER BY wr_datetime DESC LIMIT ? OFFSET ?",
		strings.Join(dataParts, " UNION ALL "),
	)

	type galleryRow struct {
		BoTable    string `gorm:"column:bo_table"`
		WrID       int    `gorm:"column:wr_id"`
		WrSubject  string `gorm:"column:wr_subject"`
		WrName     string `gorm:"column:wr_name"`
		MbID       string `gorm:"column:mb_id"`
		WrHit      int    `gorm:"column:wr_hit"`
		WrGood     int    `gorm:"column:wr_good"`
		WrComment  int    `gorm:"column:wr_comment"`
		WrDatetime string `gorm:"column:wr_datetime"`
	}

	var rows []galleryRow
	if err := r.db.Raw(dataSQL, limit, offset).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	items := make([]*domain.GalleryItem, len(rows))
	for i, row := range rows {
		thumbnail := r.getFirstImage(row.BoTable, row.WrID)
		items[i] = &domain.GalleryItem{
			BoardID:      row.BoTable,
			PostID:       row.WrID,
			Title:        row.WrSubject,
			Author:       row.WrName,
			AuthorID:     row.MbID,
			ThumbnailURL: thumbnail,
			Views:        row.WrHit,
			Likes:        row.WrGood,
			CommentCount: row.WrComment,
			CreatedAt:    row.WrDatetime,
		}
	}

	return items, total, nil
}

// UnifiedSearch searches across all boards with UNION ALL
func (r *GalleryRepository) UnifiedSearch(boardIDs []string, keyword string, page, limit int) ([]*domain.UnifiedSearchResult, int64, error) {
	if len(boardIDs) == 0 || keyword == "" {
		return nil, 0, nil
	}

	offset := (page - 1) * limit
	likeKeyword := "%" + keyword + "%"

	// Count
	var countParts []string
	for _, bid := range boardIDs {
		countParts = append(countParts, fmt.Sprintf(
			"SELECT wr_id FROM `g5_write_%s` WHERE wr_is_comment = 0 AND wr_parent = wr_id AND (wr_subject LIKE ? OR wr_content LIKE ?)",
			bid,
		))
	}
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS t", strings.Join(countParts, " UNION ALL "))

	// Build args for count (2 args per board)
	countArgs := make([]interface{}, 0, len(boardIDs)*2)
	for range boardIDs {
		countArgs = append(countArgs, likeKeyword, likeKeyword)
	}

	var total int64
	if err := r.db.Raw(countSQL, countArgs...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	// Data
	var dataParts []string
	for _, bid := range boardIDs {
		dataParts = append(dataParts, fmt.Sprintf(
			"SELECT '%s' AS bo_table, wr_id, wr_subject, SUBSTRING(wr_content, 1, 200) AS wr_content, wr_name, mb_id, wr_hit, wr_good, wr_datetime "+
				"FROM `g5_write_%s` WHERE wr_is_comment = 0 AND wr_parent = wr_id AND (wr_subject LIKE ? OR wr_content LIKE ?)",
			bid, bid,
		))
	}
	dataSQL := fmt.Sprintf(
		"SELECT * FROM (%s) AS t ORDER BY wr_datetime DESC LIMIT ? OFFSET ?",
		strings.Join(dataParts, " UNION ALL "),
	)

	dataArgs := make([]interface{}, 0, len(boardIDs)*2+2)
	for range boardIDs {
		dataArgs = append(dataArgs, likeKeyword, likeKeyword)
	}
	dataArgs = append(dataArgs, limit, offset)

	type searchRow struct {
		BoTable    string `gorm:"column:bo_table"`
		WrID       int    `gorm:"column:wr_id"`
		WrSubject  string `gorm:"column:wr_subject"`
		WrContent  string `gorm:"column:wr_content"`
		WrName     string `gorm:"column:wr_name"`
		MbID       string `gorm:"column:mb_id"`
		WrHit      int    `gorm:"column:wr_hit"`
		WrGood     int    `gorm:"column:wr_good"`
		WrDatetime string `gorm:"column:wr_datetime"`
	}

	var rows []searchRow
	if err := r.db.Raw(dataSQL, dataArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	results := make([]*domain.UnifiedSearchResult, len(rows))
	for i, row := range rows {
		results[i] = &domain.UnifiedSearchResult{
			BoardID:   row.BoTable,
			PostID:    row.WrID,
			Title:     row.WrSubject,
			Content:   row.WrContent,
			Author:    row.WrName,
			AuthorID:  row.MbID,
			Views:     row.WrHit,
			Likes:     row.WrGood,
			CreatedAt: row.WrDatetime,
		}
	}

	return results, total, nil
}

// getFirstImage returns the URL of the first image file for a post
func (r *GalleryRepository) getFirstImage(boardID string, wrID int) string {
	var file domain.BoardFile
	err := r.db.Where("bo_table = ? AND wr_id = ? AND bf_no = 0", boardID, wrID).
		First(&file).Error
	if err != nil {
		return ""
	}
	if file.File != "" {
		return fmt.Sprintf("/data/file/%s/%s", boardID, file.File)
	}
	return ""
}

// GetAllBoardIDs returns all active board IDs
func (r *GalleryRepository) GetAllBoardIDs() ([]string, error) {
	var boardIDs []string
	if err := r.db.Table("g5_board").
		Select("bo_table").
		Where("bo_use = 1").
		Pluck("bo_table", &boardIDs).Error; err != nil {
		return nil, err
	}
	return boardIDs, nil
}
