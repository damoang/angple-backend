package gnuboard

import "time"

// G5BoardFile represents the g5_board_file table (file attachments)
type G5BoardFile struct {
	BoTable    string    `gorm:"column:bo_table;primaryKey" json:"bo_table"`
	WrID       int       `gorm:"column:wr_id;primaryKey" json:"wr_id"`
	BfNo       int       `gorm:"column:bf_no;primaryKey" json:"bf_no"`
	BfSource   string    `gorm:"column:bf_source" json:"bf_source"`     // 원본 파일명
	BfFile     string    `gorm:"column:bf_file" json:"bf_file"`         // 저장된 파일명
	BfContent  string    `gorm:"column:bf_content" json:"bf_content"`   // 파일 설명
	BfDownload int       `gorm:"column:bf_download" json:"bf_download"` // 다운로드 횟수
	BfFilesize int64     `gorm:"column:bf_filesize" json:"bf_filesize"` // 파일 크기
	BfWidth    int       `gorm:"column:bf_width" json:"bf_width"`       // 이미지 가로
	BfHeight   int       `gorm:"column:bf_height" json:"bf_height"`     // 이미지 세로
	BfType     int       `gorm:"column:bf_type" json:"bf_type"`         // 파일 타입 (0: 일반, 1: 이미지 등)
	BfDateTime time.Time `gorm:"column:bf_datetime" json:"bf_datetime"`
}

// TableName returns the table name for GORM
func (G5BoardFile) TableName() string {
	return "g5_board_file"
}

// FileResponse is the API response format for file attachments
type FileResponse struct {
	ID             int    `json:"id"`               // bf_no
	OriginalName   string `json:"original_name"`    // bf_source
	FileName       string `json:"filename"`         // bf_file
	Description    string `json:"description"`      // bf_content
	URL            string `json:"url"`              // 다운로드/표시 URL
	ThumbnailURL   string `json:"thumbnail_url"`    // 썸네일 URL (이미지인 경우)
	Size           int64  `json:"size"`             // bf_filesize
	Width          int    `json:"width,omitempty"`  // bf_width
	Height         int    `json:"height,omitempty"` // bf_height
	IsImage        bool   `json:"is_image"`         // 이미지 여부
	DownloadCount  int    `json:"download_count"`   // bf_download
}

// ToFileResponse converts G5BoardFile to API response format
func (f *G5BoardFile) ToFileResponse(baseURL string) FileResponse {
	// 이미지 여부 판단 (bf_type이 1이거나 이미지 확장자인 경우)
	isImage := f.BfType == 1 || f.BfWidth > 0 || f.BfHeight > 0

	// 파일 URL 생성
	fileURL := baseURL + "/data/file/" + f.BoTable + "/" + f.BfFile
	thumbnailURL := ""
	if isImage {
		thumbnailURL = fileURL // 이미지인 경우 썸네일로 사용
	}

	return FileResponse{
		ID:            f.BfNo,
		OriginalName:  f.BfSource,
		FileName:      f.BfFile,
		Description:   f.BfContent,
		URL:           fileURL,
		ThumbnailURL:  thumbnailURL,
		Size:          f.BfFilesize,
		Width:         f.BfWidth,
		Height:        f.BfHeight,
		IsImage:       isImage,
		DownloadCount: f.BfDownload,
	}
}
