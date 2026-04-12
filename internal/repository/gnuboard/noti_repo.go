package gnuboard

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// joinOr joins SQL conditions with OR
func joinOr(conditions []string) string {
	return strings.Join(conditions, " OR ")
}

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
	Exists(mbID, boTable string, wrID int, fromCase, relMbID string) (bool, error)
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

// Exists checks if a duplicate unread notification already exists
func (r *notiRepository) Exists(mbID, boTable string, wrID int, fromCase, relMbID string) (bool, error) {
	var count int64
	err := r.db.Model(&Notification{}).
		Where("mb_id = ? AND bo_table = ? AND wr_id = ? AND ph_from_case = ? AND rel_mb_id = ? AND ph_readed = 'N'", mbID, boTable, wrID, fromCase, relMbID).
		Count(&count).Error
	return count > 0, err
}

// GetGroupedNotifications returns notifications grouped by (bo_table, wr_id, ph_from_case)
// Optimized: 2-pass approach — first identify top N groups (lightweight), then enrich only those groups
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
	case "memo":
		fromCaseFilter = "AND ph_from_case = 'memo'"
	case "system":
		fromCaseFilter = "AND ph_from_case IN ('write', 'inquire', 'answer')"
	}

	// Count total unread (fast — uses idx_mb_readed index)
	var unreadCount int64
	if err := r.db.Model(&Notification{}).
		Where("mb_id = ? AND ph_readed = 'N'", mbID).
		Count(&unreadCount).Error; err != nil {
		return nil, 0, 0, err
	}

	// Pass 1: Identify top N groups by MAX(ph_id) — lightweight, no GROUP_CONCAT
	offset := (page - 1) * limit
	topGroupsSQL := `SELECT
		bo_table, wr_id, ph_from_case,
		MAX(ph_id) as latest_ph_id,
		COUNT(*) as sender_count
	FROM g5_na_noti
	WHERE mb_id = ? ` + fromCaseFilter + `
	GROUP BY bo_table, wr_id, ph_from_case
	ORDER BY latest_ph_id DESC
	LIMIT ? OFFSET ?`

	type topGroup struct {
		BoTable     string `gorm:"column:bo_table"`
		WrID        int    `gorm:"column:wr_id"`
		PhFromCase  string `gorm:"column:ph_from_case"`
		LatestPhID  int    `gorm:"column:latest_ph_id"`
		SenderCount int    `gorm:"column:sender_count"`
	}
	var tops []topGroup
	if err := r.db.Raw(topGroupsSQL, mbID, limit, offset).Scan(&tops).Error; err != nil {
		return nil, 0, 0, err
	}

	if len(tops) == 0 {
		return []GroupedNotification{}, 0, unreadCount, nil
	}

	// Estimate total groups from first page (avoid expensive COUNT subquery)
	var totalGroups int64
	if page == 1 && len(tops) < limit {
		totalGroups = int64(len(tops))
	} else {
		countSQL := `SELECT COUNT(*) FROM (
			SELECT 1 FROM g5_na_noti
			WHERE mb_id = ? ` + fromCaseFilter + `
			GROUP BY bo_table, wr_id, ph_from_case
		) t`
		if err := r.db.Raw(countSQL, mbID).Scan(&totalGroups).Error; err != nil {
			return nil, 0, 0, err
		}
	}

	// Pass 2a: Get the latest notification row for each group (PK lookup)
	phIDs := make([]int, len(tops))
	for i, t := range tops {
		phIDs[i] = t.LatestPhID
	}

	var latestRows []Notification
	if err := r.db.Where("ph_id IN ?", phIDs).Find(&latestRows).Error; err != nil {
		return nil, 0, 0, err
	}
	latestMap := make(map[int]Notification, len(latestRows))
	for _, row := range latestRows {
		latestMap[row.PhID] = row
	}

	// Pass 2b: Get unread counts per group (only if there are unread notifications)
	type unreadResult struct {
		BoTable    string `gorm:"column:bo_table"`
		WrID       int    `gorm:"column:wr_id"`
		PhFromCase string `gorm:"column:ph_from_case"`
		Cnt        int    `gorm:"column:cnt"`
	}
	var unreadResults []unreadResult
	if unreadCount > 0 {
		conditions := make([]string, 0, len(tops))
		args := make([]interface{}, 0, len(tops)*3+1)
		args = append(args, mbID)
		for _, t := range tops {
			conditions = append(conditions, "(bo_table = ? AND wr_id = ? AND ph_from_case = ?)")
			args = append(args, t.BoTable, t.WrID, t.PhFromCase)
		}
		unreadSQL := `SELECT bo_table, wr_id, ph_from_case, COUNT(*) as cnt
			FROM g5_na_noti
			WHERE mb_id = ? AND ph_readed = 'N' AND (` + joinOr(conditions) + `)
			GROUP BY bo_table, wr_id, ph_from_case`
		r.db.Raw(unreadSQL, args...).Scan(&unreadResults)
	}
	unreadMap := make(map[string]int, len(unreadResults))
	for _, u := range unreadResults {
		key := u.BoTable + "|" + fmt.Sprintf("%d", u.WrID) + "|" + u.PhFromCase
		unreadMap[key] = u.Cnt
	}

	// Pass 2c: Get senders (skip GROUP_CONCAT for single-sender groups)
	senderMap := make(map[string]string, len(tops))
	var multiSenderTops []topGroup
	for _, t := range tops {
		key := t.BoTable + "|" + fmt.Sprintf("%d", t.WrID) + "|" + t.PhFromCase
		if t.SenderCount <= 1 {
			if latest, ok := latestMap[t.LatestPhID]; ok {
				senderMap[key] = latest.RelMbNick
			}
		} else {
			multiSenderTops = append(multiSenderTops, t)
		}
	}

	if len(multiSenderTops) > 0 {
		type senderResult struct {
			BoTable    string `gorm:"column:bo_table"`
			WrID       int    `gorm:"column:wr_id"`
			PhFromCase string `gorm:"column:ph_from_case"`
			Senders    string `gorm:"column:senders"`
		}
		senderConditions := make([]string, 0, len(multiSenderTops))
		senderArgs := make([]interface{}, 0, len(multiSenderTops)*3+1)
		senderArgs = append(senderArgs, mbID)
		for _, t := range multiSenderTops {
			senderConditions = append(senderConditions, "(bo_table = ? AND wr_id = ? AND ph_from_case = ?)")
			senderArgs = append(senderArgs, t.BoTable, t.WrID, t.PhFromCase)
		}
		var senderResults []senderResult
		sendersSQL := `SELECT bo_table, wr_id, ph_from_case,
			SUBSTRING_INDEX(GROUP_CONCAT(DISTINCT rel_mb_nick ORDER BY ph_datetime DESC SEPARATOR '||'), '||', 5) as senders
			FROM g5_na_noti
			WHERE mb_id = ? AND (` + joinOr(senderConditions) + `)
			GROUP BY bo_table, wr_id, ph_from_case`
		r.db.Raw(sendersSQL, senderArgs...).Scan(&senderResults)
		for _, s := range senderResults {
			key := s.BoTable + "|" + fmt.Sprintf("%d", s.WrID) + "|" + s.PhFromCase
			senderMap[key] = s.Senders
		}
	}

	// Assemble results in the same order as tops
	groups := make([]GroupedNotification, 0, len(tops))
	for _, t := range tops {
		latest, ok := latestMap[t.LatestPhID]
		if !ok {
			continue
		}
		key := t.BoTable + "|" + fmt.Sprintf("%d", t.WrID) + "|" + t.PhFromCase
		uc := unreadMap[key]
		senders := senderMap[key]

		groups = append(groups, GroupedNotification{
			BoTable:       t.BoTable,
			WrID:          t.WrID,
			PhFromCase:    t.PhFromCase,
			LatestPhID:    t.LatestPhID,
			LatestAt:      latest.PhDatetime,
			SenderCount:   t.SenderCount,
			UnreadCount:   uc,
			LatestSender:  latest.RelMbNick,
			Senders:       senders,
			RelURL:        latest.RelURL,
			ParentSubject: latest.ParentSubject,
			RelMsg:        latest.RelMsg,
		})
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
