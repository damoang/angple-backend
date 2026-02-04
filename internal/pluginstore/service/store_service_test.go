package service

import (
	"testing"

	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"github.com/damoang/angple-backend/internal/pluginstore/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	db.AutoMigrate(&domain.PluginInstallation{}, &domain.PluginSetting{}, &domain.PluginEvent{})
	return db
}

func setupStoreService(t *testing.T) (*StoreService, *CatalogService, *plugin.Manager) {
	t.Helper()
	db := setupTestDB(t)

	installRepo := repository.NewInstallationRepository(db)
	eventRepo := repository.NewEventRepository(db)
	settingRepo := repository.NewSettingRepository(db)

	catalogSvc := NewCatalogService(installRepo)
	testManifest := &plugin.PluginManifest{
		Name:    "test-plugin",
		Version: "1.0.0",
		Title:   "Test Plugin",
	}
	catalogSvc.RegisterManifest(testManifest)

	logger := plugin.NewDefaultLogger("test")
	storeSvc := NewStoreService(installRepo, eventRepo, settingRepo, catalogSvc, logger)

	manager := plugin.NewManager("", db, nil, logger, nil, nil)

	// 내장 플러그인 등록
	mockPlugin := &mockPlugin{name: "test-plugin"}
	manager.RegisterBuiltIn("test-plugin", mockPlugin, testManifest)

	return storeSvc, catalogSvc, manager
}

type mockPlugin struct {
	name        string
	initialized bool
	shutdown    bool
}

func (m *mockPlugin) Name() string                               { return m.name }
func (m *mockPlugin) Migrate(db *gorm.DB) error                  { return nil }
func (m *mockPlugin) Initialize(ctx *plugin.PluginContext) error { m.initialized = true; return nil }
func (m *mockPlugin) RegisterRoutes(router gin.IRouter)          {}
func (m *mockPlugin) Shutdown() error                            { m.shutdown = true; return nil }

func TestInstallPlugin(t *testing.T) {
	storeSvc, _, manager := setupStoreService(t)

	err := storeSvc.Install("test-plugin", "admin1", manager)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	inst, err := storeSvc.GetInstallation("test-plugin")
	if err != nil {
		t.Fatalf("GetInstallation failed: %v", err)
	}
	if inst.Status != domain.StatusEnabled {
		t.Errorf("expected status enabled, got %s", inst.Status)
	}
}

func TestInstallDuplicate(t *testing.T) {
	storeSvc, _, manager := setupStoreService(t)

	_ = storeSvc.Install("test-plugin", "admin1", manager)
	err := storeSvc.Install("test-plugin", "admin1", manager)
	if err == nil {
		t.Fatal("expected error for duplicate install")
	}
}

func TestDisableEnable(t *testing.T) {
	storeSvc, _, manager := setupStoreService(t)

	_ = storeSvc.Install("test-plugin", "admin1", manager)

	// Disable
	err := storeSvc.Disable("test-plugin", "admin1", manager)
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
	inst, _ := storeSvc.GetInstallation("test-plugin")
	if inst.Status != domain.StatusDisabled {
		t.Errorf("expected disabled, got %s", inst.Status)
	}

	// Enable
	err = storeSvc.Enable("test-plugin", "admin1", manager)
	if err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	inst, _ = storeSvc.GetInstallation("test-plugin")
	if inst.Status != domain.StatusEnabled {
		t.Errorf("expected enabled, got %s", inst.Status)
	}
}

func TestUninstall(t *testing.T) {
	storeSvc, _, manager := setupStoreService(t)

	_ = storeSvc.Install("test-plugin", "admin1", manager)

	err := storeSvc.Uninstall("test-plugin", "admin1", manager)
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	_, err = storeSvc.GetInstallation("test-plugin")
	if err == nil {
		t.Fatal("expected error after uninstall")
	}
}

func TestInstallNotInCatalog(t *testing.T) {
	storeSvc, _, manager := setupStoreService(t)

	err := storeSvc.Install("nonexistent", "admin1", manager)
	if err == nil {
		t.Fatal("expected error for unknown plugin")
	}
}

