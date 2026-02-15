package repository

import (
	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PermissionRepository 플러그인 권한 저장소
type PermissionRepository struct {
	db *gorm.DB
}

// NewPermissionRepository 생성자
func NewPermissionRepository(db *gorm.DB) *PermissionRepository {
	return &PermissionRepository{db: db}
}

// Upsert 권한 생성 또는 업데이트 (label만 갱신, min_level은 유지)
func (r *PermissionRepository) Upsert(perm *domain.PluginPermission) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "plugin_name"}, {Name: "permission_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"label"}),
	}).Create(perm).Error
}

// UpdateMinLevel 권한의 최소 레벨 변경
func (r *PermissionRepository) UpdateMinLevel(pluginName, permissionID string, minLevel int) error {
	return r.db.Model(&domain.PluginPermission{}).
		Where("plugin_name = ? AND permission_id = ?", pluginName, permissionID).
		Update("min_level", minLevel).Error
}

// GetByPlugin 플러그인의 모든 권한 조회
func (r *PermissionRepository) GetByPlugin(pluginName string) ([]domain.PluginPermission, error) {
	var perms []domain.PluginPermission
	err := r.db.Where("plugin_name = ?", pluginName).Find(&perms).Error
	return perms, err
}

// GetByID 특정 권한 조회
func (r *PermissionRepository) GetByID(pluginName, permissionID string) (*domain.PluginPermission, error) {
	var perm domain.PluginPermission
	err := r.db.Where("plugin_name = ? AND permission_id = ?", pluginName, permissionID).First(&perm).Error
	if err != nil {
		return nil, err
	}
	return &perm, nil
}

// DeleteByPlugin 플러그인의 모든 권한 삭제
func (r *PermissionRepository) DeleteByPlugin(pluginName string) error {
	return r.db.Where("plugin_name = ?", pluginName).Delete(&domain.PluginPermission{}).Error
}
