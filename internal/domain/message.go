package domain

import "time"

// Message represents a private message (g5_memo table)
type Message struct {
	SendDatetime time.Time  `gorm:"column:me_send_datetime" json:"send_datetime"`
	ReadDatetime *time.Time `gorm:"column:me_read_datetime" json:"read_datetime,omitempty"`
	RecvMbID     string     `gorm:"column:me_recv_mb_id;index" json:"recv_mb_id"`
	SendMbID     string     `gorm:"column:me_send_mb_id;index" json:"send_mb_id"`
	Memo         string     `gorm:"column:me_memo;type:text" json:"memo"`
	SendIP       string     `gorm:"column:me_send_ip" json:"-"`
	Type         string     `gorm:"column:me_type" json:"type"` // "recv" or "send"
	ID           int        `gorm:"column:me_id;primaryKey;autoIncrement" json:"id"`
}

func (Message) TableName() string {
	return "g5_memo"
}

// SendMessageRequest represents a send message request
type SendMessageRequest struct {
	ToUserID string `json:"to_user_id" binding:"required"`
	Content  string `json:"content" binding:"required"`
}

// MessageResponse represents a message in API responses
type MessageResponse struct {
	SendDatetime string `json:"send_datetime"`
	ReadDatetime string `json:"read_datetime,omitempty"`
	FromUserID   string `json:"from_user_id"`
	ToUserID     string `json:"to_user_id"`
	Content      string `json:"content"`
	ID           int    `json:"id"`
	IsRead       bool   `json:"is_read"`
}

// ToResponse converts Message to MessageResponse
func (m *Message) ToResponse() *MessageResponse {
	resp := &MessageResponse{
		ID:           m.ID,
		FromUserID:   m.SendMbID,
		ToUserID:     m.RecvMbID,
		Content:      m.Memo,
		SendDatetime: m.SendDatetime.Format("2006-01-02 15:04:05"),
		IsRead:       m.ReadDatetime != nil,
	}
	if m.ReadDatetime != nil {
		resp.ReadDatetime = m.ReadDatetime.Format("2006-01-02 15:04:05")
	}
	return resp
}
