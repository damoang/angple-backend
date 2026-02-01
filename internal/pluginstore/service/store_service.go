package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"github.com/damoang/angple-backend/internal/pluginstore/repository"
)

// StoreService 플러그인 스토어 서비스 (설치/활성화/비활성화/제거)
type StoreService struct {
	installRepo *repository.InstallationRepository
	eventRepo   *repository.EventRepository
	settingRepo *repository.SettingRepository
	catalogSvc  *CatalogService
	logger      plugin.Logger
}

// NewStoreService 생성자
func NewStoreService(
	installRepo *repository.InstallationRepository,
	eventRepo *repository.EventRepository,
	settingRepo *repository.SettingRepository,
	catalogSvc *CatalogService,
	logger plugin.Logger,
) *StoreService {
	return &StoreService{
		installRepo: installRepo,
		eventRepo:   eventRepo,
		settingRepo: settingRepo,
		catalogSvc:  catalogSvc,
		logger:      logger,
	}
}

// Install 플러그인 설치 (DB 레코드 생성 + Enable)
func (s *StoreService) Install(name, actorID string, manager *plugin.Manager) error {
	// 카탈로그에 존재하는지 확인
	manifest := s.catalogSvc.GetManifest(name)
	if manifest == nil {
		return fmt.Errorf("plugin %s not found in catalog", name)
	}

	// 이미 설치되었는지 확인
	existing, _ := s.installRepo.FindByName(name)
	if existing != nil {
		return fmt.Errorf("plugin %s is already installed", name)
	}

	// 의존성 체크
	if err := s.checkDependencies(manifest); err != nil {
		return err
	}

	// 충돌 체크
	if err := s.checkConflicts(manifest); err != nil {
		return err
	}

	// DB 레코드 생성
	now := time.Now()
	inst := &domain.PluginInstallation{
		PluginName:  name,
		Version:     manifest.Version,
		Status:      domain.StatusEnabled,
		InstalledAt: now,
		EnabledAt:   &now,
		InstalledBy: &actorID,
	}

	if err := s.installRepo.Create(inst); err != nil {
		return fmt.Errorf("failed to create installation record: %w", err)
	}

	// 플러그인 매니저에서 활성화
	if err := manager.Enable(name); err != nil {
		// 롤백: DB 상태를 error로 변경
		errMsg := err.Error()
		inst.Status = domain.StatusError
		inst.ErrorMessage = &errMsg
		_ = s.installRepo.Update(inst)
		s.logEvent(name, domain.EventError, map[string]string{"error": errMsg}, actorID)
		return fmt.Errorf("failed to enable plugin: %w", err)
	}

	// 이벤트 로그
	s.logEvent(name, domain.EventInstalled, map[string]string{"version": manifest.Version}, actorID)

	s.logger.Info("Plugin %s installed and enabled by %s", name, actorID)
	return nil
}

// Enable 플러그인 활성화
func (s *StoreService) Enable(name, actorID string, manager *plugin.Manager) error {
	inst, err := s.installRepo.FindByName(name)
	if err != nil {
		return fmt.Errorf("plugin %s is not installed", name)
	}

	if inst.Status == domain.StatusEnabled {
		return nil
	}

	// 매니저에서 활성화
	if err := manager.Enable(name); err != nil {
		errMsg := err.Error()
		inst.Status = domain.StatusError
		inst.ErrorMessage = &errMsg
		_ = s.installRepo.Update(inst)
		s.logEvent(name, domain.EventError, map[string]string{"error": errMsg}, actorID)
		return fmt.Errorf("failed to enable plugin: %w", err)
	}

	// DB 상태 갱신
	now := time.Now()
	inst.Status = domain.StatusEnabled
	inst.EnabledAt = &now
	inst.ErrorMessage = nil
	if err := s.installRepo.Update(inst); err != nil {
		return fmt.Errorf("failed to update installation: %w", err)
	}

	s.logEvent(name, domain.EventEnabled, nil, actorID)
	s.logger.Info("Plugin %s enabled by %s", name, actorID)
	return nil
}

// Disable 플러그인 비활성화
func (s *StoreService) Disable(name, actorID string, manager *plugin.Manager) error {
	inst, err := s.installRepo.FindByName(name)
	if err != nil {
		return fmt.Errorf("plugin %s is not installed", name)
	}

	if inst.Status == domain.StatusDisabled {
		return nil
	}

	// 매니저에서 비활성화
	if err := manager.Disable(name); err != nil {
		return fmt.Errorf("failed to disable plugin: %w", err)
	}

	// DB 상태 갱신
	now := time.Now()
	inst.Status = domain.StatusDisabled
	inst.DisabledAt = &now
	if err := s.installRepo.Update(inst); err != nil {
		return fmt.Errorf("failed to update installation: %w", err)
	}

	s.logEvent(name, domain.EventDisabled, nil, actorID)
	s.logger.Info("Plugin %s disabled by %s", name, actorID)
	return nil
}

