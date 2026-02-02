package v2

import (
	"time"

	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// MessageRepository v2 message data access
type MessageRepository interface {
	Create(msg *v2.V2Message) error
	FindByID(id uint64) (*v2.V2Message, error)
	FindInbox(userID uint64, page, limit int) ([]*v2.V2Message, int64, error)
	FindSent(userID uint64, page, limit int) ([]*v2.V2Message, int64, error)
	MarkAsRead(id uint64) error
	DeleteForUser(id, userID uint64) error
}

type messageRepository struct {
	db *gorm.DB
}

// NewMessageRepository creates a new v2 MessageRepository
func NewMessageRepository(db *gorm.DB) MessageRepository {
	return &messageRepository{db: db}
}

func (r *messageRepository) Create(msg *v2.V2Message) error {
	return r.db.Create(msg).Error
}

func (r *messageRepository) FindByID(id uint64) (*v2.V2Message, error) {
	var msg v2.V2Message
	err := r.db.Where("id = ?", id).First(&msg).Error
	return &msg, err
}

func (r *messageRepository) FindInbox(userID uint64, page, limit int) ([]*v2.V2Message, int64, error) {
	var messages []*v2.V2Message
	var total int64

	query := r.db.Model(&v2.V2Message{}).Where("receiver_id = ? AND deleted_by_receiver = false", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&messages).Error; err != nil {
		return nil, 0, err
	}
	return messages, total, nil
}

func (r *messageRepository) FindSent(userID uint64, page, limit int) ([]*v2.V2Message, int64, error) {
	var messages []*v2.V2Message
	var total int64

	query := r.db.Model(&v2.V2Message{}).Where("sender_id = ? AND deleted_by_sender = false", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&messages).Error; err != nil {
		return nil, 0, err
	}
	return messages, total, nil
}

func (r *messageRepository) MarkAsRead(id uint64) error {
	now := time.Now()
	return r.db.Model(&v2.V2Message{}).Where("id = ?", id).
		Updates(map[string]interface{}{"is_read": true, "read_at": now}).Error
}

func (r *messageRepository) DeleteForUser(id, userID uint64) error {
	msg, err := r.FindByID(id)
	if err != nil {
		return err
	}
	if msg.SenderID == userID {
		return r.db.Model(&v2.V2Message{}).Where("id = ?", id).Update("deleted_by_sender", true).Error
	}
	if msg.ReceiverID == userID {
		return r.db.Model(&v2.V2Message{}).Where("id = ?", id).Update("deleted_by_receiver", true).Error
	}
	return gorm.ErrRecordNotFound
}
