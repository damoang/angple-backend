package repository

import (
	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"gorm.io/gorm"
)

// InstallationRepository 플러그인 설치 상태 저장소
type InstallationRepository struct {
	db *gorm.DB
}

// NewInstallationRepository 생성자
func NewInstallationRepository(db *gorm.DB) *InstallationRepository {
	return &InstallationRepository{db: db}
}

// FindByName 플러그인 이름으로 조회
func (r *InstallationRepository) FindByName(name string) (*domain.PluginInstallation, error) {
	var inst domain.PluginInstallation
	err := r.db.Where("plugin_name = ?", name).First(&inst).Error
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

// FindAll 전체 조회
func (r *InstallationRepository) FindAll() ([]domain.PluginInstallation, error) {
	var list []domain.PluginInstallation
	err := r.db.Find(&list).Error
	return list, err
}

// FindEnabled 활성화된 플러그인 조회
func (r *InstallationRepository) FindEnabled() ([]domain.PluginInstallation, error) {
	var list []domain.PluginInstallation
	err := r.db.Where("status = ?", domain.StatusEnabled).Find(&list).Error
	return list, err
}

// Create 새 설치 레코드 생성
func (r *InstallationRepository) Create(inst *domain.PluginInstallation) error {
	return r.db.Create(inst).Error
}

// Update 설치 레코드 갱신
func (r *InstallationRepository) Update(inst *domain.PluginInstallation) error {
	return r.db.Save(inst).Error
}

// Delete 설치 레코드 삭제
func (r *InstallationRepository) Delete(name string) error {
	return r.db.Where("plugin_name = ?", name).Delete(&domain.PluginInstallation{}).Error
}
