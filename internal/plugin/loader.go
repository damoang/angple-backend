package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Loader 플러그인 로더
type Loader struct {
	pluginsDir string
}

// NewLoader 새 로더 생성
func NewLoader(pluginsDir string) *Loader {
	return &Loader{
		pluginsDir: pluginsDir,
	}
}

// LoadManifest plugin.yaml 파일 로드
func (l *Loader) LoadManifest(pluginPath string) (*PluginManifest, error) {
	manifestPath := filepath.Join(pluginPath, "plugin.yaml")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin.yaml: %w", err)
	}

	var manifest PluginManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse plugin.yaml: %w", err)
	}

	// 필수 필드 검증
	if err := l.validateManifest(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// validateManifest 필수 필드 검증
func (l *Loader) validateManifest(m *PluginManifest) error {
	if m.Name == "" {
		return fmt.Errorf("plugin name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("plugin version is required")
	}
	if m.Title == "" {
		return fmt.Errorf("plugin title is required")
	}
	if m.Requires.Angple == "" {
		return fmt.Errorf("requires.angple version is required")
	}
	return nil
}

// DiscoverPlugins 플러그인 디렉토리에서 모든 플러그인 검색
func (l *Loader) DiscoverPlugins() ([]*PluginInfo, error) {
	var plugins []*PluginInfo

	// 플러그인 디렉토리가 없으면 빈 슬라이스 반환
	if _, err := os.Stat(l.pluginsDir); os.IsNotExist(err) {
		return plugins, nil
	}

	entries, err := os.ReadDir(l.pluginsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugins directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginPath := filepath.Join(l.pluginsDir, entry.Name())
		manifestPath := filepath.Join(pluginPath, "plugin.yaml")

		// plugin.yaml이 없으면 스킵
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		manifest, err := l.LoadManifest(pluginPath)
		if err != nil {
			plugins = append(plugins, &PluginInfo{
				Path:   pluginPath,
				Status: StatusError,
				Error:  err,
			})
			continue
		}

		plugins = append(plugins, &PluginInfo{
			Manifest: manifest,
			Path:     pluginPath,
			Status:   StatusDisabled,
		})
	}

	return plugins, nil
}

// LoadPluginManifestByName 이름으로 플러그인 매니페스트 로드
func (l *Loader) LoadPluginManifestByName(name string) (*PluginManifest, string, error) {
	pluginPath := filepath.Join(l.pluginsDir, name)

	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		return nil, "", fmt.Errorf("plugin %s not found", name)
	}

	manifest, err := l.LoadManifest(pluginPath)
	if err != nil {
		return nil, "", err
	}

	return manifest, pluginPath, nil
}

// GetMigrationFiles 마이그레이션 파일 경로 반환
func (l *Loader) GetMigrationFiles(pluginPath string) ([]string, error) {
	migrationsDir := filepath.Join(pluginPath, "migrations")

	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		return nil, nil // 마이그레이션 디렉토리 없으면 빈 슬라이스
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			files = append(files, filepath.Join(migrationsDir, entry.Name()))
		}
	}

	return files, nil
}
