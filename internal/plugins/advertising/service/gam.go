package service

import (
	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/plugins/advertising/domain"
	"github.com/damoang/angple-backend/internal/plugins/advertising/repository"
)

// GAMService GAM 광고 서비스 인터페이스
type GAMService interface {
	GetConfig() (*domain.GAMConfigResponse, error)
	GetAdUnitByPosition(position string) (*domain.AdUnitConfig, error)
}

// gamService GAM 서비스 구현체
type gamService struct {
	repo   repository.AdRepository
	config map[string]interface{}
	logger plugin.Logger
}

// NewGAMService GAM 서비스 생성자
func NewGAMService(repo repository.AdRepository, config map[string]interface{}, logger plugin.Logger) GAMService {
	return &gamService{
		repo:   repo,
		config: config,
		logger: logger,
	}
}

// GetConfig GAM 전역 설정 조회
func (s *gamService) GetConfig() (*domain.GAMConfigResponse, error) {
	// 설정에서 기본값 로드
	networkCode := s.getConfigString("gam_network_code", "22996793498")
	enableGAM := s.getConfigBool("enable_gam", true)
	enableFallback := s.getConfigBool("enable_adsense_fallback", true)

	// DB에서 활성화된 GAM 광고 단위 조회
	units, err := s.repo.ListAdUnitsByType(domain.AdTypeGAM, true)
	if err != nil {
		s.logger.Error("failed to list GAM ad units", "error", err)
		return nil, err
	}

	// 응답 구성
	adUnits := make(map[string]*domain.AdUnitConfig)
	for _, unit := range units {
		adUnits[unit.Position] = &domain.AdUnitConfig{
			Unit:       unit.GAMUnitPath,
			Sizes:      unit.Sizes,
			Responsive: unit.ResponsiveBreakpoints,
		}
	}

	// 위치 매핑 (하위 호환성)
	positionMap := s.getDefaultPositionMap()

	return &domain.GAMConfigResponse{
		NetworkCode:    networkCode,
		EnableGAM:      enableGAM,
		EnableFallback: enableFallback,
		AdUnits:        adUnits,
		PositionMap:    positionMap,
	}, nil
}

// GetAdUnitByPosition 특정 위치 광고 단위 조회
func (s *gamService) GetAdUnitByPosition(position string) (*domain.AdUnitConfig, error) {
	unit, err := s.repo.FindAdUnitByPosition(position)
	if err != nil {
		return nil, err
	}

	if unit.AdType != domain.AdTypeGAM {
		return nil, nil
	}

	return &domain.AdUnitConfig{
		Unit:       unit.GAMUnitPath,
		Sizes:      unit.Sizes,
		Responsive: unit.ResponsiveBreakpoints,
	}, nil
}

// getConfigString 설정에서 문자열 값 조회
func (s *gamService) getConfigString(key, defaultValue string) string {
	if val, ok := s.config[key]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return defaultValue
}

// getConfigBool 설정에서 불리언 값 조회
func (s *gamService) getConfigBool(key string, defaultValue bool) bool {
	if val, ok := s.config[key]; ok {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}
	return defaultValue
}

// getDefaultPositionMap 기본 위치 매핑 반환
func (s *gamService) getDefaultPositionMap() map[string]string {
	return map[string]string{
		"board-head":           "banner-horizontal",
		"mobile-banner":        "banner-responsive",
		"sidebar":              "banner-square",
		"wing":                 "banner-vertical",
		"custom":               "banner-responsive",
		"group-head":           "banner-horizontal",
		"sidebar-bottom2":      "banner-square",
		"board-view-head":      "banner-horizontal",
		"board-list-head":      "banner-medium",
		"index-head":           "banner-small",
		"board-content":        "banner-responsive",
		"comment-top":          "banner-compact",
		"board-content-bottom": "banner-large",
		"board-middle":         "banner-horizontal",
		"menu-offcanvas":       "banner-responsive",
		"banner-square":        "banner-square",
		"infeed":               "infeed",
		"large":                "banner-large",
		"large-728":            "banner-large-728",
		"small":                "banner-small",
		"responsive":           "banner-responsive",
		"compact":              "banner-compact",
		"pc-only":              "banner-horizontal",
	}
}
