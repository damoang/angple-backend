package service

import (
	"encoding/json"
	"fmt"

	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"github.com/damoang/angple-backend/internal/pluginstore/repository"
)

// SettingService 플러그인 설정 관리 서비스
type SettingService struct {
	settingRepo *repository.SettingRepository
	eventRepo   *repository.EventRepository
	catalogSvc  *CatalogService
}

// NewSettingService 생성자
func NewSettingService(
	settingRepo *repository.SettingRepository,
	eventRepo *repository.EventRepository,
	catalogSvc *CatalogService,
) *SettingService {
	return &SettingService{
		settingRepo: settingRepo,
		eventRepo:   eventRepo,
		catalogSvc:  catalogSvc,
	}
}

// SettingWithSchema 설정값과 스키마 결합
type SettingWithSchema struct {
	Key          string      `json:"key"`
	Value        interface{} `json:"value"`
	Type         string      `json:"type"`
	Label        string      `json:"label"`
	DefaultValue interface{} `json:"default"`
}

// GetSettings 플러그인 설정 조회 (스키마 + 값 결합)
func (s *SettingService) GetSettings(pluginName string) ([]SettingWithSchema, error) {
	manifest := s.catalogSvc.GetManifest(pluginName)
	if manifest == nil {
		return nil, fmt.Errorf("plugin %s not found", pluginName)
	}

	// DB에서 저장된 설정 조회
	saved, err := s.settingRepo.GetAll(pluginName)
	if err != nil {
		return nil, fmt.Errorf("failed to get settings: %w", err)
	}

	savedMap := make(map[string]string)
	for _, s := range saved {
		if s.SettingValue != nil {
			savedMap[s.SettingKey] = *s.SettingValue
		}
	}

	// 스키마 기반으로 결과 생성
	result := make([]SettingWithSchema, 0, len(manifest.Settings))
	for _, cfg := range manifest.Settings {
		item := SettingWithSchema{
			Key:          cfg.Key,
			Type:         cfg.Type,
			Label:        cfg.Label,
			DefaultValue: cfg.Default,
			Value:        cfg.Default, // 기본값
		}

		if v, ok := savedMap[cfg.Key]; ok {
			item.Value = v
		}

		result = append(result, item)
	}

	return result, nil
}

// SaveSettings 플러그인 설정 저장
func (s *SettingService) SaveSettings(pluginName string, settings map[string]string, actorID string) error {
	manifest := s.catalogSvc.GetManifest(pluginName)
	if manifest == nil {
		return fmt.Errorf("plugin %s not found", pluginName)
	}

	// 스키마 맵 생성 (검증용)
	schemaMap := make(map[string]plugin.SettingConfig)
	for _, cfg := range manifest.Settings {
		schemaMap[cfg.Key] = cfg
	}

	for key, value := range settings {
		schema, ok := schemaMap[key]
		if !ok {
			return fmt.Errorf("unknown setting key: %s", key)
		}

		// 스키마 기반 검증
		if err := ValidateSetting(schema, value); err != nil {
			return err
		}

		v := value
		setting := &domain.PluginSetting{
			PluginName:   pluginName,
			SettingKey:   key,
			SettingValue: &v,
		}
		if err := s.settingRepo.Set(setting); err != nil {
			return fmt.Errorf("failed to save setting %s: %w", key, err)
		}
	}

	// 이벤트 로그
	detailsJSON, _ := json.Marshal(settings)
	detailsStr := string(detailsJSON)
	event := &domain.PluginEvent{
		PluginName: pluginName,
		EventType:  domain.EventConfigChanged,
		Details:    &detailsStr,
		ActorID:    &actorID,
	}
	_ = s.eventRepo.Create(event)

	return nil
}

// GetSettingsAsMap 설정값을 map[string]interface{}로 반환 (PluginContext.Config 주입용)
// 기본값 적용 + 타입 변환 포함
func (s *SettingService) GetSettingsAsMap(pluginName string) (map[string]interface{}, error) {
	manifest := s.catalogSvc.GetManifest(pluginName)

	saved, err := s.settingRepo.GetAll(pluginName)
	if err != nil {
		return nil, err
	}

	savedMap := make(map[string]string)
	for _, setting := range saved {
		if setting.SettingValue != nil {
			savedMap[setting.SettingKey] = *setting.SettingValue
		}
	}

	// 매니페스트가 없으면 string 그대로 반환
	if manifest == nil {
		result := make(map[string]interface{}, len(savedMap))
		for k, v := range savedMap {
			result[k] = v
		}
		return result, nil
	}

	// 기본값 적용
	withDefaults := ApplyDefaults(manifest.Settings, savedMap)

	// 스키마 맵 생성
	schemaMap := make(map[string]plugin.SettingConfig)
	for _, cfg := range manifest.Settings {
		schemaMap[cfg.Key] = cfg
	}

	// 타입 변환
	result := make(map[string]interface{}, len(withDefaults))
	for k, v := range withDefaults {
		if schema, ok := schemaMap[k]; ok {
			result[k] = ConvertSettingValue(schema, v)
		} else {
			result[k] = v
		}
	}

	return result, nil
}