// Uninstall 플러그인 제거
func (s *StoreService) Uninstall(name, actorID string, manager *plugin.Manager) error {
	inst, err := s.installRepo.FindByName(name)
	if err != nil {
		return fmt.Errorf("plugin %s is not installed", name)
	}

	// 활성화 상태면 먼저 비활성화
	if inst.Status == domain.StatusEnabled {
		if err := manager.Disable(name); err != nil {
			s.logger.Warn("Failed to disable plugin %s during uninstall: %v", name, err)
		}
	}

	// 설정 삭제
	if err := s.settingRepo.DeleteByPlugin(name); err != nil {
		s.logger.Warn("Failed to delete settings for plugin %s: %v", name, err)
	}

	// 설치 레코드 삭제
	if err := s.installRepo.Delete(name); err != nil {
		return fmt.Errorf("failed to delete installation: %w", err)
	}

	s.logEvent(name, domain.EventUninstalled, nil, actorID)
	s.logger.Info("Plugin %s uninstalled by %s", name, actorID)
	return nil
}

// BootEnabledPlugins 서버 부팅 시 DB에서 enabled 플러그인 자동 활성화
func (s *StoreService) BootEnabledPlugins(manager *plugin.Manager) error {
	enabled, err := s.installRepo.FindEnabled()
	if err != nil {
		return fmt.Errorf("failed to load enabled plugins: %w", err)
	}

	for _, inst := range enabled {
		if err := manager.Enable(inst.PluginName); err != nil {
			s.logger.Warn("Failed to boot plugin %s: %v", inst.PluginName, err)
			// DB 상태를 error로 변경
			errMsg := err.Error()
			inst.Status = domain.StatusError
			inst.ErrorMessage = &errMsg
			_ = s.installRepo.Update(&inst)
			continue
		}
		s.logger.Info("Booted plugin: %s", inst.PluginName)
	}

	return nil
}

// GetInstallation 설치 정보 조회
func (s *StoreService) GetInstallation(name string) (*domain.PluginInstallation, error) {
	return s.installRepo.FindByName(name)
}

// GetEvents 플러그인 이벤트 로그 조회
func (s *StoreService) GetEvents(pluginName string, limit int) ([]domain.PluginEvent, error) {
	return s.eventRepo.ListByPlugin(pluginName, limit)
}

// checkDependencies 의존성 검증
func (s *StoreService) checkDependencies(manifest *plugin.PluginManifest) error {
	for _, dep := range manifest.Requires.Plugins {
		inst, err := s.installRepo.FindByName(dep.Name)
		if err != nil || inst == nil {
			return fmt.Errorf("dependency not satisfied: plugin %s requires %s", manifest.Name, dep.Name)
		}
		if inst.Status != domain.StatusEnabled {
			return fmt.Errorf("dependency not satisfied: plugin %s requires %s to be enabled", manifest.Name, dep.Name)
		}
		// 의존 플러그인 버전 범위 검증
		if dep.Version != "" {
			if err := plugin.CheckVersionRange(inst.Version, dep.Version); err != nil {
				return fmt.Errorf("dependency version not satisfied: plugin %s requires %s %s, installed %s",
					manifest.Name, dep.Name, dep.Version, inst.Version)
			}
		}
	}
	return nil
}

// checkConflicts 충돌 검증
func (s *StoreService) checkConflicts(manifest *plugin.PluginManifest) error {
	for _, conflict := range manifest.Conflicts {
		inst, err := s.installRepo.FindByName(conflict)
		if err == nil && inst != nil && inst.Status == domain.StatusEnabled {
			return fmt.Errorf("conflict: plugin %s conflicts with enabled plugin %s", manifest.Name, conflict)
		}
	}
	return nil
}

// logEvent 이벤트 기록 헬퍼
func (s *StoreService) logEvent(pluginName, eventType string, details map[string]string, actorID string) {
	var detailsJSON *string
	if details != nil {
		b, _ := json.Marshal(details)
		str := string(b)
		detailsJSON = &str
	}

	event := &domain.PluginEvent{
		PluginName: pluginName,
		EventType:  eventType,
		Details:    detailsJSON,
		ActorID:    &actorID,
	}

	if err := s.eventRepo.Create(event); err != nil {
		s.logger.Warn("Failed to log event for plugin %s: %v", pluginName, err)
	}
}
