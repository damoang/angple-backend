package v2

import (
	"errors"

	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// DeviceRepository v2 device (FCM token) data access
type DeviceRepository interface {
	Upsert(device *v2.V2Device) error
	DeleteByUserAndToken(userID uint64, token string) error
	// DeleteByToken 은 토큰 단독으로 삭제한다 — 세션 만료로 JWT 가 없는 기기의
	// 등록 해제용(#13444). 토큰은 비추측성 시크릿이라 보유 자체가 소유 증명이다.
	DeleteByToken(token string) error
	ListByUser(userID uint64) ([]v2.V2Device, error)
}

type deviceRepository struct {
	db *gorm.DB
}

// NewDeviceRepository creates a new v2 DeviceRepository
func NewDeviceRepository(db *gorm.DB) DeviceRepository {
	return &deviceRepository{db: db}
}

// Upsert inserts a new device or updates the existing row that owns the token.
// Tokens are globally unique, so a token re-used across users is reassigned to
// the new user (covers logout-on-A then login-on-B on the same device).
func (r *deviceRepository) Upsert(device *v2.V2Device) error {
	var existing v2.V2Device
	result := r.db.Where("token = ?", device.Token).First(&existing)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return r.db.Create(device).Error
	}
	if result.Error != nil {
		return result.Error
	}
	existing.UserID = device.UserID
	existing.Platform = device.Platform
	existing.AppVersion = device.AppVersion
	return r.db.Save(&existing).Error
}

func (r *deviceRepository) DeleteByUserAndToken(userID uint64, token string) error {
	return r.db.Where("user_id = ? AND token = ?", userID, token).Delete(&v2.V2Device{}).Error
}

func (r *deviceRepository) ListByUser(userID uint64) ([]v2.V2Device, error) {
	var devices []v2.V2Device
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&devices).Error
	return devices, err
}

func (r *deviceRepository) DeleteByToken(token string) error {
	return r.db.Where("token = ?", token).Delete(&v2.V2Device{}).Error
}
