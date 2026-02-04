package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoader_LoadManifest(t *testing.T) {
	// 테스트용 임시 디렉토리 생성
	tempDir := t.TempDir()
	pluginDir := filepath.Join(tempDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	// 테스트용 plugin.yaml 생성
	manifestContent := `
name: test-plugin
version: 1.0.0
title: Test Plugin
description: A test plugin
author: Test Author
requires:
  angple: ">=1.0.0"
routes:
  - path: /test
    method: GET
    handler: TestHandler
    auth: none
`
	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// 로더 테스트
	loader := NewLoader(tempDir)
	manifest, err := loader.LoadManifest(pluginDir)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}

	// 검증
	if manifest.Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got '%s'", manifest.Name)
	}
	if manifest.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", manifest.Version)
	}
	if manifest.Title != "Test Plugin" {
		t.Errorf("expected title 'Test Plugin', got '%s'", manifest.Title)
	}
	if manifest.Requires.Angple != ">=1.0.0" {
		t.Errorf("expected requires.angple '>=1.0.0', got '%s'", manifest.Requires.Angple)
	}
	if len(manifest.Routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(manifest.Routes))
	}
}

func TestLoader_ValidateManifest_MissingName(t *testing.T) {
	tempDir := t.TempDir()
	pluginDir := filepath.Join(tempDir, "invalid-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	// name 필드 누락
	manifestContent := `
version: 1.0.0
title: Invalid Plugin
requires:
  angple: ">=1.0.0"
`
	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	loader := NewLoader(tempDir)
	_, err := loader.LoadManifest(pluginDir)
	if err == nil {
		t.Error("expected error for missing name, got nil")
	}
}

func TestLoader_DiscoverPlugins(t *testing.T) {
	tempDir := t.TempDir()

	// 2개의 플러그인 디렉토리 생성
	for _, name := range []string{"plugin-a", "plugin-b"} {
		pluginDir := filepath.Join(tempDir, name)
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			t.Fatalf("failed to create plugin dir: %v", err)
		}

		manifest := `
name: ` + name + `
version: 1.0.0
title: ` + name + `
requires:
  angple: ">=1.0.0"
`
		manifestPath := filepath.Join(pluginDir, "plugin.yaml")
		if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
			t.Fatalf("failed to write manifest: %v", err)
		}
	}

	// 빈 디렉토리 (plugin.yaml 없음)
	emptyDir := filepath.Join(tempDir, "empty-dir")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatalf("failed to create empty dir: %v", err)
	}

	loader := NewLoader(tempDir)
	plugins, err := loader.DiscoverPlugins()
	if err != nil {
		t.Fatalf("DiscoverPlugins failed: %v", err)
	}

	// plugin.yaml이 있는 디렉토리만 발견되어야 함
	if len(plugins) != 2 {
		t.Errorf("expected 2 plugins, got %d", len(plugins))
	}
}

func TestLoader_NonExistentDirectory(t *testing.T) {
	loader := NewLoader("/non/existent/path")
	plugins, err := loader.DiscoverPlugins()
	if err != nil {
		t.Errorf("expected no error for non-existent directory, got: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}
