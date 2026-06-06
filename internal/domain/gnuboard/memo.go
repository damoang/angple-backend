package gnuboard

import (
	"strings"
	"time"
)

// G5Memo represents the g5_memo table (Gnuboard messages/쪽지)
type G5Memo struct {
	MeID           int       `gorm:"column:me_id;primaryKey;autoIncrement" json:"me_id"`
	MeSendMbID     string    `gorm:"column:me_send_mb_id" json:"me_send_mb_id"`
	MeRecvMbID     string    `gorm:"column:me_recv_mb_id" json:"me_recv_mb_id"`
	MeMemo         string    `gorm:"column:me_memo" json:"me_memo"`
	MeReadDatetime string    `gorm:"column:me_read_datetime" json:"me_read_datetime"`
	MeSendDatetime time.Time `gorm:"column:me_send_datetime" json:"me_send_datetime"`
	MeType         string    `gorm:"column:me_type" json:"me_type"`       // 'recv' or 'send'
	MeSendID       int       `gorm:"column:me_send_id" json:"me_send_id"` // paired message ID
	MeSendIP       string    `gorm:"column:me_send_ip" json:"me_send_ip"`
}

// TableName returns the table name for GORM
func (G5Memo) TableName() string {
	return "g5_memo"
}

// IsRead returns whether the memo has been read.
// me_read_datetime은 NOT NULL datetime 컬럼으로 미열람 시 zero date('0000-00-00 00:00:00')가
// 저장된다. DSN이 parseTime=True라 zero date가 Go zero time 문자열("0001-01-01 ...")로
// 스캔되므로, 두 형태 모두 미열람으로 판정해야 한다. (기존 코드는 raw 문자열만 비교해
// 모든 쪽지를 읽음으로 오판 → 미열람 표시·읽음 처리가 전부 동작하지 않았음)
func (m *G5Memo) IsRead() bool {
	if m.MeReadDatetime == "" {
		return false
	}
	if strings.HasPrefix(m.MeReadDatetime, "0000-00-00") || strings.HasPrefix(m.MeReadDatetime, "0001-01-01") {
		return false
	}
	return true
}
