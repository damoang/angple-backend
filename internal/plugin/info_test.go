package plugin

import (
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type infoTestPlugin struct{}

func (p *infoTestPlugin) Name() string                      { return "info-test" }
func (p *infoTestPlugin) Migrate(_ *gorm.DB) error          { return nil }
func (p *infoTestPlugin) Initialize(_ *PluginContext) error { return nil }
func (p *infoTestPlugin) RegisterRoutes(_ gin.IRouter)      {}
func (p *infoTestPlugin) Shutdown() error                   { return nil }

func TestGetOverview(t *testing.T) {
	logger := NewDefaultLogger("test")
	m := NewManager("plugins", nil, nil, logger, nil, nil)

	m.mu.Lock()
	m.plugins["alpha"] = &PluginInfo{
		Manifest:  &PluginManifest{Name: "alpha", Version: "1.0.0", Title: "Alpha"},
		Status:    StatusEnabled,
		IsBuiltIn: true,
		Instance:  &infoTestPlugin{},
	}
	m.plugins["beta"] = &PluginInfo{
		Manifest: &PluginManifest{Name: "beta", Version: "0.1.0", Title: "Beta"},
		Status:   StatusDisabled,
		Instance: &infoTestPlugin{},
	}
	m.mu.Unlock()

	overview := m.GetOverview()

	if overview.TotalPlugins != 2 {
		t.Fatalf("expected 2 total, got %d", overview.TotalPlugins)
	}
	if overview.EnabledCount != 1 {
		t.Fatalf("expected 1 enabled, got %d", overview.EnabledCount)
	}
	if overview.DisabledCount != 1 {
		t.Fatalf("expected 1 disabled, got %d", overview.DisabledCount)
	}
}

func TestGetPluginDetail(t *testing.T) {
	logger := NewDefaultLogger("test")
	m := NewManager("plugins", nil, nil, logger, nil, nil)

	m.mu.Lock()
	m.plugins["test-detail"] = &PluginInfo{
		Manifest: &PluginManifest{
			Name:    "test-detail",
			Version: "2.0.0",
			Title:   "Detail Test",
			Author:  "tester",
			Settings: []SettingConfig{
				{Key: "color", Type: "string"},
			},
		},
		Status:   StatusEnabled,
		Instance: &infoTestPlugin{},
	}
	m.mu.Unlock()

	detail := m.GetDetail("test-detail")
	if detail == nil {
		t.Fatal("expected detail, got nil")
	}
	if detail.Name != "test-detail" {
		t.Fatalf("expected test-detail, got %s", detail.Name)
	}
	if detail.Author != "tester" {
		t.Fatalf("expected tester, got %s", detail.Author)
	}
	if len(detail.Settings) != 1 {
		t.Fatalf("expected 1 setting, got %d", len(detail.Settings))
	}
	// nil 슬라이스가 아닌 빈 슬라이스인지 확인
	if detail.Routes == nil {
		t.Fatal("routes should not be nil")
	}
	if detail.Events == nil {
		t.Fatal("events should not be nil")
	}
}

func TestGetPluginDetail_NotFound(t *testing.T) {
	logger := NewDefaultLogger("test")
	m := NewManager("plugins", nil, nil, logger, nil, nil)

	detail := m.GetDetail("nonexistent")
	if detail != nil {
		t.Fatal("expected nil for nonexistent plugin")
	}
}
