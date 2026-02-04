package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("DB 생성 실패: %v", err)
	}
	db.AutoMigrate(&PluginMigrationRecord{})
	return db
}

func setupMigrationFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	migDir := filepath.Join(dir, "migrations")
	os.MkdirAll(migDir, 0o755)
	for name, content := range files {
		os.WriteFile(filepath.Join(migDir, name), []byte(content), 0o644)
	}
}

func TestRunMigrations_ExecutesSQL(t *testing.T) {
	db := setupMigrationTestDB(t)
	logger := NewDefaultLogger("test")

	// 임시 플러그인 디렉토리 생성
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	os.MkdirAll(pluginDir, 0o755)

	// 마이그레이션 파일 생성
	setupMigrationFiles(t, pluginDir, map[string]string{
		"001_init.up.sql":      "CREATE TABLE test_items (id INTEGER PRIMARY KEY, name TEXT);",
		"002_add_field.up.sql": "ALTER TABLE test_items ADD COLUMN price REAL;",
	})

	manager := NewManager(tmpDir, db, nil, logger, nil, nil)
	manager.mu.Lock()
	manager.plugins["test-plugin"] = &PluginInfo{
		Manifest: &PluginManifest{Name: "test-plugin", Version: "1.0.0"},
		Path:     pluginDir,
		Status:   StatusDisabled,
	}
	manager.mu.Unlock()

	// 마이그레이션 실행
	if err := manager.RunMigrations("test-plugin"); err != nil {
		t.Fatalf("RunMigrations 실패: %v", err)
	}

	// 테이블이 생성되었는지 확인
	var count int64
	db.Raw("SELECT COUNT(*) FROM test_items").Scan(&count)
	// 에러 없으면 테이블 존재

	// 이력 확인
	var records []PluginMigrationRecord
	db.Where("plugin_name = ?", "test-plugin").Order("filename").Find(&records)
	if len(records) != 2 {
		t.Fatalf("마이그레이션 이력 2건 예상, %d건 조회됨", len(records))
	}
	if records[0].Filename != "001_init.up.sql" {
		t.Errorf("첫 번째 이력 파일명: %s", records[0].Filename)
	}
	if records[1].Filename != "002_add_field.up.sql" {
		t.Errorf("두 번째 이력 파일명: %s", records[1].Filename)
	}
}

func TestRunMigrations_SkipsAlreadyExecuted(t *testing.T) {
	db := setupMigrationTestDB(t)
	logger := NewDefaultLogger("test")

	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "test-plugin")
	os.MkdirAll(pluginDir, 0o755)

	setupMigrationFiles(t, pluginDir, map[string]string{
		"001_init.up.sql": "CREATE TABLE skip_test (id INTEGER PRIMARY KEY);",
	})

	manager := NewManager(tmpDir, db, nil, logger, nil, nil)
	manager.mu.Lock()
	manager.plugins["test-plugin"] = &PluginInfo{
		Manifest: &PluginManifest{Name: "test-plugin", Version: "1.0.0"},
		Path:     pluginDir,
		Status:   StatusDisabled,
	}
	manager.mu.Unlock()

	// 첫 번째 실행
	if err := manager.RunMigrations("test-plugin"); err != nil {
		t.Fatalf("첫 번째 RunMigrations 실패: %v", err)
	}

	// 두 번째 실행 (스킵되어야 함)
	if err := manager.RunMigrations("test-plugin"); err != nil {
		t.Fatalf("두 번째 RunMigrations 실패: %v", err)
	}

	// 이력은 1건만
	var records []PluginMigrationRecord
	db.Where("plugin_name = ?", "test-plugin").Find(&records)
	if len(records) != 1 {
		t.Errorf("이력 1건 예상, %d건 조회됨", len(records))
	}
}

func TestRunMigrations_NoFiles(t *testing.T) {
	db := setupMigrationTestDB(t)
	logger := NewDefaultLogger("test")

	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "empty-plugin")
	os.MkdirAll(pluginDir, 0o755)

	manager := NewManager(tmpDir, db, nil, logger, nil, nil)
	manager.mu.Lock()
	manager.plugins["empty-plugin"] = &PluginInfo{
		Manifest: &PluginManifest{Name: "empty-plugin", Version: "1.0.0"},
		Path:     pluginDir,
		Status:   StatusDisabled,
	}
	manager.mu.Unlock()

	// 마이그레이션 없으면 정상 반환
	if err := manager.RunMigrations("empty-plugin"); err != nil {
		t.Fatalf("빈 마이그레이션 실패: %v", err)
	}
}

func TestRunMigrations_PluginNotFound(t *testing.T) {
	db := setupMigrationTestDB(t)
	logger := NewDefaultLogger("test")

	manager := NewManager(t.TempDir(), db, nil, logger, nil, nil)

	if err := manager.RunMigrations("nonexistent"); err == nil {
		t.Error("존재하지 않는 플러그인에 대해 에러 예상")
	}
}
