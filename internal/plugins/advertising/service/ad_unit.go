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
	ErrAdUnitNotFound = errors.New("ad unit not found")
)

// AdUnitService 광고 단위 서비스 인터페이스
type AdUnitService interface {
	// 공개 API
	GetAdUnitByPosition(position string) (*domain.AdUnitResponse, error)
	GetAdConfig(position string, sessionKey string) (*domain.AdPositionResponse, error)

	// 관리자 API
	CreateAdUnit(req *domain.CreateAdUnitRequest) (*domain.AdUnitResponse, error)
	UpdateAdUnit(id uint64, req *domain.UpdateAdUnitRequest) (*domain.AdUnitResponse, error)
	DeleteAdUnit(id uint64) error
	GetAdUnit(id uint64) (*domain.AdUnitResponse, error)
	ListAdUnits() ([]*domain.AdUnitResponse, error)
}

// adUnitService 광고 단위 서비스 구현체
type adUnitService struct {
	repo           repository.AdRepository
	gamService     GAMService
	adsenseService AdsenseService
	config         map[string]interface{}
	logger         plugin.Logger
}

// NewAdUnitService 광고 단위 서비스 생성자
func NewAdUnitService(
	repo repository.AdRepository,
	gamService GAMService,
	adsenseService AdsenseService,
	config map[string]interface{},
	logger plugin.Logger,
) AdUnitService {
	return &adUnitService{
		repo:           repo,
		gamService:     gamService,
		adsenseService: adsenseService,
		config:         config,
		logger:         logger,
	}
}

// GetAdUnitByPosition 위치별 광고 단위 조회
func (s *adUnitService) GetAdUnitByPosition(position string) (*domain.AdUnitResponse, error) {
	unit, err := s.repo.FindAdUnitByPosition(position)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdUnitNotFound
		}
		return nil, err
	}

	return unit.ToResponse(), nil
}

// GetAdConfig 위치별 통합 광고 설정 조회 (GAM + AdSense fallback)
func (s *adUnitService) GetAdConfig(position string, sessionKey string) (*domain.AdPositionResponse, error) {
	enableGAM := s.getConfigBool("enable_gam", true)
	enableFallback := s.getConfigBool("enable_adsense_fallback", true)

	response := &domain.AdPositionResponse{
		Position:      position,
		RotationIndex: s.adsenseService.GetRotationIndex(sessionKey, 8),
	}

	// GAM 설정 조회
	if enableGAM {
		gamConfig, err := s.gamService.GetAdUnitByPosition(position)
		if err == nil && gamConfig != nil {
			response.GAM = gamConfig
		}
	}

	// AdSense fallback 설정 조회
	if enableFallback || response.GAM == nil {
		adsenseSlot, err := s.adsenseService.GetSlotByPosition(position, sessionKey)
		if err == nil && adsenseSlot != nil {
			response.Adsense = adsenseSlot
		}
	}

	return response, nil
}

// CreateAdUnit 광고 단위 생성
func (s *adUnitService) CreateAdUnit(req *domain.CreateAdUnitRequest) (*domain.AdUnitResponse, error) {
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	unit := &domain.AdUnit{
		Name:                  req.Name,
		AdType:                req.AdType,
		GAMUnitPath:           req.GAMUnitPath,
		AdsenseSlot:           req.AdsenseSlot,
		AdsenseClient:         req.AdsenseClient,
		Sizes:                 req.Sizes,
		ResponsiveBreakpoints: req.ResponsiveBreakpoints,
		Position:              req.Position,
		Priority:              req.Priority,
		IsActive:              isActive,
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	if err := s.repo.CreateAdUnit(unit); err != nil {
		s.logger.Error("failed to create ad unit", "error", err)
		return nil, err
	}

	return unit.ToResponse(), nil
}

// UpdateAdUnit 광고 단위 수정
func (s *adUnitService) UpdateAdUnit(id uint64, req *domain.UpdateAdUnitRequest) (*domain.AdUnitResponse, error) {
	existing, err := s.repo.FindAdUnitByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdUnitNotFound
		}
		return nil, err
	}

	// 업데이트할 필드만 적용
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.AdType != nil {
		existing.AdType = *req.AdType
	}
	if req.GAMUnitPath != nil {
		existing.GAMUnitPath = *req.GAMUnitPath
	}
	if req.AdsenseSlot != nil {
		existing.AdsenseSlot = *req.AdsenseSlot
	}
	if req.AdsenseClient != nil {
		existing.AdsenseClient = *req.AdsenseClient
	}
	if req.Sizes != nil {
		existing.Sizes = req.Sizes
	}
	if req.ResponsiveBreakpoints != nil {
		existing.ResponsiveBreakpoints = req.ResponsiveBreakpoints
	}
	if req.Position != nil {
		existing.Position = *req.Position
	}
	if req.Priority != nil {
		existing.Priority = *req.Priority
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	if err := s.repo.UpdateAdUnit(id, existing); err != nil {
		s.logger.Error("failed to update ad unit", "error", err, "id", id)
		return nil, err
	}

	// 업데이트된 데이터 다시 조회
	updated, err := s.repo.FindAdUnitByID(id)
	if err != nil {
		return nil, err
	}

	return updated.ToResponse(), nil
}

// DeleteAdUnit 광고 단위 삭제
func (s *adUnitService) DeleteAdUnit(id uint64) error {
	_, err := s.repo.FindAdUnitByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAdUnitNotFound
		}
		return err
	}

	if err := s.repo.DeleteAdUnit(id); err != nil {
		s.logger.Error("failed to delete ad unit", "error", err, "id", id)
		return err
	}

	return nil
}

// GetAdUnit 광고 단위 상세 조회
func (s *adUnitService) GetAdUnit(id uint64) (*domain.AdUnitResponse, error) {
	unit, err := s.repo.FindAdUnitByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdUnitNotFound
		}
		return nil, err
	}

	return unit.ToResponse(), nil
}

// ListAdUnits 모든 광고 단위 조회
func (s *adUnitService) ListAdUnits() ([]*domain.AdUnitResponse, error) {
	units, err := s.repo.ListAdUnits(false)
	if err != nil {
		s.logger.Error("failed to list ad units", "error", err)
		return nil, err
	}

	responses := make([]*domain.AdUnitResponse, 0, len(units))
	for _, unit := range units {
		responses = append(responses, unit.ToResponse())
	}

	return responses, nil
}

// getConfigBool 설정에서 불리언 값 조회
func (s *adUnitService) getConfigBool(key string, defaultValue bool) bool {
	if val, ok := s.config[key]; ok {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}
	return defaultValue
}
