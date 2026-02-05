package service

import (
	"crypto/md5"
	"encoding/binary"
	"math/rand"

	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/plugins/advertising/domain"
	"github.com/damoang/angple-backend/internal/plugins/advertising/repository"
)

// AdsenseService AdSense 광고 서비스 인터페이스
type AdsenseService interface {
	GetConfig() (*domain.AdsenseConfigResponse, error)
	GetSlotByPosition(position string, sessionKey string) (*domain.AdsenseSlotInfo, error)
	GetRotationIndex(sessionKey string, maxSlots int) int
}

// adsenseService AdSense 서비스 구현체
type adsenseService struct {
	repo   repository.AdRepository
	config map[string]interface{}
	logger plugin.Logger
}

// NewAdsenseService AdSense 서비스 생성자
func NewAdsenseService(repo repository.AdRepository, config map[string]interface{}, logger plugin.Logger) AdsenseService {
	return &adsenseService{
		repo:   repo,
		config: config,
		logger: logger,
	}
}

// GetConfig AdSense 전역 설정 조회
func (s *adsenseService) GetConfig() (*domain.AdsenseConfigResponse, error) {
	clientID := s.getConfigString("adsense_client_id", "ca-pub-5124617752473025")

	// DB에서 로테이션 설정 조회
	configs, err := s.repo.ListRotationConfigs()
	if err != nil {
		s.logger.Error("failed to list rotation configs", "error", err)
		// DB 오류 시 기본 설정 반환
		return s.getDefaultConfig(clientID), nil
	}

	slots := make(map[string]*domain.AdsenseSlotGroup)
	for _, config := range configs {
		slots[config.Position] = &domain.AdsenseSlotGroup{
			Slots: config.SlotPool,
		}
	}

	// 기본 슬롯이 없으면 추가
	if len(slots) == 0 {
		return s.getDefaultConfig(clientID), nil
	}

	return &domain.AdsenseConfigResponse{
		ClientID: clientID,
		Slots:    slots,
	}, nil
}

// GetSlotByPosition 위치별 AdSense 슬롯 조회 (로테이션 적용)
func (s *adsenseService) GetSlotByPosition(position string, sessionKey string) (*domain.AdsenseSlotInfo, error) {
	clientID := s.getConfigString("adsense_client_id", "ca-pub-5124617752473025")

	// 로테이션 설정 조회
	rotationConfig, err := s.repo.FindRotationConfigByPosition(position)
	if err != nil {
		// 설정이 없으면 기본 슬롯 사용
		defaultSlots := s.getDefaultSlotsForPosition(position)
		if len(defaultSlots) == 0 {
			return nil, nil
		}
		index := s.GetRotationIndex(sessionKey, len(defaultSlots))
		return &domain.AdsenseSlotInfo{
			ClientID: clientID,
			Slot:     defaultSlots[index],
		}, nil
	}

	if len(rotationConfig.SlotPool) == 0 {
		return nil, nil
	}

	// 로테이션 인덱스 계산
	var slot string
	switch rotationConfig.RotationStrategy {
	case domain.RotationRandom:
		slot = rotationConfig.SlotPool[rand.Intn(len(rotationConfig.SlotPool))]
	case domain.RotationSequential:
		fallthrough
	default:
		index := s.GetRotationIndex(sessionKey, len(rotationConfig.SlotPool))
		slot = rotationConfig.SlotPool[index]
	}

	return &domain.AdsenseSlotInfo{
		ClientID: clientID,
		Slot:     slot,
	}, nil
}

// GetRotationIndex 세션 키 기반 로테이션 인덱스 계산
// PHP의 AdRotation 클래스와 동일한 로직 (JWT jti 해시 기반)
func (s *adsenseService) GetRotationIndex(sessionKey string, maxSlots int) int {
	if maxSlots <= 0 {
		return 0
	}

	if sessionKey == "" {
		return rand.Intn(maxSlots)
	}

	// MD5 해시 기반 결정적 인덱스
	hash := md5.Sum([]byte(sessionKey))
	// 처음 8바이트를 uint64로 변환
	num := binary.BigEndian.Uint64(hash[:8])
	return int(num % uint64(maxSlots))
}

// getConfigString 설정에서 문자열 값 조회
func (s *adsenseService) getConfigString(key, defaultValue string) string {
	if val, ok := s.config[key]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return defaultValue
}

// getDefaultConfig 기본 설정 반환
func (s *adsenseService) getDefaultConfig(clientID string) *domain.AdsenseConfigResponse {
	return &domain.AdsenseConfigResponse{
		ClientID: clientID,
		Slots: map[string]*domain.AdsenseSlotGroup{
			"banner_horizontal": {
				Slots: []string{"1282465226", "2649190580", "3781227288", "2468145615", "8268294873", "1273950610", "9281514713", "1980527625"},
			},
			"banner_responsive": {
				Slots: []string{"8336276313", "5710112977", "4188421399", "8915162137", "7602080468", "7968433046", "7041282612", "9368595884"},
			},
			"banner_square": {
				Slots: []string{"7466402991", "5618613634", "4744870889", "3431789215", "5728200944", "3102037601", "2349753787", "1788955938", "1090893531"},
			},
			"banner_vertical": {
				Slots: []string{"7464730194", "1774011047", "8147847708", "7273749737"},
			},
			"banner_small": {
				Slots: []string{"8336276313", "5710112977", "4188421399", "8915162137", "7602080468", "7968433046", "7041282612", "9368595884", "1980732555", "4258455619"},
			},
			"infeed": {
				Slots: []string{"9024980950", "8452181607", "4153843942", "7901517260", "5861978607", "4548896939", "1922733594", "7410508775"},
			},
			"infeed_dark": {
				Slots: []string{"5858055273", "2194142431", "5346440459", "5666483580", "8199834500", "6001046571", "8567979094", "4556102961"},
			},
		},
	}
}

// getDefaultSlotsForPosition 위치별 기본 슬롯 반환
func (s *adsenseService) getDefaultSlotsForPosition(position string) []string {
	config := s.getDefaultConfig("")

	// 위치를 슬롯 그룹에 매핑
	positionToGroup := map[string]string{
		"banner-horizontal":    "banner_horizontal",
		"banner-responsive":    "banner_responsive",
		"banner-square":        "banner_square",
		"banner-vertical":      "banner_vertical",
		"banner-small":         "banner_small",
		"banner-compact":       "banner_responsive",
		"banner-medium":        "banner_horizontal",
		"banner-large":         "banner_horizontal",
		"banner-large-728":     "banner_horizontal",
		"banner-view-content":  "banner_horizontal",
		"banner-halfpage":      "banner_vertical",
		"infeed":               "infeed",
	}

	groupName := positionToGroup[position]
	if groupName == "" {
		groupName = "banner_responsive"
	}

	if group, ok := config.Slots[groupName]; ok {
		return group.Slots
	}

	return nil
}
