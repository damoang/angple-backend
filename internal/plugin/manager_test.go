package plugin

import (
	"fmt"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MockPlugin 테스트용 목 플러그인
type MockPlugin struct {
	name        string
	initialized bool
	shutdown    bool
	routesCount int
}

func (m *MockPlugin) Name() string {
	return m.name
}

func (m *MockPlugin) Migrate(_ *gorm.DB) error {
	return nil
}

func (m *MockPlugin) Initialize(_ *PluginContext) error {
	m.initialized = true
	return nil
}

func (m *MockPlugin) RegisterRoutes(_ gin.IRouter) {
	m.routesCount++
}

func (m *MockPlugin) Shutdown() error {
	m.shutdown = true
	return nil
}

func TestManager_RegisterBuiltIn(t *testing.T) {
	logger := NewDefaultLogger("test")
	manager := NewManager("/tmp/plugins", nil, nil, logger, nil, nil)

	mockPlugin := &MockPlugin{name: "test-plugin"}
	manifest := &PluginManifest{
		Name:    "test-plugin",
		Version: "1.0.0",
		Title:   "Test Plugin",
		Requires: Requires{
			Angple: ">=1.0.0",
		},
	}

	err := manager.RegisterBuiltIn("test-plugin", mockPlugin, manifest)
	if err != nil {
		t.Fatalf("RegisterBuiltIn failed: %v", err)
	}

	// 플러그인이 등록되었는지 확인
	info, exists := manager.GetPlugin("test-plugin")
	if !exists {
		t.Error("plugin should exist after registration")
	}
	if info.IsBuiltIn != true {
		t.Error("plugin should be marked as built-in")
	}
	if info.Status != StatusDisabled {
		t.Errorf("expected status %s, got %s", StatusDisabled, info.Status)
	}
}

func TestManager_EnableDisable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logger := NewDefaultLogger("test")
	manager := NewManager("/tmp/plugins", nil, nil, logger, nil, nil)

	// 테스트용 라우터 설정
	router := gin.New()
	manager.GetRegistry().SetRouter(router)

	mockPlugin := &MockPlugin{name: "test-plugin"}
	manifest := &PluginManifest{
		Name:    "test-plugin",
		Version: "1.0.0",
		Title:   "Test Plugin",
		Requires: Requires{
			Angple: ">=1.0.0",
		},
	}

	// 등록
	_ = manager.RegisterBuiltIn("test-plugin", mockPlugin, manifest)

	// 활성화
	err := manager.Enable("test-plugin")
	if err != nil {
		t.Fatalf("Enable failed: %v", err)
	}

	info, _ := manager.GetPlugin("test-plugin")
	if info.Status != StatusEnabled {
		t.Errorf("expected status %s, got %s", StatusEnabled, info.Status)
	}
	if !mockPlugin.initialized {
		t.Error("plugin should be initialized after enable")
	}

	// 비활성화
	err = manager.Disable("test-plugin")
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}

	info, _ = manager.GetPlugin("test-plugin")
	if info.Status != StatusDisabled {
		t.Errorf("expected status %s, got %s", StatusDisabled, info.Status)
	}
	if !mockPlugin.shutdown {
		t.Error("plugin should be shutdown after disable")
	}
}

func TestManager_GetAllPlugins(t *testing.T) {
	logger := NewDefaultLogger("test")
	manager := NewManager("/tmp/plugins", nil, nil, logger, nil, nil)

	// 여러 플러그인 등록
	for _, name := range []string{"plugin-a", "plugin-b", "plugin-c"} {
		mockPlugin := &MockPlugin{name: name}
		manifest := &PluginManifest{
			Name:    name,
			Version: "1.0.0",
			Title:   name,
			Requires: Requires{
				Angple: ">=1.0.0",
			},
		}
		_ = manager.RegisterBuiltIn(name, mockPlugin, manifest)
	}

	all := manager.GetAllPlugins()
	if len(all) != 3 {
		t.Errorf("expected 3 plugins, got %d", len(all))
	}
}

func TestManager_EnableNonExistent(t *testing.T) {
	logger := NewDefaultLogger("test")
	manager := NewManager("/tmp/plugins", nil, nil, logger, nil, nil)

	err := manager.Enable("non-existent")
	if err == nil {
		t.Error("expected error for non-existent plugin")
	}
}

// HealthyPlugin implements HealthCheckable
type HealthyPlugin struct {
	MockPlugin
}

func (h *HealthyPlugin) HealthCheck() error { return nil }

// UnhealthyPlugin implements HealthCheckable with error
type UnhealthyPlugin struct {
	MockPlugin
}

func (u *UnhealthyPlugin) HealthCheck() error {
	return fmt.Errorf("database connection lost")
}

func TestCheckHealth_Healthy(t *testing.T) {
	logger := NewDefaultLogger("test")
	manager := NewManager("/tmp/plugins", nil, nil, logger, nil, nil)
	router := gin.New()
	manager.GetRegistry().SetRouter(router)

	p := &HealthyPlugin{MockPlugin{name: "healthy-plugin"}}
	manifest := &PluginManifest{Name: "healthy-plugin", Version: "1.0.0", Requires: Requires{Angple: ">=1.0.0"}}
	_ = manager.RegisterBuiltIn("healthy-plugin", p, manifest)
	_ = manager.Enable("healthy-plugin")

	result := manager.CheckHealth("healthy-plugin")
	if result.Status != "healthy" {
		t.Errorf("expected healthy, got %s", result.Status)
	}
}

func TestCheckHealth_Unhealthy(t *testing.T) {
	logger := NewDefaultLogger("test")
	manager := NewManager("/tmp/plugins", nil, nil, logger, nil, nil)
	router := gin.New()
	manager.GetRegistry().SetRouter(router)

	p := &UnhealthyPlugin{MockPlugin{name: "bad-plugin"}}
	manifest := &PluginManifest{Name: "bad-plugin", Version: "1.0.0", Requires: Requires{Angple: ">=1.0.0"}}
	_ = manager.RegisterBuiltIn("bad-plugin", p, manifest)
	_ = manager.Enable("bad-plugin")

	result := manager.CheckHealth("bad-plugin")
	if result.Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %s", result.Status)
	}
	if result.Message != "database connection lost" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestCheckHealth_Disabled(t *testing.T) {
	logger := NewDefaultLogger("test")
	manager := NewManager("/tmp/plugins", nil, nil, logger, nil, nil)

	manifest := &PluginManifest{Name: "disabled-plugin", Version: "1.0.0"}
	_ = manager.RegisterBuiltIn("disabled-plugin", &MockPlugin{name: "disabled-plugin"}, manifest)

	result := manager.CheckHealth("disabled-plugin")
	if result.Status != "disabled" {
		t.Errorf("expected disabled, got %s", result.Status)
	}
}

func TestCheckAllHealth(t *testing.T) {
	logger := NewDefaultLogger("test")
	manager := NewManager("/tmp/plugins", nil, nil, logger, nil, nil)
	router := gin.New()
	manager.GetRegistry().SetRouter(router)

	p1 := &HealthyPlugin{MockPlugin{name: "p1"}}
	m1 := &PluginManifest{Name: "p1", Version: "1.0.0", Requires: Requires{Angple: ">=1.0.0"}}
	_ = manager.RegisterBuiltIn("p1", p1, m1)
	_ = manager.Enable("p1")

	p2 := &MockPlugin{name: "p2"}
	m2 := &PluginManifest{Name: "p2", Version: "1.0.0"}
	_ = manager.RegisterBuiltIn("p2", p2, m2)

	results := manager.CheckAllHealth()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}
