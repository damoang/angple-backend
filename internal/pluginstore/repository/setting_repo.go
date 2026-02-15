package repository

import (
	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SettingRepository 플러그인 설정 저장소
type SettingRepository struct {
	db *gorm.DB
}

// NewSettingRepository 생성자
func NewSettingRepository(db *gorm.DB) *SettingRepository {
	return &SettingRepository{db: db}
}

// Get 특정 설정 조회
func (r *SettingRepository) Get(pluginName, key string) (*domain.PluginSetting, error) {
	var s domain.PluginSetting
	err := r.db.Where("plugin_name = ? AND setting_key = ?", pluginName, key).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetAll 플러그인의 전체 설정 조회
func (r *SettingRepository) GetAll(pluginName string) ([]domain.PluginSetting, error) {
	var list []domain.PluginSetting
	err := r.db.Where("plugin_name = ?", pluginName).Find(&list).Error
	return list, err
}

// Set 설정 저장 (upsert)
func (r *SettingRepository) Set(s *domain.PluginSetting) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "plugin_name"}, {Name: "setting_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"setting_value"}),
	}).Create(s).Error
}

// DeleteByPlugin 플러그인의 모든 설정 삭제
func (r *SettingRepository) DeleteByPlugin(pluginName string) error {
	return r.db.Where("plugin_name = ?", pluginName).Delete(&domain.PluginSetting{}).Error
}
