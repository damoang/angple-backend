package gnuboard

import (
	"time"

	"gorm.io/gorm"
)

// Notification represents a row in g5_na_noti table
type Notification struct {
	PhID          int       `gorm:"column:ph_id;primaryKey"`
	PhToCase      string    `gorm:"column:ph_to_case"`
	PhFromCase    string    `gorm:"column:ph_from_case"`
	BoTable       string    `gorm:"column:bo_table"`
	WrID          int       `gorm:"column:wr_id"`
	MbID          string    `gorm:"column:mb_id"`
	RelMbID       string    `gorm:"column:rel_mb_id"`
	RelMbNick     string    `gorm:"column:rel_mb_nick"`
	RelMsg        string    `gorm:"column:rel_msg"`
	RelURL        string    `gorm:"column:rel_url"`
	PhReaded      string    `gorm:"column:ph_readed"`
	PhDatetime    time.Time `gorm:"column:ph_datetime"`
	ParentSubject string    `gorm:"column:parent_subject"`
	WrParent      int       `gorm:"column:wr_parent"`
}

// TableName returns the g5_na_noti table name
func (Notification) TableName() string { return "g5_na_noti" }

// NotiRepository provides access to g5_na_noti table
type NotiRepository interface {
	GetNotifications(mbID string, page, limit int) ([]Notification, int64, error)
	CountUnread(mbID string) (int64, error)
	MarkAsRead(mbID string, phID int) error
	MarkAllAsRead(mbID string) error
	Delete(mbID string, phID int) error
	Create(noti *Notification) error
}

type notiRepository struct {
	db *gorm.DB
}

// NewNotiRepository creates a new NotiRepository for g5_na_noti
func NewNotiRepository(db *gorm.DB) NotiRepository {
	return &notiRepository{db: db}
}

// GetNotifications returns notifications for the user with pagination
func (r *notiRepository) GetNotifications(mbID string, page, limit int) ([]Notification, int64, error) {
	var notifications []Notification
	var total int64

	query := r.db.Model(&Notification{}).Where("mb_id = ?", mbID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("ph_id DESC").Offset(offset).Limit(limit).Find(&notifications).Error; err != nil {
		return nil, 0, err
	}

	return notifications, total, nil
}

// CountUnread returns the count of unread notifications for the user
func (r *notiRepository) CountUnread(mbID string) (int64, error) {
	var count int64
	err := r.db.Model(&Notification{}).
		Where("mb_id = ? AND ph_readed = 'N'", mbID).
		Count(&count).Error
	return count, err
}

// MarkAsRead marks a single notification as read
func (r *notiRepository) MarkAsRead(mbID string, phID int) error {
	return r.db.Model(&Notification{}).
		Where("ph_id = ? AND mb_id = ?", phID, mbID).
		Update("ph_readed", "Y").Error
}

// MarkAllAsRead marks all unread notifications as read for the user
func (r *notiRepository) MarkAllAsRead(mbID string) error {
	return r.db.Model(&Notification{}).
		Where("mb_id = ? AND ph_readed = 'N'", mbID).
		Update("ph_readed", "Y").Error
}

// Delete deletes a notification for the user
func (r *notiRepository) Delete(mbID string, phID int) error {
	return r.db.Where("ph_id = ? AND mb_id = ?", phID, mbID).
		Delete(&Notification{}).Error
}

// Create inserts a new notification
func (r *notiRepository) Create(noti *Notification) error {
	return r.db.Create(noti).Error
}
