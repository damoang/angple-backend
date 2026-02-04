package repository

import (
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// MessageRepository message data access interface
type MessageRepository interface {
	Create(msg *domain.Message) error
	FindByID(id int) (*domain.Message, error)
	FindInbox(mbID string, page, limit int) ([]*domain.Message, int64, error)
	FindSent(mbID string, page, limit int) ([]*domain.Message, int64, error)
	MarkAsRead(id int) error
	Delete(id int, mbID string) error
}

type messageRepository struct {
	db *gorm.DB
}

// NewMessageRepository creates a new MessageRepository
func NewMessageRepository(db *gorm.DB) MessageRepository {
	return &messageRepository{db: db}
}

// Create creates a new message (both recv and send records)
func (r *messageRepository) Create(msg *domain.Message) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 수신자 레코드
		recvMsg := &domain.Message{
			RecvMbID:     msg.RecvMbID,
			SendMbID:     msg.SendMbID,
			Memo:         msg.Memo,
			SendDatetime: msg.SendDatetime,
			SendIP:       msg.SendIP,
			Type:         "recv",
		}
		if err := tx.Create(recvMsg).Error; err != nil {
			return err
		}

		// 발신자 레코드
		sendMsg := &domain.Message{
			RecvMbID:     msg.RecvMbID,
			SendMbID:     msg.SendMbID,
			Memo:         msg.Memo,
			SendDatetime: msg.SendDatetime,
			SendIP:       msg.SendIP,
			Type:         "send",
		}
		if err := tx.Create(sendMsg).Error; err != nil {
			return err
		}

		// 수신자 mb_memo_cnt 증가
		return tx.Table("g5_member").Where("mb_id = ?", msg.RecvMbID).
			UpdateColumn("mb_memo_cnt", gorm.Expr("mb_memo_cnt + 1")).Error
	})
}

// FindByID finds a message by ID
func (r *messageRepository) FindByID(id int) (*domain.Message, error) {
	var msg domain.Message
	err := r.db.Where("me_id = ?", id).First(&msg).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// FindInbox returns received messages for a member
func (r *messageRepository) FindInbox(mbID string, page, limit int) ([]*domain.Message, int64, error) {
	var messages []*domain.Message
	var total int64

	r.db.Model(&domain.Message{}).
		Where("me_recv_mb_id = ? AND me_type = ?", mbID, "recv").
		Count(&total)

	offset := (page - 1) * limit
	err := r.db.Where("me_recv_mb_id = ? AND me_type = ?", mbID, "recv").
		Order("me_id DESC").
		Offset(offset).Limit(limit).
		Find(&messages).Error
	return messages, total, err
}

// FindSent returns sent messages for a member
func (r *messageRepository) FindSent(mbID string, page, limit int) ([]*domain.Message, int64, error) {
	var messages []*domain.Message
	var total int64

	r.db.Model(&domain.Message{}).
		Where("me_send_mb_id = ? AND me_type = ?", mbID, "send").
		Count(&total)

	offset := (page - 1) * limit
	err := r.db.Where("me_send_mb_id = ? AND me_type = ?", mbID, "send").
		Order("me_id DESC").
		Offset(offset).Limit(limit).
		Find(&messages).Error
	return messages, total, err
}

// MarkAsRead marks a message as read
func (r *messageRepository) MarkAsRead(id int) error {
	now := time.Now()
	return r.db.Model(&domain.Message{}).
		Where("me_id = ? AND me_read_datetime IS NULL", id).
		Update("me_read_datetime", now).Error
}

// Delete deletes a message
func (r *messageRepository) Delete(id int, mbID string) error {
	result := r.db.Where("me_id = ? AND (me_recv_mb_id = ? OR me_send_mb_id = ?)", id, mbID, mbID).
		Delete(&domain.Message{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