func TestBootEnabledPlugins(t *testing.T) {
	storeSvc, _, manager := setupStoreService(t)

	_ = storeSvc.Install("test-plugin", "admin1", manager)

	// 새 매니저로 부팅 시뮬레이션
	logger := plugin.NewDefaultLogger("test")
	newManager := plugin.NewManager("", setupTestDB(t), nil, logger, nil, nil)
	mockP := &mockPlugin{name: "test-plugin"}
	testManifest := &plugin.PluginManifest{Name: "test-plugin", Version: "1.0.0"}
	newManager.RegisterBuiltIn("test-plugin", mockP, testManifest)

	err := storeSvc.BootEnabledPlugins(newManager)
	if err != nil {
		t.Fatalf("BootEnabledPlugins failed: %v", err)
	}

	if !mockP.initialized {
		t.Error("expected plugin to be initialized after boot")
	}
}

func TestConflictCheck(t *testing.T) {
	db := setupTestDB(t)
	installRepo := repository.NewInstallationRepository(db)
	eventRepo := repository.NewEventRepository(db)
	settingRepo := repository.NewSettingRepository(db)

	catalogSvc := NewCatalogService(installRepo)

	pluginA := &plugin.PluginManifest{Name: "plugin-a", Version: "1.0.0"}
	pluginB := &plugin.PluginManifest{Name: "plugin-b", Version: "1.0.0", Conflicts: []string{"plugin-a"}}
	catalogSvc.RegisterManifest(pluginA)
	catalogSvc.RegisterManifest(pluginB)

	logger := plugin.NewDefaultLogger("test")
	storeSvc := NewStoreService(installRepo, eventRepo, settingRepo, catalogSvc, logger)

	manager := plugin.NewManager("", db, nil, logger, nil, nil)
	manager.RegisterBuiltIn("plugin-a", &mockPlugin{name: "plugin-a"}, pluginA)
	manager.RegisterBuiltIn("plugin-b", &mockPlugin{name: "plugin-b"}, pluginB)

	_ = storeSvc.Install("plugin-a", "admin", manager)

	err := storeSvc.Install("plugin-b", "admin", manager)
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestConflictBidirectional(t *testing.T) {
	db := setupTestDB(t)
	installRepo := repository.NewInstallationRepository(db)
	eventRepo := repository.NewEventRepository(db)
	settingRepo := repository.NewSettingRepository(db)

	catalogSvc := NewCatalogService(installRepo)

	// plugin-x에서 plugin-y를 충돌로 선언하지 않았지만
	// plugin-y에서 plugin-x를 충돌로 선언 → plugin-x 활성화 시에도 차단되어야 함
	pluginX := &plugin.PluginManifest{Name: "plugin-x", Version: "1.0.0"}
	pluginY := &plugin.PluginManifest{Name: "plugin-y", Version: "1.0.0", Conflicts: []string{"plugin-x"}}
	catalogSvc.RegisterManifest(pluginX)
	catalogSvc.RegisterManifest(pluginY)

	logger := plugin.NewDefaultLogger("test")
	storeSvc := NewStoreService(installRepo, eventRepo, settingRepo, catalogSvc, logger)

	manager := plugin.NewManager("", db, nil, logger, nil, nil)
	manager.RegisterBuiltIn("plugin-x", &mockPlugin{name: "plugin-x"}, pluginX)
	manager.RegisterBuiltIn("plugin-y", &mockPlugin{name: "plugin-y"}, pluginY)

	// plugin-y 먼저 설치 (활성)
	_ = storeSvc.Install("plugin-y", "admin", manager)

	// plugin-x 설치 시도 → plugin-y가 plugin-x를 충돌로 선언했으므로 차단
	err := storeSvc.Install("plugin-x", "admin", manager)
	if err == nil {
		t.Fatal("expected conflict error from bidirectional check")
	}
}

func TestNoConflict(t *testing.T) {
	db := setupTestDB(t)
	installRepo := repository.NewInstallationRepository(db)
	eventRepo := repository.NewEventRepository(db)
	settingRepo := repository.NewSettingRepository(db)

	catalogSvc := NewCatalogService(installRepo)

	pluginA := &plugin.PluginManifest{Name: "plugin-a", Version: "1.0.0"}
	pluginC := &plugin.PluginManifest{Name: "plugin-c", Version: "1.0.0"}
	catalogSvc.RegisterManifest(pluginA)
	catalogSvc.RegisterManifest(pluginC)

	logger := plugin.NewDefaultLogger("test")
	storeSvc := NewStoreService(installRepo, eventRepo, settingRepo, catalogSvc, logger)

	manager := plugin.NewManager("", db, nil, logger, nil, nil)
	manager.RegisterBuiltIn("plugin-a", &mockPlugin{name: "plugin-a"}, pluginA)
	manager.RegisterBuiltIn("plugin-c", &mockPlugin{name: "plugin-c"}, pluginC)

	_ = storeSvc.Install("plugin-a", "admin", manager)
	err := storeSvc.Install("plugin-c", "admin", manager)
	if err != nil {
		t.Fatalf("expected no conflict, got: %v", err)
	}
}

func TestDisableBlockedByDependency(t *testing.T) {
	db := setupTestDB(t)
	installRepo := repository.NewInstallationRepository(db)
	eventRepo := repository.NewEventRepository(db)
	settingRepo := repository.NewSettingRepository(db)

	catalogSvc := NewCatalogService(installRepo)

	basePlugin := &plugin.PluginManifest{Name: "base-plugin", Version: "1.0.0"}
	childPlugin := &plugin.PluginManifest{
		Name:    "child-plugin",
		Version: "1.0.0",
		Requires: plugin.Requires{
			Plugins: []plugin.PluginDependency{{Name: "base-plugin", Version: ">=1.0.0"}},
		},
	}
	catalogSvc.RegisterManifest(basePlugin)
	catalogSvc.RegisterManifest(childPlugin)

	logger := plugin.NewDefaultLogger("test")
	storeSvc := NewStoreService(installRepo, eventRepo, settingRepo, catalogSvc, logger)

	manager := plugin.NewManager("", db, nil, logger, nil, nil)
	manager.RegisterBuiltIn("base-plugin", &mockPlugin{name: "base-plugin"}, basePlugin)
	manager.RegisterBuiltIn("child-plugin", &mockPlugin{name: "child-plugin"}, childPlugin)

	_ = storeSvc.Install("base-plugin", "admin", manager)
	_ = storeSvc.Install("child-plugin", "admin", manager)

	// base-plugin 비활성화 시도 → child-plugin이 의존하므로 차단
	err := storeSvc.Disable("base-plugin", "admin", manager)
	if err == nil {
		t.Fatal("expected error: cannot disable base-plugin while child-plugin depends on it")
	}
}

func TestUninstallBlockedByDependency(t *testing.T) {
	db := setupTestDB(t)
	installRepo := repository.NewInstallationRepository(db)
	eventRepo := repository.NewEventRepository(db)
	settingRepo := repository.NewSettingRepository(db)

	catalogSvc := NewCatalogService(installRepo)

	basePlugin := &plugin.PluginManifest{Name: "base-plugin", Version: "1.0.0"}
	childPlugin := &plugin.PluginManifest{
		Name:    "child-plugin",
		Version: "1.0.0",
		Requires: plugin.Requires{
			Plugins: []plugin.PluginDependency{{Name: "base-plugin", Version: ">=1.0.0"}},
		},
	}
	catalogSvc.RegisterManifest(basePlugin)
	catalogSvc.RegisterManifest(childPlugin)

	logger := plugin.NewDefaultLogger("test")
	storeSvc := NewStoreService(installRepo, eventRepo, settingRepo, catalogSvc, logger)

	manager := plugin.NewManager("", db, nil, logger, nil, nil)
	manager.RegisterBuiltIn("base-plugin", &mockPlugin{name: "base-plugin"}, basePlugin)
	manager.RegisterBuiltIn("child-plugin", &mockPlugin{name: "child-plugin"}, childPlugin)

	_ = storeSvc.Install("base-plugin", "admin", manager)
	_ = storeSvc.Install("child-plugin", "admin", manager)

	// base-plugin 삭제 시도 → child-plugin이 의존하므로 차단
	err := storeSvc.Uninstall("base-plugin", "admin", manager)
	if err == nil {
		t.Fatal("expected error: cannot uninstall base-plugin while child-plugin depends on it")
	}

	// child-plugin 먼저 삭제 → 그 후 base-plugin 삭제 가능
	_ = storeSvc.Uninstall("child-plugin", "admin", manager)
	err = storeSvc.Uninstall("base-plugin", "admin", manager)
	if err != nil {
		t.Fatalf("expected base-plugin uninstall to succeed after child removed: %v", err)
	}
}

func TestGetEvents(t *testing.T) {
	storeSvc, _, manager := setupStoreService(t)

	_ = storeSvc.Install("test-plugin", "admin1", manager)

	events, err := storeSvc.GetEvents("test-plugin", 10)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	if len(events) == 0 {
		t.Error("expected at least one event after install")
	}
}
