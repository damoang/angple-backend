package domain

import "github.com/damoang/angple-backend/internal/plugin"

// CatalogEntry 내장 플러그인 카탈로그 항목
type CatalogEntry struct {
	Name         string                 `json:"name"`
	Version      string                 `json:"version"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Author       string                 `json:"author"`
	License      string                 `json:"license"`
	Category     string                 `json:"category"`
	Icon         string                 `json:"icon"`
	Tags         []string               `json:"tags"`
	Dependencies []string               `json:"dependencies"`
	Conflicts    []string               `json:"conflicts"`
	Settings     []plugin.SettingConfig `json:"settings"`
	// 런타임에 채워지는 필드
	IsInstalled bool   `json:"is_installed"`
	Status      string `json:"status"` // enabled, disabled, error, ""
}

// CatalogEntryFromManifest 매니페스트에서 카탈로그 항목 생성
func CatalogEntryFromManifest(m *plugin.PluginManifest) *CatalogEntry {
	deps := make([]string, 0, len(m.Requires.Plugins))
	for _, d := range m.Requires.Plugins {
		deps = append(deps, d.Name)
	}

	return &CatalogEntry{
		Name:         m.Name,
		Version:      m.Version,
		Title:        m.Title,
		Description:  m.Description,
		Author:       m.Author,
		License:      m.License,
		Conflicts:    m.Conflicts,
		Dependencies: deps,
		Settings:     m.Settings,
	}
}
