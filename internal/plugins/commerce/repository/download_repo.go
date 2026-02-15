package repository

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"gorm.io/gorm"
)

// DownloadRepository 다운로드 저장소 인터페이스
type DownloadRepository interface {
	// 생성/수정
	Create(download *domain.Download) error
	Update(id uint64, download *domain.Download) error
	IncrementDownloadCount(id uint64) error

	// 조회
	FindByID(id uint64) (*domain.Download, error)
	FindByToken(token string) (*domain.Download, error)
	FindByTokenWithFile(token string) (*domain.Download, error)
	FindByOrderItemAndFile(orderItemID, fileID uint64) (*domain.Download, error)

	// 목록 조회
	ListByOrderItem(orderItemID uint64) ([]*domain.Download, error)
	ListByUser(userID uint64) ([]*domain.Download, error)

	// 토큰 생성
	GenerateToken() (string, error)
}

// downloadRepository GORM 구현체
type downloadRepository struct {
	db *gorm.DB
}

// NewDownloadRepository 생성자
func NewDownloadRepository(db *gorm.DB) DownloadRepository {
	return &downloadRepository{db: db}
}

// Create 다운로드 레코드 생성
func (r *downloadRepository) Create(download *domain.Download) error {
	now := time.Now()
	download.CreatedAt = now
	download.UpdatedAt = now

	if download.DownloadToken == "" {
		token, err := r.GenerateToken()
		if err != nil {
			return err
		}
		download.DownloadToken = token
	}

	return r.db.Create(download).Error
}

// Update 다운로드 레코드 수정
func (r *downloadRepository) Update(id uint64, download *domain.Download) error {
	download.UpdatedAt = time.Now()
	return r.db.Model(&domain.Download{}).Where("id = ?", id).Updates(download).Error
}

// IncrementDownloadCount 다운로드 횟수 증가
func (r *downloadRepository) IncrementDownloadCount(id uint64) error {
	now := time.Now()
	return r.db.Model(&domain.Download{}).
		Where("id = ?", id).
		UpdateColumns(map[string]interface{}{
			"download_count":   gorm.Expr("download_count + ?", 1),
			"last_download_at": now,
			"updated_at":       now,
		}).Error
}

// FindByID ID로 다운로드 조회
func (r *downloadRepository) FindByID(id uint64) (*domain.Download, error) {
	var download domain.Download
	err := r.db.Where("id = ?", id).First(&download).Error
	if err != nil {
		return nil, err
	}
	return &download, nil
}

// FindByToken 토큰으로 다운로드 조회
func (r *downloadRepository) FindByToken(token string) (*domain.Download, error) {
	var download domain.Download
	err := r.db.Where("download_token = ?", token).First(&download).Error
	if err != nil {
		return nil, err
	}
	return &download, nil
}

// FindByTokenWithFile 토큰으로 다운로드 조회 (파일 정보 포함)
func (r *downloadRepository) FindByTokenWithFile(token string) (*domain.Download, error) {
	var download domain.Download
	err := r.db.Preload("ProductFile").Preload("OrderItem").
		Where("download_token = ?", token).First(&download).Error
	if err != nil {
		return nil, err
	}
	return &download, nil
}

// FindByOrderItemAndFile 주문 아이템 ID와 파일 ID로 다운로드 조회
func (r *downloadRepository) FindByOrderItemAndFile(orderItemID, fileID uint64) (*domain.Download, error) {
	var download domain.Download
	err := r.db.Where("order_item_id = ? AND file_id = ?", orderItemID, fileID).First(&download).Error
	if err != nil {
		return nil, err
	}
	return &download, nil
}

// ListByOrderItem 주문 아이템의 다운로드 목록 조회
func (r *downloadRepository) ListByOrderItem(orderItemID uint64) ([]*domain.Download, error) {
	var downloads []*domain.Download
	err := r.db.Preload("ProductFile").
		Where("order_item_id = ?", orderItemID).
		Order("created_at ASC").
		Find(&downloads).Error
	if err != nil {
		return nil, err
	}
	return downloads, nil
}

// ListByUser 사용자의 다운로드 목록 조회
func (r *downloadRepository) ListByUser(userID uint64) ([]*domain.Download, error) {
	var downloads []*domain.Download
	err := r.db.Preload("ProductFile").Preload("OrderItem").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&downloads).Error
	if err != nil {
		return nil, err
	}
	return downloads, nil
}

// GenerateToken 다운로드 토큰 생성
func (r *downloadRepository) GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// ProductFileRepository 상품 파일 저장소 인터페이스
type ProductFileRepository interface {
	Create(file *domain.ProductFile) error
	Update(id uint64, file *domain.ProductFile) error
	Delete(id uint64) error
	FindByID(id uint64) (*domain.ProductFile, error)
	ListByProduct(productID uint64) ([]*domain.ProductFile, error)
}

// productFileRepository GORM 구현체
type productFileRepository struct {
	db *gorm.DB
}

// NewProductFileRepository 생성자
func NewProductFileRepository(db *gorm.DB) ProductFileRepository {
	return &productFileRepository{db: db}
}

// Create 상품 파일 생성
func (r *productFileRepository) Create(file *domain.ProductFile) error {
	now := time.Now()
	file.CreatedAt = now
	file.UpdatedAt = now
	return r.db.Create(file).Error
}

// Update 상품 파일 수정
func (r *productFileRepository) Update(id uint64, file *domain.ProductFile) error {
	file.UpdatedAt = time.Now()
	return r.db.Model(&domain.ProductFile{}).Where("id = ?", id).Updates(file).Error
}

// Delete 상품 파일 삭제
func (r *productFileRepository) Delete(id uint64) error {
	return r.db.Delete(&domain.ProductFile{}, id).Error
}

// FindByID ID로 상품 파일 조회
func (r *productFileRepository) FindByID(id uint64) (*domain.ProductFile, error) {
	var file domain.ProductFile
	err := r.db.Where("id = ?", id).First(&file).Error
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// ListByProduct 상품의 파일 목록 조회
func (r *productFileRepository) ListByProduct(productID uint64) ([]*domain.ProductFile, error) {
	var files []*domain.ProductFile
	err := r.db.Where("product_id = ?", productID).Order("sort_order ASC, created_at ASC").Find(&files).Error
	if err != nil {
		return nil, err
	}
	return files, nil
}
