package gnuboard

import (
	"strings"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"

	"gorm.io/gorm"
)

// imageExtensions lists common image file extensions
var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".bmp": true, ".svg": true, ".avif": true,
}

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

// GetThumbnails returns the first image file for each post (batch query for list pages)
// Returns a map of postID -> thumbnail URL
func (r *FileRepository) GetThumbnails(boardID string, postIDs []int, baseURL string) map[int]string {
	if len(postIDs) == 0 {
		return map[int]string{}
	}

	// Get first file (bf_no = 0) for each post
	var files []gnuboard.G5BoardFile
	r.db.Where("bo_table = ? AND wr_id IN ? AND bf_no = 0", boardID, postIDs).
		Find(&files)

	result := make(map[int]string, len(files))
	for _, f := range files {
		// Check if file is an image by extension, bf_type, or dimensions
		isImage := f.BfType == 1 || f.BfWidth > 0
		if !isImage {
			// Check by file extension
			lowerName := strings.ToLower(f.BfFile)
			for ext := range imageExtensions {
				if strings.HasSuffix(lowerName, ext) {
					isImage = true
					break
				}
			}
		}
		if isImage {
			result[f.WrID] = baseURL + "/data/file/" + f.BoTable + "/" + f.BfFile
		}
	}
	return result
}
