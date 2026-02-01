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
	loader      *Loader
	registry    *Registry
	hookManager *HookManager
	db          *gorm.DB
	redis       *redis.Client
	plugins     map[string]*PluginInfo
	mu          sync.RWMutex
	logger      Logger
}

// NewManager 새 매니저 생성
func NewManager(pluginsDir string, db *gorm.DB, redisClient *redis.Client, logger Logger) *Manager {
	return &Manager{
		loader:      NewLoader(pluginsDir),
		registry:    NewRegistry(),
		hookManager: NewHookManager(logger),
		db:          db,
		redis:       redisClient,
		plugins:     make(map[string]*PluginInfo),
		logger:      logger,
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

	// 플러그인 인스턴스가 있으면 마이그레이션 → 초기화
	if info.Instance != nil {
		// 1) 마이그레이션 먼저
		if err := info.Instance.Migrate(m.db); err != nil {
			info.Status = StatusError
			info.Error = err
			return fmt.Errorf("failed to migrate plugin %s: %w", name, err)
		}
		info.MigratedAt = time.Now().Unix()

		// 2) 초기화
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

		// 3) Hook 등록 (HookAware 구현 시)
		if ha, ok := info.Instance.(HookAware); ok {
			ha.RegisterHooks(m.hookManager)
			m.logger.Info("Registered hooks for plugin: %s", name)
		}

		// 4) 라우트 등록
		m.registry.RegisterPlugin(info)
	}

	// 플러그인 메뉴 등록
	if info.Manifest != nil && len(info.Manifest.Menus) > 0 {
		if err := m.registerPluginMenus(name, info.Manifest.Menus); err != nil {
			m.logger.Warn("Failed to register menus for plugin %s: %v", name, err)
		}
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

	// Hook 해제
	m.hookManager.Unregister(name)

	// 라우트 해제
	m.registry.UnregisterPlugin(name)

	// 플러그인 메뉴 비활성화
	if err := m.disablePluginMenus(name); err != nil {
		m.logger.Warn("Failed to disable menus for plugin %s: %v", name, err)
	}

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
	m.mu.RLock()
	info, exists := m.plugins[name]
	m.mu.RUnlock()

	if !exists || info.Instance == nil {
		return fmt.Errorf("built-in plugin %s not found or has no instance", name)
	}

	// 플러그인 자체의 Migrate() 호출
	return info.Instance.Migrate(m.db)
}

// ============================================
// 플러그인 메뉴 관리
// ============================================

// PluginMenu 플러그인 메뉴 DB 모델 (domain.Menu와 동일한 테이블 사용)
type PluginMenu struct {
	ID            int64   `gorm:"column:id;primaryKey"`
	ParentID      *int64  `gorm:"column:parent_id"`
	Title         string  `gorm:"column:title"`
	URL           string  `gorm:"column:url"`
	Icon          string  `gorm:"column:icon"`
	Depth         int     `gorm:"column:depth"`
	OrderNum      int     `gorm:"column:order_num"`
	IsActive      bool    `gorm:"column:is_active"`
	Target        string  `gorm:"column:target"`
	ViewLevel     int     `gorm:"column:view_level"`
	ShowInHeader  bool    `gorm:"column:show_in_header"`
	ShowInSidebar bool    `gorm:"column:show_in_sidebar"`
	PluginName    *string `gorm:"column:plugin_name"`
}

func (PluginMenu) TableName() string {
	return "menus"
}

// registerPluginMenus 플러그인 메뉴 등록
func (m *Manager) registerPluginMenus(pluginName string, menus []MenuConfig) error {
	if m.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// URL -> DB ID 매핑 (부모 메뉴 찾기용)
	urlToID := make(map[string]int64)

	// 먼저 기존 플러그인 메뉴 확인 (이미 존재하면 활성화만)
	var existingCount int64
	m.db.Model(&PluginMenu{}).Where("plugin_name = ?", pluginName).Count(&existingCount)

	if existingCount > 0 {
		// 이미 존재하면 활성화만
		if err := m.db.Model(&PluginMenu{}).Where("plugin_name = ?", pluginName).Update("is_active", true).Error; err != nil {
			return fmt.Errorf("failed to activate plugin menus: %w", err)
		}
		m.logger.Info("Activated %d existing menus for plugin: %s", existingCount, pluginName)
		return nil
	}

	// 새 메뉴 등록 (2패스: 1. 루트 메뉴, 2. 자식 메뉴)
	// Pass 1: 부모가 없는 메뉴 (루트) 먼저 등록
	for _, menuCfg := range menus {
		if menuCfg.ParentPath != "" {
			continue // 자식 메뉴는 나중에
		}

		viewLevel := menuCfg.ViewLevel
		if viewLevel == 0 {
			viewLevel = 1 // 기본값
		}

		menu := &PluginMenu{
			Title:         menuCfg.Title,
			URL:           menuCfg.URL,
			Icon:          menuCfg.Icon,
			Depth:         0, // 루트 메뉴
			OrderNum:      menuCfg.OrderNum,
			IsActive:      true,
			Target:        "_self",
			ViewLevel:     viewLevel,
			ShowInHeader:  menuCfg.ShowInHeader,
			ShowInSidebar: menuCfg.ShowInSidebar,
			PluginName:    &pluginName,
		}

		if err := m.db.Create(menu).Error; err != nil {
			return fmt.Errorf("failed to create menu %s: %w", menuCfg.Title, err)
		}

		urlToID[menuCfg.URL] = menu.ID
		m.logger.Info("Created root menu: %s (ID: %d)", menuCfg.Title, menu.ID)
	}

	// Pass 2: 자식 메뉴 등록
	for _, menuCfg := range menus {
		if menuCfg.ParentPath == "" {
			continue // 루트 메뉴는 이미 등록됨
		}

		// 부모 메뉴 ID 찾기
		var parentID *int64
		if pid, ok := urlToID[menuCfg.ParentPath]; ok {
			parentID = &pid
		} else {
			// DB에서 부모 메뉴 찾기
			var parentMenu PluginMenu
			if err := m.db.Where("url = ?", menuCfg.ParentPath).First(&parentMenu).Error; err == nil {
				parentID = &parentMenu.ID
			} else {
				m.logger.Warn("Parent menu not found for %s (parent: %s)", menuCfg.URL, menuCfg.ParentPath)
			}
		}

		viewLevel := menuCfg.ViewLevel
		if viewLevel == 0 {
			viewLevel = 1
		}

		depth := 1
		if parentID != nil {
			// 부모의 depth + 1
			var parentMenu PluginMenu
			if err := m.db.First(&parentMenu, *parentID).Error; err == nil {
				depth = parentMenu.Depth + 1
			}
		}

		menu := &PluginMenu{
			ParentID:      parentID,
			Title:         menuCfg.Title,
			URL:           menuCfg.URL,
			Icon:          menuCfg.Icon,
			Depth:         depth,
			OrderNum:      menuCfg.OrderNum,
			IsActive:      true,
			Target:        "_self",
			ViewLevel:     viewLevel,
			ShowInHeader:  menuCfg.ShowInHeader,
			ShowInSidebar: menuCfg.ShowInSidebar,
			PluginName:    &pluginName,
		}

		if err := m.db.Create(menu).Error; err != nil {
			return fmt.Errorf("failed to create menu %s: %w", menuCfg.Title, err)
		}

		urlToID[menuCfg.URL] = menu.ID
		m.logger.Info("Created child menu: %s (ID: %d, ParentID: %v)", menuCfg.Title, menu.ID, parentID)
	}

	m.logger.Info("Registered %d menus for plugin: %s", len(menus), pluginName)
	return nil
}

// disablePluginMenus 플러그인 메뉴 비활성화
func (m *Manager) disablePluginMenus(pluginName string) error {
	if m.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// 플러그인 메뉴를 비활성화 (삭제하지 않음)
	result := m.db.Model(&PluginMenu{}).
		Where("plugin_name = ?", pluginName).
		Update("is_active", false)

	if result.Error != nil {
		return fmt.Errorf("failed to disable plugin menus: %w", result.Error)
	}

	m.logger.Info("Disabled %d menus for plugin: %s", result.RowsAffected, pluginName)
	return nil
}

// GetHookManager HookManager 반환
func (m *Manager) GetHookManager() *HookManager {
	return m.hookManager
}

// GetEnabledPluginNames 활성화된 플러그인 이름 목록 반환
func (m *Manager) GetEnabledPluginNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var names []string
	for name, info := range m.plugins {
		if info.Status == StatusEnabled {
			names = append(names, name)
		}
	}
	return names
}
