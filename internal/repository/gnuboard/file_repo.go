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

// IncrementDownloadCount increases the download count for a file
func (r *FileRepository) IncrementDownloadCount(boardID string, postID int, fileNo int) error {
	return r.db.Model(&gnuboard.G5BoardFile{}).
		Where("bo_table = ? AND wr_id = ? AND bf_no = ?", boardID, postID, fileNo).
		UpdateColumn("bf_download", gorm.Expr("bf_download + 1")).
		Error
}
