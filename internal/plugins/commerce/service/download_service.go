package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/repository"
	"gorm.io/gorm"
)

// 다운로드 에러 정의
var (
	ErrDownloadNotFound     = errors.New("download not found")
	ErrDownloadForbidden    = errors.New("you are not the owner of this download")
	ErrDownloadExpired      = errors.New("download has expired")
	ErrDownloadLimitReached = errors.New("download limit reached")
	ErrFileNotFound         = errors.New("file not found")
	ErrOrderNotPaid         = errors.New("order is not paid")
	ErrNotDigitalProduct    = errors.New("not a digital product")
	ErrInvalidSignature     = errors.New("invalid download signature")
)

// DownloadService 다운로드 서비스 인터페이스
type DownloadService interface {
	// 다운로드 권한 생성
	CreateDownloadAccess(orderItemID uint64, userID uint64) ([]*domain.DownloadResponse, error)

	// 다운로드 목록 조회
	ListDownloads(userID uint64, orderItemID uint64) ([]*domain.DownloadResponse, error)
	ListUserDownloads(userID uint64) ([]*domain.DownloadResponse, error)

	// 다운로드 URL 생성
	GenerateDownloadURL(userID uint64, orderItemID uint64, fileID uint64, baseURL string, secretKey string) (*domain.DownloadURLResponse, error)

	// 다운로드 처리
	ProcessDownload(token string, signature string, userID uint64) (*domain.ProductFile, error)

	// 서명 검증
	VerifySignature(token string, signature string, secretKey string, expiresAt time.Time) bool
}

// downloadService 구현체
type downloadService struct {
	downloadRepo    repository.DownloadRepository
	orderRepo       repository.OrderRepository
	productFileRepo repository.ProductFileRepository
}

// NewDownloadService 생성자
func NewDownloadService(
	downloadRepo repository.DownloadRepository,
	orderRepo repository.OrderRepository,
	productFileRepo repository.ProductFileRepository,
) DownloadService {
	return &downloadService{
		downloadRepo:    downloadRepo,
		orderRepo:       orderRepo,
		productFileRepo: productFileRepo,
	}
}

// CreateDownloadAccess 주문 아이템에 대한 다운로드 권한 생성
func (s *downloadService) CreateDownloadAccess(orderItemID uint64, userID uint64) ([]*domain.DownloadResponse, error) {
	// 주문 아이템 조회
	orderItem, err := s.orderRepo.FindItemByID(orderItemID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderItemNotFound
		}
		return nil, err
	}

	// 디지털 상품인지 확인
	if orderItem.ProductType != domain.ProductTypeDigital {
		return nil, ErrNotDigitalProduct
	}

	// 상품 파일 목록 조회
	files, err := s.productFileRepo.ListByProduct(orderItem.ProductID)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, ErrFileNotFound
	}

	// 다운로드 권한 생성
	var downloads []*domain.DownloadResponse
	for _, file := range files {
		// 기존 다운로드 권한 확인
		existing, err := s.downloadRepo.FindByOrderItemAndFile(orderItemID, file.ID)
		if err == nil && existing != nil {
			// 기존 권한 있음
			downloads = append(downloads, existing.ToResponse(0))
			continue
		}

		// 새 다운로드 권한 생성
		download := &domain.Download{
			OrderItemID:   orderItemID,
			FileID:        file.ID,
			UserID:        userID,
			DownloadCount: 0,
		}

		// 만료일 설정 (기본 30일)
		expiryDays := 30 // TODO: 설정에서 가져오기
		expiresAt := time.Now().AddDate(0, 0, expiryDays)
		download.ExpiresAt = &expiresAt

		if err := s.downloadRepo.Create(download); err != nil {
			return nil, err
		}

		download.ProductFile = file
		downloads = append(downloads, download.ToResponse(0))
	}

	return downloads, nil
}

