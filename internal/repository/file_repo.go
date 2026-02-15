package repository

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// FileRepository file data access interface
type FileRepository interface {
	FindByID(boardID string, wrID int, fileNo int) (*domain.BoardFile, error)
	Create(file *domain.BoardFile) error
	IncrementDownload(boardID string, wrID int, fileNo int) error
	GetNextFileNo(boardID string, wrID int) (int, error)
	// FindFirstFileURLs returns a map of postID -> first file URL for the given post IDs
	// Uses bf_no=0 (first attached file) and prefers bf_fileurl over constructing from bf_file
	FindFirstFileURLs(boardID string, postIDs []int) (map[int]string, error)
}

type fileRepository struct {
	db *gorm.DB
}

// NewFileRepository creates a new FileRepository
func NewFileRepository(db *gorm.DB) FileRepository {
	return &fileRepository{db: db}
}

// FindByID finds a file by composite key
func (r *fileRepository) FindByID(boardID string, wrID int, fileNo int) (*domain.BoardFile, error) {
	var file domain.BoardFile
	err := r.db.Where("bo_table = ? AND wr_id = ? AND bf_no = ?", boardID, wrID, fileNo).
		First(&file).Error
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// Create creates a new file record
func (r *fileRepository) Create(file *domain.BoardFile) error {
	return r.db.Create(file).Error
}

// IncrementDownload increments the download count
func (r *fileRepository) IncrementDownload(boardID string, wrID int, fileNo int) error {
	return r.db.Model(&domain.BoardFile{}).
		Where("bo_table = ? AND wr_id = ? AND bf_no = ?", boardID, wrID, fileNo).
		UpdateColumn("bf_download", gorm.Expr("bf_download + 1")).Error
}

// GetNextFileNo returns the next available file number for a post
func (r *fileRepository) GetNextFileNo(boardID string, wrID int) (int, error) {
	var maxNo *int
	err := r.db.Model(&domain.BoardFile{}).
		Where("bo_table = ? AND wr_id = ?", boardID, wrID).
		Select("MAX(bf_no)").
		Scan(&maxNo).Error
	if err != nil {
		return 0, err
	}
	if maxNo == nil {
		return 0, nil
	}
	return *maxNo + 1, nil
}

// FindFirstFileURLs batch-loads the first file URL for multiple posts (N+1 방지)
func (r *fileRepository) FindFirstFileURLs(boardID string, postIDs []int) (map[int]string, error) {
	if len(postIDs) == 0 {
		return map[int]string{}, nil
	}

	var files []domain.BoardFile
	err := r.db.Where("bo_table = ? AND wr_id IN ? AND bf_no = 0", boardID, postIDs).
		Select("wr_id, bf_file, bf_fileurl").
		Find(&files).Error
	if err != nil {
		return nil, err
	}

	result := make(map[int]string, len(files))
	for _, f := range files {
		if f.FileURL != "" {
			result[f.WriteID] = f.FileURL
		} else if f.File != "" {
			// Fallback: construct S3 URL from bf_file
			result[f.WriteID] = "https://s3.damoang.net/data/file/" + boardID + "/" + f.File
		}
	}
	return result, nil
}
