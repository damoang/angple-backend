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

// GroupedNotification represents a group of notifications for the same post+type
type GroupedNotification struct {
	BoTable       string    `gorm:"column:bo_table"`
	WrID          int       `gorm:"column:wr_id"`
	PhFromCase    string    `gorm:"column:ph_from_case"`
	LatestPhID    int       `gorm:"column:latest_ph_id"`
	LatestAt      time.Time `gorm:"column:latest_at"`
	SenderCount   int       `gorm:"column:sender_count"`
	UnreadCount   int       `gorm:"column:unread_count"`
	LatestSender  string    `gorm:"column:latest_sender"`
	Senders       string    `gorm:"column:senders"`
	RelURL        string    `gorm:"column:rel_url"`
	ParentSubject string    `gorm:"column:parent_subject"`
	RelMsg        string    `gorm:"column:rel_msg"`
}

// NotiRepository provides access to g5_na_noti table
type NotiRepository interface {
	GetNotifications(mbID string, page, limit int) ([]Notification, int64, error)
	GetGroupedNotifications(mbID string, page, limit int, filterType string) ([]GroupedNotification, int64, int64, error)
	CountUnread(mbID string) (int64, error)
	MarkAsRead(mbID string, phID int) error
	MarkAllAsRead(mbID string) error
	MarkGroupAsRead(mbID, boTable string, wrID int, fromCase string) error
	Delete(mbID string, phID int) error
	DeleteGroup(mbID, boTable string, wrID int, fromCase string) error
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

// GetGroupedNotifications returns notifications grouped by (bo_table, wr_id, ph_from_case)
func (r *notiRepository) GetGroupedNotifications(mbID string, page, limit int, filterType string) ([]GroupedNotification, int64, int64, error) {
	// Build filter condition
	fromCaseFilter := ""
	switch filterType {
	case "comment":
		fromCaseFilter = "AND ph_from_case IN ('board', 'comment', 'reply')"
	case "like":
		fromCaseFilter = "AND ph_from_case = 'good'"
	case "mention":
		fromCaseFilter = "AND ph_from_case = 'mention'"
	case "system":
		fromCaseFilter = "AND ph_from_case IN ('write', 'inquire', 'answer')"
	}

	// Count total groups
	var totalGroups int64
	countSQL := `SELECT COUNT(*) FROM (
		SELECT 1 FROM g5_na_noti
		WHERE mb_id = ? ` + fromCaseFilter + `
		GROUP BY bo_table, wr_id, ph_from_case
	) t`
	if err := r.db.Raw(countSQL, mbID).Scan(&totalGroups).Error; err != nil {
		return nil, 0, 0, err
	}

	// Count total unread
	var unreadCount int64
	if err := r.db.Model(&Notification{}).
		Where("mb_id = ? AND ph_readed = 'N'", mbID).
		Count(&unreadCount).Error; err != nil {
		return nil, 0, 0, err
	}

	// Get grouped notifications
	offset := (page - 1) * limit
	groupSQL := `SELECT
		bo_table,
		wr_id,
		ph_from_case,
		MAX(ph_id) as latest_ph_id,
		MAX(ph_datetime) as latest_at,
		COUNT(*) as sender_count,
		SUM(CASE WHEN ph_readed = 'N' THEN 1 ELSE 0 END) as unread_count,
		SUBSTRING_INDEX(GROUP_CONCAT(rel_mb_nick ORDER BY ph_datetime DESC SEPARATOR '||'), '||', 1) as latest_sender,
		SUBSTRING_INDEX(GROUP_CONCAT(DISTINCT rel_mb_nick ORDER BY ph_datetime DESC SEPARATOR '||'), '||', 5) as senders,
		SUBSTRING_INDEX(GROUP_CONCAT(rel_url ORDER BY ph_datetime DESC SEPARATOR '||'), '||', 1) as rel_url,
		SUBSTRING_INDEX(GROUP_CONCAT(parent_subject ORDER BY ph_datetime DESC SEPARATOR '||'), '||', 1) as parent_subject,
		SUBSTRING_INDEX(GROUP_CONCAT(rel_msg ORDER BY ph_datetime DESC SEPARATOR '||'), '||', 1) as rel_msg
	FROM g5_na_noti
	WHERE mb_id = ? ` + fromCaseFilter + `
	GROUP BY bo_table, wr_id, ph_from_case
	ORDER BY latest_ph_id DESC
	LIMIT ? OFFSET ?`

	var groups []GroupedNotification
	if err := r.db.Raw(groupSQL, mbID, limit, offset).Scan(&groups).Error; err != nil {
		return nil, 0, 0, err
	}

	return groups, totalGroups, unreadCount, nil
}

// MarkGroupAsRead marks all notifications in a group as read
func (r *notiRepository) MarkGroupAsRead(mbID, boTable string, wrID int, fromCase string) error {
	return r.db.Model(&Notification{}).
		Where("mb_id = ? AND bo_table = ? AND wr_id = ? AND ph_from_case = ? AND ph_readed = 'N'", mbID, boTable, wrID, fromCase).
		Update("ph_readed", "Y").Error
}

// DeleteGroup deletes all notifications in a group
func (r *notiRepository) DeleteGroup(mbID, boTable string, wrID int, fromCase string) error {
	return r.db.Where("mb_id = ? AND bo_table = ? AND wr_id = ? AND ph_from_case = ?", mbID, boTable, wrID, fromCase).
		Delete(&Notification{}).Error
}
