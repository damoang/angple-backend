package v2

import "time"

// V2Message represents a private message between users
type V2Message struct {
	ID         uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	SenderID   uint64     `gorm:"column:sender_id;index" json:"sender_id"`
	ReceiverID uint64     `gorm:"column:receiver_id;index" json:"receiver_id"`
	Content    string     `gorm:"column:content;type:text" json:"content"`
	IsRead     bool       `gorm:"column:is_read;default:false" json:"is_read"`
	ReadAt     *time.Time `gorm:"column:read_at" json:"read_at,omitempty"`
	DeletedBySender   bool `gorm:"column:deleted_by_sender;default:false" json:"-"`
	DeletedByReceiver bool `gorm:"column:deleted_by_receiver;default:false" json:"-"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (V2Message) TableName() string { return "v2_messages" }
