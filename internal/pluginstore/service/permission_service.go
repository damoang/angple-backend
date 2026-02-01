package service

import (
	"fmt"

	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"github.com/damoang/angple-backend/internal/pluginstore/repository"
)

// PermissionService 플러그인 권한 관리 서비스
type PermissionService struct {
	permRepo   *repository.PermissionRepository
	catalogSvc *CatalogService
}

// NewPermissionService 생성자
func NewPermissionService(
	permRepo *repository.PermissionRepository,
	catalogSvc *CatalogService,
) *PermissionService {
	return &PermissionService{
		permRepo:   permRepo,
		catalogSvc: catalogSvc,
	}
}

// SyncPermissions 매니페스트의 권한 정의를 DB에 동기화
// 새 권한은 기본 min_level=1로 생성, 기존 권한의 min_level은 유지
func (s *PermissionService) SyncPermissions(pluginName string, permissions []plugin.Permission) error {
	for _, p := range permissions {
		perm := &domain.PluginPermission{
			PluginName:   pluginName,
			PermissionID: p.ID,
			Label:        p.Label,
			MinLevel:     1, // 기본값 (Upsert에서 기존 min_level 유지)
		}
		if err := s.permRepo.Upsert(perm); err != nil {
			return fmt.Errorf("failed to sync permission %s: %w", p.ID, err)
		}
	}
	return nil
}

// DeletePermissions 플러그인 권한 삭제 (언인스톨 시)
func (s *PermissionService) DeletePermissions(pluginName string) error {
	return s.permRepo.DeleteByPlugin(pluginName)
}

// CheckPermission 사용자가 특정 권한을 가지는지 확인
func (s *PermissionService) CheckPermission(pluginName, permissionID string, userLevel int) (bool, error) {
	perm, err := s.permRepo.GetByID(pluginName, permissionID)
	if err != nil {
		// 권한 정의가 없으면 허용 (정의되지 않은 권한은 체크하지 않음)
		return true, nil
	}
	return userLevel >= perm.MinLevel, nil
}

// GetPermissions 플러그인의 권한 목록 조회
func (s *PermissionService) GetPermissions(pluginName string) ([]domain.PluginPermission, error) {
	return s.permRepo.GetByPlugin(pluginName)
}

// UpdatePermissionLevel 권한의 최소 레벨 변경
func (s *PermissionService) UpdatePermissionLevel(pluginName, permissionID string, minLevel int) error {
	if minLevel < 1 || minLevel > 10 {
		return fmt.Errorf("min_level must be between 1 and 10")
	}
	return s.permRepo.UpdateMinLevel(pluginName, permissionID, minLevel)
}
