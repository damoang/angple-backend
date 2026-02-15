package plugin

import (
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type lifecycleMock struct {
	name        string
	installed   bool
	uninstalled bool
	enabled     bool
	disabled    bool
}

func (m *lifecycleMock) Name() string                      { return m.name }
func (m *lifecycleMock) Migrate(_ *gorm.DB) error          { return nil }
func (m *lifecycleMock) Initialize(_ *PluginContext) error { return nil }
func (m *lifecycleMock) RegisterRoutes(_ gin.IRouter)      {}
func (m *lifecycleMock) Shutdown() error                   { return nil }
func (m *lifecycleMock) OnInstall() error                  { m.installed = true; return nil }
func (m *lifecycleMock) OnUninstall() error                { m.uninstalled = true; return nil }
func (m *lifecycleMock) OnEnable() error                   { m.enabled = true; return nil }
func (m *lifecycleMock) OnDisable() error                  { m.disabled = true; return nil }

func setupLifecycleTest(t *testing.T) (*Manager, *lifecycleMock) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	logger := NewDefaultLogger("test")
	mgr := NewManager("", db, nil, logger, nil, nil)
	mock := &lifecycleMock{name: "lc-plugin"}
	manifest := &PluginManifest{Name: "lc-plugin", Version: "1.0.0"}
	mgr.RegisterBuiltIn("lc-plugin", mock, manifest)
	return mgr, mock
}

func TestLifecycle_OnEnable(t *testing.T) {
	mgr, mock := setupLifecycleTest(t)

	if err := mgr.Enable("lc-plugin"); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	if !mock.enabled {
		t.Error("expected OnEnable to be called")
	}
}

func TestLifecycle_OnDisable(t *testing.T) {
	mgr, mock := setupLifecycleTest(t)

	_ = mgr.Enable("lc-plugin")
	if err := mgr.Disable("lc-plugin"); err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
	if !mock.disabled {
		t.Error("expected OnDisable to be called")
	}
}

func TestLifecycle_OnInstall(t *testing.T) {
	mgr, mock := setupLifecycleTest(t)

	_ = mgr.Enable("lc-plugin")
	mgr.NotifyInstall("lc-plugin")
	if !mock.installed {
		t.Error("expected OnInstall to be called")
	}
}

func TestLifecycle_OnUninstall(t *testing.T) {
	mgr, mock := setupLifecycleTest(t)

	_ = mgr.Enable("lc-plugin")
	mgr.NotifyUninstall("lc-plugin")
	if !mock.uninstalled {
		t.Error("expected OnUninstall to be called")
	}
}

func TestLifecycle_NotImplemented(t *testing.T) {
	// Plugin that does NOT implement LifecycleAware should not panic
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	logger := NewDefaultLogger("test")
	mgr := NewManager("", db, nil, logger, nil, nil)

	type simplePlugin struct {
		lifecycleMockBase
	}
	simple := &simplePlugin{}
	simple.name = "simple"
	manifest := &PluginManifest{Name: "simple", Version: "1.0.0"}
	mgr.RegisterBuiltIn("simple", simple, manifest)

	// Should not panic
	if err := mgr.Enable("simple"); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	mgr.NotifyInstall("simple")
	mgr.NotifyUninstall("simple")
	if err := mgr.Disable("simple"); err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
}

type lifecycleMockBase struct {
	name string
}

func (m *lifecycleMockBase) Name() string                      { return m.name }
func (m *lifecycleMockBase) Migrate(_ *gorm.DB) error          { return nil }
func (m *lifecycleMockBase) Initialize(_ *PluginContext) error { return nil }
func (m *lifecycleMockBase) RegisterRoutes(_ gin.IRouter)      {}
func (m *lifecycleMockBase) Shutdown() error                   { return nil }
