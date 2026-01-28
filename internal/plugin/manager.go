package plugin

import (
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Manager 플러그인 매니저 - 플러그인 라이프사이클 관리
type Manager struct {
	loader   *Loader
	registry *Registry
	db       *gorm.DB
	redis    *redis.Client
	plugins  map[string]*PluginInfo
	mu       sync.RWMutex
	logger   Logger
}

// NewManager 새 매니저 생성
func NewManager(pluginsDir string, db *gorm.DB, redisClient *redis.Client, logger Logger) *Manager {
	return &Manager{
		loader:   NewLoader(pluginsDir),
		registry: NewRegistry(),
		db:       db,
		redis:    redisClient,
		plugins:  make(map[string]*PluginInfo),
		logger:   logger,
	}
}

// GetRegistry 레지스트리 반환
func (m *Manager) GetRegistry() *Registry {
	return m.registry
}

// LoadAll 모든 플러그인 로드
func (m *Manager) LoadAll() error {
	discovered, err := m.loader.DiscoverPlugins()
	if err != nil {
		return fmt.Errorf("failed to discover plugins: %w", err)
	}

	for _, info := range discovered {
		if info.Error != nil {
			m.logger.Error("Plugin load error: %s - %v", info.Path, info.Error)
			continue
		}

		m.mu.Lock()
		m.plugins[info.Manifest.Name] = info
		m.mu.Unlock()

		m.logger.Info("Discovered plugin: %s v%s", info.Manifest.Name, info.Manifest.Version)
	}

	return nil
}

// RegisterBuiltIn 내장 플러그인 등록
func (m *Manager) RegisterBuiltIn(name string, p Plugin, manifest *PluginManifest) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	info := &PluginInfo{
		Manifest:  manifest,
		Path:      "",
		Status:    StatusDisabled,
		Instance:  p,
		IsBuiltIn: true,
		LoadedAt:  time.Now().Unix(),
	}

	m.plugins[name] = info
	m.logger.Info("Registered built-in plugin: %s v%s", name, manifest.Version)

	return nil
}

// Enable 플러그인 활성화
func (m *Manager) Enable(name string) error {
	m.mu.Lock()
	info, exists := m.plugins[name]
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	if info.Status == StatusEnabled {
		return nil // 이미 활성화됨
	}

	// 플러그인 인스턴스가 있으면 초기화
	if info.Instance != nil {
		ctx := &PluginContext{
			DB:       m.db,
			Redis:    m.redis,
			Config:   make(map[string]interface{}),
			Logger:   m.logger,
			BasePath: info.Path,
		}

		if err := info.Instance.Initialize(ctx); err != nil {
			info.Status = StatusError
			info.Error = err
			return fmt.Errorf("failed to initialize plugin %s: %w", name, err)
		}

		// 라우트 등록
		m.registry.RegisterPlugin(info)
	}

	m.mu.Lock()
	info.Status = StatusEnabled
	m.mu.Unlock()

	m.logger.Info("Enabled plugin: %s", name)
	return nil
}

// Disable 플러그인 비활성화
func (m *Manager) Disable(name string) error {
	m.mu.Lock()
	info, exists := m.plugins[name]
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	if info.Status != StatusEnabled {
		return nil // 이미 비활성화됨
	}

	// 플러그인 종료
	if info.Instance != nil {
		if err := info.Instance.Shutdown(); err != nil {
			m.logger.Warn("Plugin %s shutdown error: %v", name, err)
		}
	}

	// 라우트 해제
	m.registry.UnregisterPlugin(name)

	m.mu.Lock()
	info.Status = StatusDisabled
	m.mu.Unlock()

	m.logger.Info("Disabled plugin: %s", name)
	return nil
}

// GetPlugin 플러그인 정보 조회
func (m *Manager) GetPlugin(name string) (*PluginInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.plugins[name]
	return info, exists
}

// GetAllPlugins 모든 플러그인 정보 조회
func (m *Manager) GetAllPlugins() []*PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*PluginInfo, 0, len(m.plugins))
	for _, info := range m.plugins {
		result = append(result, info)
	}
	return result
}

// GetEnabledPlugins 활성화된 플러그인만 조회
func (m *Manager) GetEnabledPlugins() []*PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*PluginInfo
	for _, info := range m.plugins {
		if info.Status == StatusEnabled {
			result = append(result, info)
		}
	}
	return result
}

// Shutdown 모든 플러그인 종료
func (m *Manager) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, info := range m.plugins {
		if info.Status == StatusEnabled && info.Instance != nil {
			if err := info.Instance.Shutdown(); err != nil {
				m.logger.Warn("Plugin %s shutdown error: %v", name, err)
			}
		}
		info.Status = StatusDisabled
	}

	m.logger.Info("All plugins shutdown complete")
	return nil
}

// RunMigrations 플러그인 마이그레이션 실행
func (m *Manager) RunMigrations(name string) error {
	m.mu.RLock()
	info, exists := m.plugins[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	if info.IsBuiltIn {
		// 내장 플러그인은 별도 마이그레이션 디렉토리 사용
		return m.runBuiltInMigrations(name)
	}

	files, err := m.loader.GetMigrationFiles(info.Path)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return nil
	}

	// TODO: 마이그레이션 버전 추적 및 실행
	m.logger.Info("Found %d migration files for plugin %s", len(files), name)
	return nil
}

// runBuiltInMigrations 내장 플러그인 마이그레이션 실행
func (m *Manager) runBuiltInMigrations(name string) error {
	// 내장 플러그인 마이그레이션 경로: migration/plugins/{name}/
	// 실제 실행은 외부 마이그레이션 도구 또는 GORM AutoMigrate 사용
	m.logger.Info("Running built-in migrations for plugin: %s", name)
	return nil
}
