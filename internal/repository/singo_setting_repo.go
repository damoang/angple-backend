package repository

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// SingoSettingRepository — singo_settings 테이블 접근
type SingoSettingRepository struct {
	db *gorm.DB
}

func NewSingoSettingRepository(db *gorm.DB) *SingoSettingRepository {
	return &SingoSettingRepository{db: db}
}

// GetAll — 모든 설정 조회
func (r *SingoSettingRepository) GetAll() ([]domain.SingoSetting, error) {
	var settings []domain.SingoSetting
	err := r.db.Find(&settings).Error
	return settings, err
}

// Get — 특정 키의 값 조회
func (r *SingoSettingRepository) Get(key string) (string, error) {
	var setting domain.SingoSetting
	err := r.db.Where("`key` = ?", key).First(&setting).Error
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

// Set — 설정 값 저장 (upsert)
func (r *SingoSettingRepository) Set(key, value, updatedBy string) error {
	setting := domain.SingoSetting{
		Key:       key,
		Value:     value,
		UpdatedBy: updatedBy,
	}
	return r.db.Save(&setting).Error
}
