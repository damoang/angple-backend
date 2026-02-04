package domain

import (
	"time"
)

// ProductFile 디지털 상품 파일 엔티티
type ProductFile struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	ProductID   uint64    `gorm:"not null" json:"product_id"`
	FileName    string    `gorm:"column:file_name;size:255;not null" json:"file_name"`
	FilePath    string    `gorm:"column:file_path;size:500;not null" json:"-"`
	FileSize    uint64    `gorm:"column:file_size;not null" json:"file_size"`
	FileType    string    `gorm:"column:file_type;size:100" json:"file_type"`
	FileHash    string    `gorm:"column:file_hash;size:64" json:"-"`
	DisplayName string    `gorm:"column:display_name;size:255" json:"display_name"`
	Description string    `gorm:"type:text" json:"description,omitempty"`
	SortOrder   int       `gorm:"column:sort_order;default:0" json:"sort_order"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at"`
}

// TableName GORM 테이블명
func (ProductFile) TableName() string {
	return "commerce_product_files"
}

// ProductFileResponse 상품 파일 응답 DTO
type ProductFileResponse struct {
	ID          uint64    `json:"id"`
	ProductID   uint64    `json:"product_id"`
	FileName    string    `json:"file_name"`
	FileSize    uint64    `json:"file_size"`
	FileType    string    `json:"file_type"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description,omitempty"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
}

// ToResponse ProductFile을 ProductFileResponse로 변환
func (f *ProductFile) ToResponse() *ProductFileResponse {
	displayName := f.DisplayName
	if displayName == "" {
		displayName = f.FileName
	}

	return &ProductFileResponse{
		ID:          f.ID,
		ProductID:   f.ProductID,
		FileName:    f.FileName,
		FileSize:    f.FileSize,
		FileType:    f.FileType,
		DisplayName: displayName,
		Description: f.Description,
		SortOrder:   f.SortOrder,
		CreatedAt:   f.CreatedAt,
	}
}

// CreateProductFileRequest 상품 파일 생성 요청 DTO
type CreateProductFileRequest struct {
	FileName    string `json:"file_name" binding:"required,max=255"`
	FilePath    string `json:"file_path" binding:"required,max=500"`
	FileSize    uint64 `json:"file_size" binding:"required,gte=1"`
	FileType    string `json:"file_type" binding:"omitempty,max=100"`
	FileHash    string `json:"file_hash" binding:"omitempty,max=64"`
	DisplayName string `json:"display_name" binding:"omitempty,max=255"`
	Description string `json:"description" binding:"omitempty"`
	SortOrder   int    `json:"sort_order" binding:"omitempty,gte=0"`
}

// UpdateProductFileRequest 상품 파일 수정 요청 DTO
type UpdateProductFileRequest struct {
	DisplayName *string `json:"display_name" binding:"omitempty,max=255"`
	Description *string `json:"description" binding:"omitempty"`
	SortOrder   *int    `json:"sort_order" binding:"omitempty,gte=0"`
}
