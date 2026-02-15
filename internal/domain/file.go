package domain

import "time"

// BoardFile represents an attached file (g5_board_file table)
type BoardFile struct {
	DateTime time.Time `gorm:"column:bf_datetime" json:"datetime"`
	BoardID  string    `gorm:"column:bo_table;primaryKey;size:20" json:"board_id"`
	Source   string    `gorm:"column:bf_source;size:255" json:"source"`
	File     string    `gorm:"column:bf_file;size:255" json:"file"`
	FileURL  string    `gorm:"column:bf_fileurl;size:255" json:"file_url"`
	Content  string    `gorm:"column:bf_content;type:text" json:"content"`
	Storage  string    `gorm:"column:bf_storage;size:50" json:"storage"`
	WriteID  int       `gorm:"column:wr_id;primaryKey" json:"write_id"`
	FileNo   int       `gorm:"column:bf_no;primaryKey" json:"file_no"`
	Download int       `gorm:"column:bf_download;default:0" json:"download"`
	FileSize int       `gorm:"column:bf_filesize" json:"file_size"`
	Width    int       `gorm:"column:bf_width;default:0" json:"width"`
	Height   int       `gorm:"column:bf_height;default:0" json:"height"`
	Type     int       `gorm:"column:bf_type;default:0" json:"type"`
}

func (BoardFile) TableName() string {
	return "g5_board_file"
}

// FileUploadResponse represents the response after a file upload
type FileUploadResponse struct {
	URL      string `json:"url"`
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

// FileDownloadInfo contains information needed for file download
type FileDownloadInfo struct {
	FilePath    string
	Source      string
	ContentType string
}
