package service

import (
	"testing"

	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"github.com/damoang/angple-backend/internal/pluginstore/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPermTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	db.AutoMigrate(&domain.PluginPermission{})
	return db
}

func TestPermissionService_SyncPermissions(t *testing.T) {
	db := setupPermTestDB(t)
	permRepo := repository.NewPermissionRepository(db)
	svc := NewPermissionService(permRepo, nil)

	perms := []plugin.Permission{
		{ID: "commerce.use", Label: "상점 이용"},
		{ID: "commerce.admin", Label: "상점 관리"},
	}

	if err := svc.SyncPermissions("commerce", perms); err != nil {
		t.Fatalf("SyncPermissions failed: %v", err)
	}

	// 조회 확인
	result, err := svc.GetPermissions("commerce")
	if err != nil {
		t.Fatalf("GetPermissions failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(result))
	}

	// 기본 min_level = 1
	for _, p := range result {
		if p.MinLevel != 1 {
			t.Errorf("expected default min_level 1, got %d for %s", p.MinLevel, p.PermissionID)
		}
	}
}

func TestPermissionService_SyncPreservesMinLevel(t *testing.T) {
	db := setupPermTestDB(t)
	permRepo := repository.NewPermissionRepository(db)
	svc := NewPermissionService(permRepo, nil)

	// 초기 동기화
	perms := []plugin.Permission{{ID: "commerce.admin", Label: "관리"}}
	svc.SyncPermissions("commerce", perms)

	// 관리자 레벨로 변경
	svc.UpdatePermissionLevel("commerce", "commerce.admin", 10)

	// 재동기화 (라벨 변경)
	perms[0].Label = "관리 (업데이트)"
	svc.SyncPermissions("commerce", perms)

	// min_level이 10으로 유지되는지 확인
	result, _ := svc.GetPermissions("commerce")
	if result[0].MinLevel != 10 {
		t.Errorf("expected min_level 10 after re-sync, got %d", result[0].MinLevel)
	}
	if result[0].Label != "관리 (업데이트)" {
		t.Errorf("expected updated label, got %q", result[0].Label)
	}
}

func TestPermissionService_CheckPermission(t *testing.T) {
	db := setupPermTestDB(t)
	permRepo := repository.NewPermissionRepository(db)
	svc := NewPermissionService(permRepo, nil)

	svc.SyncPermissions("commerce", []plugin.Permission{
		{ID: "commerce.use", Label: "이용"},
		{ID: "commerce.admin", Label: "관리"},
	})
	svc.UpdatePermissionLevel("commerce", "commerce.admin", 10)

	tests := []struct {
		perm  string
		level int
		want  bool
	}{
		{"commerce.use", 1, true},     // level 1 >= min 1
		{"commerce.use", 0, false},    // level 0 < min 1
		{"commerce.admin", 5, false},  // level 5 < min 10
		{"commerce.admin", 10, true},  // level 10 >= min 10
		{"commerce.unknown", 1, true}, // 정의되지 않은 권한 → 허용
	}

	for _, tc := range tests {
		got, err := svc.CheckPermission("commerce", tc.perm, tc.level)
		if err != nil {
			t.Errorf("CheckPermission(%q, %d) error: %v", tc.perm, tc.level, err)
			continue
		}
		if got != tc.want {
			t.Errorf("CheckPermission(%q, %d) = %v, want %v", tc.perm, tc.level, got, tc.want)
		}
	}
}

func TestPermissionService_UpdatePermissionLevel_InvalidRange(t *testing.T) {
	db := setupPermTestDB(t)
	permRepo := repository.NewPermissionRepository(db)
	svc := NewPermissionService(permRepo, nil)

	if err := svc.UpdatePermissionLevel("x", "x.use", 0); err == nil {
		t.Error("expected error for min_level 0")
	}
	if err := svc.UpdatePermissionLevel("x", "x.use", 11); err == nil {
		t.Error("expected error for min_level 11")
	}
}

func TestPermissionService_DeletePermissions(t *testing.T) {
	db := setupPermTestDB(t)
	permRepo := repository.NewPermissionRepository(db)
	svc := NewPermissionService(permRepo, nil)

	svc.SyncPermissions("commerce", []plugin.Permission{
		{ID: "commerce.use", Label: "이용"},
	})

	if err := svc.DeletePermissions("commerce"); err != nil {
		t.Fatalf("DeletePermissions failed: %v", err)
	}

	result, _ := svc.GetPermissions("commerce")
	if len(result) != 0 {
		t.Errorf("expected 0 permissions after delete, got %d", len(result))
	}
}
