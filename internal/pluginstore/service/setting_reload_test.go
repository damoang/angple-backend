package service

import (
	"testing"

	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"github.com/damoang/angple-backend/internal/pluginstore/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// mockReloader 리로더 모의 객체
type mockReloader struct {
	reloaded []string
}

func (m *mockReloader) ReloadPlugin(name string) error {
	m.reloaded = append(m.reloaded, name)
	return nil
}

func setupSettingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := db.AutoMigrate(&domain.PluginSetting{}, &domain.PluginEvent{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestSaveSettings_TriggersReload(t *testing.T) {
	db := setupSettingTestDB(t)
	settingRepo := repository.NewSettingRepository(db)
	eventRepo := repository.NewEventRepository(db)

	// CatalogService에 매니페스트 등록
	catalogSvc := NewCatalogService(nil)
	catalogSvc.RegisterManifest(&plugin.PluginManifest{
		Name: "test-plugin",
		Settings: []plugin.SettingConfig{
			{Key: "color", Type: "string", Label: "Color"},
		},
	})

	svc := NewSettingService(settingRepo, eventRepo, catalogSvc)

	reloader := &mockReloader{}
	svc.SetReloader(reloader)

	err := svc.SaveSettings("test-plugin", map[string]string{"color": "blue"}, "admin")
	if err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	if len(reloader.reloaded) != 1 || reloader.reloaded[0] != "test-plugin" {
		t.Fatalf("expected reload of test-plugin, got %v", reloader.reloaded)
	}
}

func TestSaveSettings_NoReloader(t *testing.T) {
	db := setupSettingTestDB(t)
	settingRepo := repository.NewSettingRepository(db)
	eventRepo := repository.NewEventRepository(db)

	catalogSvc := NewCatalogService(nil)
	catalogSvc.RegisterManifest(&plugin.PluginManifest{
		Name: "test-plugin",
		Settings: []plugin.SettingConfig{
			{Key: "color", Type: "string", Label: "Color"},
		},
	})

	svc := NewSettingService(settingRepo, eventRepo, catalogSvc)
	// No reloader set - should not panic

	err := svc.SaveSettings("test-plugin", map[string]string{"color": "red"}, "admin")
	if err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}
}
