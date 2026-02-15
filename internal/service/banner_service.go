package service

import (
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// BannerService defines the business logic for banners
type BannerService interface {
	// Banner methods
	GetAllBanners() ([]domain.BannerResponse, error)
	GetActiveBanners() ([]domain.BannerResponse, error)
	GetBannersByPosition(position domain.BannerPosition) (*domain.BannerListResponse, error)
	GetBannerByID(id string) (*domain.BannerResponse, error)
	CreateBanner(req *domain.CreateBannerRequest) (*domain.BannerResponse, error)
	UpdateBanner(id string, req *domain.UpdateBannerRequest) (*domain.BannerResponse, error)
	DeleteBanner(id string) error

	// Click and view tracking
	TrackClick(id string, req *domain.BannerClickRequest) error
	TrackView(id string) error
	IncrementViewCount(id string) error

	// Statistics
	GetBannerStats(id string) (*domain.BannerStatsResponse, error)
}

type bannerService struct {
	repo repository.BannerRepository
}

// NewBannerService creates a new BannerService
func NewBannerService(repo repository.BannerRepository) BannerService {
	return &bannerService{repo: repo}
}

// GetAllBanners retrieves all banners
func (s *bannerService) GetAllBanners() ([]domain.BannerResponse, error) {
	banners, err := s.repo.GetAllBanners()
	if err != nil {
		return nil, err
	}

	responses := make([]domain.BannerResponse, len(banners))
	for i, banner := range banners {
		responses[i] = banner.ToResponse()
	}

	return responses, nil
}

// GetActiveBanners retrieves active banners
func (s *bannerService) GetActiveBanners() ([]domain.BannerResponse, error) {
	banners, err := s.repo.GetActiveBanners()
	if err != nil {
		return nil, err
	}

	responses := make([]domain.BannerResponse, len(banners))
	for i, banner := range banners {
		responses[i] = banner.ToResponse()
	}

	return responses, nil
}

// GetBannersByPosition retrieves banners by position
func (s *bannerService) GetBannersByPosition(position domain.BannerPosition) (*domain.BannerListResponse, error) {
	banners, err := s.repo.GetBannersByPosition(position)
	if err != nil {
		return nil, err
	}

	responses := make([]domain.BannerResponse, len(banners))
	for i, banner := range banners {
		responses[i] = banner.ToResponse()
	}

	return &domain.BannerListResponse{
		Banners:  responses,
		Total:    len(responses),
		Position: position,
	}, nil
}

// GetBannerByID retrieves a banner by ID
func (s *bannerService) GetBannerByID(id string) (*domain.BannerResponse, error) {
	banner, err := s.repo.FindBannerByID(id)
	if err != nil {
		return nil, err
	}

	resp := banner.ToResponse()
	return &resp, nil
}

// CreateBanner creates a new banner
func (s *bannerService) CreateBanner(req *domain.CreateBannerRequest) (*domain.BannerResponse, error) {
	// Set default target
	target := req.Target
	if target == "" {
		target = "_blank"
	}

	banner := &domain.Banner{
		Title:     req.Title,
		ImageURL:  req.ImageURL,
		LinkURL:   req.LinkURL,
		Position:  req.Position,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		Priority:  req.Priority,
		IsActive:  req.IsActive,
		AltText:   req.AltText,
		Target:    target,
		Memo:      req.Memo,
	}

	if err := s.repo.CreateBanner(banner); err != nil {
		return nil, err
	}

	resp := banner.ToResponse()
	return &resp, nil
}

// UpdateBanner updates a banner
func (s *bannerService) UpdateBanner(id string, req *domain.UpdateBannerRequest) (*domain.BannerResponse, error) {
	banner, err := s.repo.FindBannerByID(id)
	if err != nil {
		return nil, err
	}

	// Update fields
	if req.Title != "" {
		banner.Title = req.Title
	}
	if req.ImageURL != "" {
		banner.ImageURL = req.ImageURL
	}
	if req.LinkURL != "" {
		banner.LinkURL = req.LinkURL
	}
	if req.Position != "" {
		banner.Position = req.Position
	}
	if req.Target != "" {
		banner.Target = req.Target
	}

	banner.StartDate = req.StartDate
	banner.EndDate = req.EndDate
	banner.Priority = req.Priority
	banner.IsActive = req.IsActive
	banner.AltText = req.AltText
	banner.Memo = req.Memo

	if err := s.repo.UpdateBanner(banner); err != nil {
		return nil, err
	}

	resp := banner.ToResponse()
	return &resp, nil
}

// DeleteBanner deletes a banner
func (s *bannerService) DeleteBanner(id string) error {
	return s.repo.DeleteBanner(id)
}

// TrackClick tracks a banner click
func (s *bannerService) TrackClick(id string, req *domain.BannerClickRequest) error {
	// Increment click count
	if err := s.repo.IncrementClickCount(id); err != nil {
		return err
	}

	// Create click log
	log := &domain.BannerClickLog{
		BannerID:  id,
		MemberID:  req.MemberID,
		IPAddress: req.IPAddress,
		UserAgent: req.UserAgent,
		Referer:   req.Referer,
	}

	return s.repo.CreateClickLog(log)
}

// TrackView tracks a banner view (alias for IncrementViewCount)
func (s *bannerService) TrackView(id string) error {
	return s.repo.IncrementViewCount(id)
}

// IncrementViewCount increments the view count of a banner
func (s *bannerService) IncrementViewCount(id string) error {
	return s.repo.IncrementViewCount(id)
}

// GetBannerStats retrieves statistics for a banner
func (s *bannerService) GetBannerStats(id string) (*domain.BannerStatsResponse, error) {
	banner, err := s.repo.FindBannerByID(id)
	if err != nil {
		return nil, err
	}

	return &domain.BannerStatsResponse{
		BannerID:   banner.ID,
		Name:       banner.Name,
		ClickCount: 0, // 실제 DB에는 없음
		ViewCount:  0, // 실제 DB에는 없음
		CTR:        0,
	}, nil
}
