package gnuboard

import (
	"github.com/damoang/angple-backend/internal/domain/gnuboard"

	"gorm.io/gorm"
)

// FileRepository handles file attachment database operations
type FileRepository struct {
	db *gorm.DB
}

// NewFileRepository creates a new file repository
func NewFileRepository(db *gorm.DB) *FileRepository {
	return &FileRepository{db: db}
}

// GetFilesByPost retrieves all files attached to a specific post
func (r *FileRepository) GetFilesByPost(boardID string, postID int) ([]gnuboard.G5BoardFile, error) {
	var files []gnuboard.G5BoardFile
	err := r.db.Where("bo_table = ? AND wr_id = ?", boardID, postID).
		Order("bf_no ASC").
		Find(&files).Error
	return files, err
}

// GetFile retrieves a specific file by board, post, and file number
func (r *FileRepository) GetFile(boardID string, postID int, fileNo int) (*gnuboard.G5BoardFile, error) {
	var file gnuboard.G5BoardFile
	err := r.db.Where("bo_table = ? AND wr_id = ? AND bf_no = ?", boardID, postID, fileNo).
		First(&file).Error
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// CreateFile inserts a new file record into g5_board_file
func (r *FileRepository) CreateFile(file *gnuboard.G5BoardFile) error {
	return r.db.Create(file).Error
}

// CreateFiles inserts multiple file records into g5_board_file
func (r *FileRepository) CreateFiles(files []gnuboard.G5BoardFile) error {
	if len(files) == 0 {
		return nil
	}
	return r.db.Create(&files).Error
}

// DeleteFilesByPost removes all file records for a specific post
func (r *FileRepository) DeleteFilesByPost(boardID string, postID int) error {
	return r.db.Where("bo_table = ? AND wr_id = ?", boardID, postID).Delete(&gnuboard.G5BoardFile{}).Error
}

// GetMaxBfNo returns the current max bf_no for a post (for determining next file number)
func (r *FileRepository) GetMaxBfNo(boardID string, postID int) (int, error) {
	var maxNo *int
	err := r.db.Model(&gnuboard.G5BoardFile{}).
		Where("bo_table = ? AND wr_id = ?", boardID, postID).
		Select("MAX(bf_no)").
		Scan(&maxNo).Error
	if err != nil {
		return -1, err
	}
	if maxNo == nil {
		return -1, nil
	}
	return *maxNo, nil
}

// IncrementDownloadCount increases the download count for a file
func (r *FileRepository) IncrementDownloadCount(boardID string, postID int, fileNo int) error {
	return r.db.Model(&gnuboard.G5BoardFile{}).
		Where("bo_table = ? AND wr_id = ? AND bf_no = ?", boardID, postID, fileNo).
		UpdateColumn("bf_download", gorm.Expr("bf_download + 1")).
		Error
}

// GetFirstImagesByPostIDs retrieves the first image file for each post in a single query.
// Returns a map of postID -> bf_file (stored filename).
func (r *FileRepository) GetFirstImagesByPostIDs(boardID string, postIDs []int) (map[int]string, error) {
	if len(postIDs) == 0 {
		return nil, nil
	}

	type row struct {
		WrID   int    `gorm:"column:wr_id"`
		BfFile string `gorm:"column:bf_file"`
	}

	var rows []row
	// Subquery: for each wr_id, get the minimum bf_no among image files
	err := r.db.Table("g5_board_file AS f").
		Select("f.wr_id, f.bf_file").
		Where("f.bo_table = ? AND f.wr_id IN ?", boardID, postIDs).
		Where("(f.bf_type = 1 OR f.bf_width > 0 OR LOWER(SUBSTRING_INDEX(f.bf_source, '.', -1)) IN ('jpg','jpeg','png','gif','webp','avif','svg','bmp'))").
		Where("f.bf_no = (?)",
			r.db.Table("g5_board_file AS f2").
				Select("MIN(f2.bf_no)").
				Where("f2.bo_table = f.bo_table AND f2.wr_id = f.wr_id").
				Where("(f2.bf_type = 1 OR f2.bf_width > 0 OR LOWER(SUBSTRING_INDEX(f2.bf_source, '.', -1)) IN ('jpg','jpeg','png','gif','webp','avif','svg','bmp'))"),
		).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make(map[int]string, len(rows))
	for _, r := range rows {
		result[r.WrID] = r.BfFile
	}
	return result, nil
}
