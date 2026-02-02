package service

import (
	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"github.com/damoang/angple-backend/internal/pluginstore/repository"
)

// CatalogService 플러그인 카탈로그 서비스
type CatalogService struct {
	installRepo *repository.InstallationRepository
	manifests   map[string]*plugin.PluginManifest
}

// NewCatalogService 생성자
func NewCatalogService(installRepo *repository.InstallationRepository) *CatalogService {
	return &CatalogService{
		installRepo: installRepo,
		manifests:   make(map[string]*plugin.PluginManifest),
	}
}

// RegisterManifest 내장 플러그인 매니페스트 등록
func (s *CatalogService) RegisterManifest(manifest *plugin.PluginManifest) {
	s.manifests[manifest.Name] = manifest
}

// ListCatalog 카탈로그 목록 조회 (DB 설치 상태 포함)
func (s *CatalogService) ListCatalog() ([]*domain.CatalogEntry, error) {
	installations, err := s.installRepo.FindAll()
	if err != nil {
		installations = nil // DB 에러시에도 카탈로그는 보여줌
	}

	installMap := make(map[string]*domain.PluginInstallation)
	for i := range installations {
		installMap[installations[i].PluginName] = &installations[i]
	}

	entries := make([]*domain.CatalogEntry, 0, len(s.manifests))
	for _, m := range s.manifests {
		entry := domain.CatalogEntryFromManifest(m)
		if inst, ok := installMap[m.Name]; ok {
			entry.IsInstalled = true
			entry.Status = inst.Status
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// GetCatalogEntry 특정 플러그인 카탈로그 항목 조회
func (s *CatalogService) GetCatalogEntry(name string) (*domain.CatalogEntry, error) {
	m, ok := s.manifests[name]
	if !ok {
		return nil, nil
	}

	entry := domain.CatalogEntryFromManifest(m)

	inst, err := s.installRepo.FindByName(name)
	if err == nil && inst != nil {
		entry.IsInstalled = true
		entry.Status = inst.Status
	}

	return entry, nil
}

// GetManifest 매니페스트 조회
func (s *CatalogService) GetManifest(name string) *plugin.PluginManifest {
	return s.manifests[name]
}

// ListManifests 전체 매니페스트 목록 반환
func (s *CatalogService) ListManifests() []*plugin.PluginManifest {
	result := make([]*plugin.PluginManifest, 0, len(s.manifests))
	for _, m := range s.manifests {
		result = append(result, m)
	}
	return result
}
