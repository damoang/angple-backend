package service

import (
	"errors"
	"time"

	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/plugins/advertising/domain"
	"github.com/damoang/angple-backend/internal/plugins/advertising/repository"
	"gorm.io/gorm"
)

// 에러 정의
var (
	ErrBannerNotFound = errors.New("banner not found")
)

// BannerService 축하 배너 서비스 인터페이스
type BannerService interface {
	// 공개 API
	GetTodayBanners() ([]*domain.CelebrationBannerResponse, error)
	GetBannersByDate(date time.Time) ([]*domain.CelebrationBannerResponse, error)

	// 관리자 API
	CreateBanner(req *domain.CreateBannerRequest) (*domain.CelebrationBannerResponse, error)
	UpdateBanner(id uint64, req *domain.UpdateBannerRequest) (*domain.CelebrationBannerResponse, error)
	DeleteBanner(id uint64) error
	GetBanner(id uint64) (*domain.CelebrationBannerResponse, error)
	ListBanners(activeOnly bool) ([]*domain.CelebrationBannerResponse, error)
}

// bannerService 축하 배너 서비스 구현체
type bannerService struct {
	repo   repository.AdRepository
	logger plugin.Logger
}

// NewBannerService 배너 서비스 생성자
func NewBannerService(repo repository.AdRepository, logger plugin.Logger) BannerService {
	return &bannerService{
		repo:   repo,
		logger: logger,
	}
}

// GetTodayBanners 오늘 날짜 축하 배너 조회
func (s *bannerService) GetTodayBanners() ([]*domain.CelebrationBannerResponse, error) {
	return s.GetBannersByDate(time.Now())
}

// GetBannersByDate 특정 날짜 배너 조회
func (s *bannerService) GetBannersByDate(date time.Time) ([]*domain.CelebrationBannerResponse, error) {
	banners, err := s.repo.ListBannersByDate(date)
	if err != nil {
		s.logger.Error("failed to list banners by date", "error", err, "date", date)
		return nil, err
	}

	responses := make([]*domain.CelebrationBannerResponse, 0, len(banners))
	for _, banner := range banners {
		responses = append(responses, banner.ToResponse())
	}

	return responses, nil
}

// CreateBanner 배너 생성
func (s *bannerService) CreateBanner(req *domain.CreateBannerRequest) (*domain.CelebrationBannerResponse, error) {
	displayDate, err := time.Parse("2006-01-02", req.DisplayDate)
	if err != nil {
		return nil, errors.New("invalid display_date format, expected YYYY-MM-DD")
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	banner := &domain.CelebrationBanner{
		Title:       req.Title,
		Content:     req.Content,
		ImageURL:    req.ImageURL,
		LinkURL:     req.LinkURL,
		DisplayDate: displayDate,
		IsActive:    isActive,
	}

	if err := s.repo.CreateBanner(banner); err != nil {
		s.logger.Error("failed to create banner", "error", err)
		return nil, err
	}

	return banner.ToResponse(), nil
}

// UpdateBanner 배너 수정
func (s *bannerService) UpdateBanner(id uint64, req *domain.UpdateBannerRequest) (*domain.CelebrationBannerResponse, error) {
	existing, err := s.repo.FindBannerByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBannerNotFound
		}
		return nil, err
	}

	// 업데이트할 필드만 적용
	if req.Title != nil {
		existing.Title = *req.Title
	}
	if req.Content != nil {
		existing.Content = *req.Content
	}
	if req.ImageURL != nil {
		existing.ImageURL = *req.ImageURL
	}
	if req.LinkURL != nil {
		existing.LinkURL = *req.LinkURL
	}
	if req.DisplayDate != nil {
		displayDate, err := time.Parse("2006-01-02", *req.DisplayDate)
		if err != nil {
			return nil, errors.New("invalid display_date format, expected YYYY-MM-DD")
		}
		existing.DisplayDate = displayDate
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	if err := s.repo.UpdateBanner(id, existing); err != nil {
		s.logger.Error("failed to update banner", "error", err, "id", id)
		return nil, err
	}

	// 업데이트된 데이터 다시 조회
	updated, err := s.repo.FindBannerByID(id)
	if err != nil {
		return nil, err
	}

	return updated.ToResponse(), nil
}

// DeleteBanner 배너 삭제
func (s *bannerService) DeleteBanner(id uint64) error {
	_, err := s.repo.FindBannerByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBannerNotFound
		}
		return err
	}

	if err := s.repo.DeleteBanner(id); err != nil {
		s.logger.Error("failed to delete banner", "error", err, "id", id)
		return err
	}

	return nil
}

// GetBanner 배너 상세 조회
func (s *bannerService) GetBanner(id uint64) (*domain.CelebrationBannerResponse, error) {
	banner, err := s.repo.FindBannerByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBannerNotFound
		}
		return nil, err
	}

	return banner.ToResponse(), nil
}

// ListBanners 모든 배너 조회
func (s *bannerService) ListBanners(activeOnly bool) ([]*domain.CelebrationBannerResponse, error) {
	banners, err := s.repo.ListBanners(activeOnly)
	if err != nil {
		s.logger.Error("failed to list banners", "error", err)
		return nil, err
	}

	responses := make([]*domain.CelebrationBannerResponse, 0, len(banners))
	for _, banner := range banners {
		responses = append(responses, banner.ToResponse())
	}

	return responses, nil
}
