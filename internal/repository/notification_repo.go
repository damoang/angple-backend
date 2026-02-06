package repository

import (
	"errors"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// NotificationRepository handles notification data operations
type NotificationRepository struct {
	db *gorm.DB
}

// NewNotificationRepository creates a new NotificationRepository
func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// GetUnreadCount returns the number of unread notifications for a member
func (r *NotificationRepository) GetUnreadCount(memberID string) (int64, error) {
	var count int64
	err := r.db.Model(&domain.Notification{}).
		Where("mb_id = ? AND ph_readed = 'N'", memberID).
		Count(&count).Error
	return count, err
}

// GetList returns paginated notifications for a member
func (r *NotificationRepository) GetList(memberID string, offset, limit int) ([]domain.Notification, int64, error) {
	var notifications []domain.Notification
	var total int64

	if err := r.db.Model(&domain.Notification{}).
		Where("mb_id = ?", memberID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.Where("mb_id = ?", memberID).
		Order("ph_datetime DESC").
		Offset(offset).
		Limit(limit).
		Find(&notifications).Error; err != nil {
		return nil, 0, err
	}

	return notifications, total, nil
}

// FindByID returns a notification by ID
func (r *NotificationRepository) FindByID(id int) (*domain.Notification, error) {
	var notification domain.Notification
	err := r.db.Where("ph_id = ?", id).First(&notification).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &notification, nil
}

// MarkAsRead marks a notification as read
func (r *NotificationRepository) MarkAsRead(id int) error {
	return r.db.Model(&domain.Notification{}).
		Where("ph_id = ?", id).
		Update("ph_readed", "Y").Error
}

// MarkAllAsRead marks all notifications as read for a member
func (r *NotificationRepository) MarkAllAsRead(memberID string) error {
	return r.db.Model(&domain.Notification{}).
		Where("mb_id = ? AND ph_readed = 'N'", memberID).
		Update("ph_readed", "Y").Error
}

// Create inserts a new notification
func (r *NotificationRepository) Create(notification *domain.Notification) error {
	return r.db.Create(notification).Error
}

// Delete deletes a notification by ID
func (r *NotificationRepository) Delete(id int) error {
	return r.db.Where("ph_id = ?", id).Delete(&domain.Notification{}).Error
}