// ListDownloads 주문 아이템의 다운로드 목록 조회
func (s *downloadService) ListDownloads(userID uint64, orderItemID uint64) ([]*domain.DownloadResponse, error) {
	downloads, err := s.downloadRepo.ListByOrderItem(orderItemID)
	if err != nil {
		return nil, err
	}

	var responses []*domain.DownloadResponse
	for _, download := range downloads {
		// 소유자 확인
		if download.UserID != userID {
			continue
		}
		responses = append(responses, download.ToResponse(0))
	}

	return responses, nil
}

// ListUserDownloads 사용자의 다운로드 목록 조회
func (s *downloadService) ListUserDownloads(userID uint64) ([]*domain.DownloadResponse, error) {
	downloads, err := s.downloadRepo.ListByUser(userID)
	if err != nil {
		return nil, err
	}

	var responses []*domain.DownloadResponse
	for _, download := range downloads {
		responses = append(responses, download.ToResponse(0))
	}

	return responses, nil
}

// GenerateDownloadURL 다운로드 URL 생성
func (s *downloadService) GenerateDownloadURL(userID uint64, orderItemID uint64, fileID uint64, baseURL string, secretKey string) (*domain.DownloadURLResponse, error) {
	// 다운로드 권한 조회
	download, err := s.downloadRepo.FindByOrderItemAndFile(orderItemID, fileID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDownloadNotFound
		}
		return nil, err
	}

	// 소유자 확인
	if download.UserID != userID {
		return nil, ErrDownloadForbidden
	}

	// 만료 확인
	if download.IsExpired() {
		return nil, ErrDownloadExpired
	}

	// 다운로드 횟수 제한 확인
	downloadLimit := 5 // TODO: 설정에서 가져오기
	if download.IsLimitReached(downloadLimit) {
		return nil, ErrDownloadLimitReached
	}

	// 파일 정보 조회
	file, err := s.productFileRepo.FindByID(fileID)
	if err != nil {
		return nil, ErrFileNotFound
	}

	// 서명된 URL 생성 (10분 유효)
	expiresAt := time.Now().Add(10 * time.Minute)
	signature := s.generateSignature(download.DownloadToken, secretKey, expiresAt)

	downloadURL := fmt.Sprintf("%s/api/plugins/commerce/downloads/%s?sig=%s&exp=%d",
		baseURL,
		download.DownloadToken,
		signature,
		expiresAt.Unix(),
	)

	return &domain.DownloadURLResponse{
		DownloadURL: downloadURL,
		ExpiresAt:   expiresAt,
		FileName:    file.FileName,
		FileSize:    int64(file.FileSize),
	}, nil
}

// ProcessDownload 다운로드 처리
func (s *downloadService) ProcessDownload(token string, signature string, userID uint64) (*domain.ProductFile, error) {
	// 다운로드 조회
	download, err := s.downloadRepo.FindByTokenWithFile(token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDownloadNotFound
		}
		return nil, err
	}

	// 소유자 확인
	if download.UserID != userID {
		return nil, ErrDownloadForbidden
	}

	// 만료 확인
	if download.IsExpired() {
		return nil, ErrDownloadExpired
	}

	// 다운로드 횟수 제한 확인
	downloadLimit := 5 // TODO: 설정에서 가져오기
	if download.IsLimitReached(downloadLimit) {
		return nil, ErrDownloadLimitReached
	}

	// 다운로드 횟수 증가
	if err := s.downloadRepo.IncrementDownloadCount(download.ID); err != nil {
		return nil, err
	}

	return download.ProductFile, nil
}

// generateSignature 다운로드 서명 생성
func (s *downloadService) generateSignature(token string, secretKey string, expiresAt time.Time) string {
	data := fmt.Sprintf("%s:%d", token, expiresAt.Unix())
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifySignature 다운로드 서명 검증
func (s *downloadService) VerifySignature(token string, signature string, secretKey string, expiresAt time.Time) bool {
	// 만료 확인
	if time.Now().After(expiresAt) {
		return false
	}

	// 서명 검증
	expected := s.generateSignature(token, secretKey, expiresAt)
	return hmac.Equal([]byte(signature), []byte(expected))
}
