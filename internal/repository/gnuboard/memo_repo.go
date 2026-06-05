package gnuboard

import (
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

// MemoRepository provides access to g5_memo table
type MemoRepository interface {
	FindInbox(mbID string, page, limit int) ([]*gnuboard.G5Memo, int64, error)
	FindSent(mbID string, page, limit int) ([]*gnuboard.G5Memo, int64, error)
	FindByID(meID int, mbID string) (*gnuboard.G5Memo, error)
	MarkAsRead(meID int) error
	Delete(meID int, mbID string) error
	Send(senderID, receiverID, content string) (*gnuboard.G5Memo, error)
	CountUnread(mbID string) (int64, error)
}

type memoRepository struct {
	db *gorm.DB
}

// NewMemoRepository creates a new MemoRepository for g5_memo
func NewMemoRepository(db *gorm.DB) MemoRepository {
	return &memoRepository{db: db}
}

// FindInbox returns received memos for the user
func (r *memoRepository) FindInbox(mbID string, page, limit int) ([]*gnuboard.G5Memo, int64, error) {
	var memos []*gnuboard.G5Memo
	var total int64

	query := r.db.Model(&gnuboard.G5Memo{}).Where("me_recv_mb_id = ? AND me_type = 'recv'", mbID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("me_id DESC").Offset(offset).Limit(limit).Find(&memos).Error; err != nil {
		return nil, 0, err
	}

	return memos, total, nil
}

// FindSent returns sent memos for the user
func (r *memoRepository) FindSent(mbID string, page, limit int) ([]*gnuboard.G5Memo, int64, error) {
	var memos []*gnuboard.G5Memo
	var total int64

	query := r.db.Model(&gnuboard.G5Memo{}).Where("me_send_mb_id = ? AND me_type = 'send'", mbID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("me_id DESC").Offset(offset).Limit(limit).Find(&memos).Error; err != nil {
		return nil, 0, err
	}

	return memos, total, nil
}

// FindByID finds a memo by ID, verifying it belongs to the user
func (r *memoRepository) FindByID(meID int, mbID string) (*gnuboard.G5Memo, error) {
	var memo gnuboard.G5Memo
	err := r.db.Where("me_id = ? AND (me_send_mb_id = ? OR me_recv_mb_id = ?)", meID, mbID, mbID).
		First(&memo).Error
	if err != nil {
		return nil, err
	}
	return &memo, nil
}

// MarkAsRead updates the read datetime for a memo
func (r *memoRepository) MarkAsRead(meID int) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	return r.db.Model(&gnuboard.G5Memo{}).Where("me_id = ?", meID).
		Update("me_read_datetime", now).Error
}

// Delete deletes a memo for the user
func (r *memoRepository) Delete(meID int, mbID string) error {
	return r.db.Where("me_id = ? AND (me_send_mb_id = ? OR me_recv_mb_id = ?)", meID, mbID, mbID).
		Delete(&gnuboard.G5Memo{}).Error
}

// Send creates a pair of memos (recv for receiver, send for sender) — Gnuboard convention
func (r *memoRepository) Send(senderID, receiverID, content string) (*gnuboard.G5Memo, error) {
	now := time.Now()

	// Create recv memo (for receiver)
	recvMemo := gnuboard.G5Memo{
		MeSendMbID:     senderID,
		MeRecvMbID:     receiverID,
		MeMemo:         content,
		MeReadDatetime: "",
		MeSendDatetime: now,
		MeType:         "recv",
	}
	if err := r.db.Create(&recvMemo).Error; err != nil {
		return nil, err
	}

	// Create send memo (for sender) with paired ID
	sendMemo := gnuboard.G5Memo{
		MeSendMbID:     senderID,
		MeRecvMbID:     receiverID,
		MeMemo:         content,
		MeReadDatetime: now.Format("2006-01-02 15:04:05"), // sender has already "read" it
		MeSendDatetime: now,
		MeType:         "send",
		MeSendID:       recvMemo.MeID,
	}
	if err := r.db.Create(&sendMemo).Error; err != nil {
		return nil, err
	}

	// Update recv memo with paired ID
	r.db.Model(&gnuboard.G5Memo{}).Where("me_id = ?", recvMemo.MeID).Update("me_send_id", sendMemo.MeID)

	return &recvMemo, nil
}

// CountUnread returns the count of unread memos for the user.
// me_read_datetime은 NOT NULL datetime 컬럼이라 미열람 = zero date.
// MySQL 8은 datetime 컬럼과 ''의 비교에서 'Incorrect DATETIME value' 에러를 던지므로 '' 비교 금지.
func (r *memoRepository) CountUnread(mbID string) (int64, error) {
	var count int64
	err := r.db.Model(&gnuboard.G5Memo{}).
		Where("me_recv_mb_id = ? AND me_type = 'recv' AND me_read_datetime = '0000-00-00 00:00:00'", mbID).
		Count(&count).Error
	return count, err
}
