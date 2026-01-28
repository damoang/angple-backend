package repository

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// G5MemoRepository handles g5_memo (쪽지) data operations
type G5MemoRepository struct {
	db *gorm.DB
}

// NewG5MemoRepository creates a new G5MemoRepository
func NewG5MemoRepository(db *gorm.DB) *G5MemoRepository {
	return &G5MemoRepository{db: db}
}

// WithTx returns a new G5MemoRepository with the given transaction
func (r *G5MemoRepository) WithTx(tx *gorm.DB) *G5MemoRepository {
	return &G5MemoRepository{db: tx}
}

// DB returns the underlying database instance
func (r *G5MemoRepository) DB() *gorm.DB {
	return r.db
}

// SendMemo sends a message (쪽지) to a member
func (r *G5MemoRepository) SendMemo(recvMemberID, sendMemberID, memo, clientIP string) error {
	now := time.Now()

	// Create receiver's memo (me_type = 'recv')
	recvMemo := &domain.G5Memo{
		RecvMemberID: recvMemberID,
		SendMemberID: sendMemberID,
		SendDatetime: now,
		ReadDatetime: "",
		Memo:         memo,
		SendID:       0,
		Type:         "recv",
		SendIP:       clientIP,
	}

	if err := r.db.Create(recvMemo).Error; err != nil {
		return fmt.Errorf("수신 쪽지 생성 실패: %w", err)
	}

	// Create sender's memo (me_type = 'send') with reference to receiver's memo
	sendMemo := &domain.G5Memo{
		RecvMemberID: recvMemberID,
		SendMemberID: sendMemberID,
		SendDatetime: now,
		ReadDatetime: "",
		Memo:         memo,
		SendID:       recvMemo.ID,
		Type:         "send",
		SendIP:       clientIP,
	}

	if err := r.db.Create(sendMemo).Error; err != nil {
		return fmt.Errorf("발신 쪽지 생성 실패: %w", err)
	}

	// Update member's me_recv_cnt (쪽지 알림 카운트)
	if err := r.db.Exec("UPDATE g5_member SET mb_memo_cnt = mb_memo_cnt + 1, mb_memo_call_mb_id = ? WHERE mb_id = ?", sendMemberID, recvMemberID).Error; err != nil {
		return fmt.Errorf("회원 쪽지 카운트 업데이트 실패: %w", err)
	}

	return nil
}

// GetUnreadCount returns the count of unread memos for a member
func (r *G5MemoRepository) GetUnreadCount(memberID string) (int64, error) {
	var count int64
	err := r.db.Model(&domain.G5Memo{}).
		Where("me_recv_mb_id = ? AND me_type = 'recv' AND me_read_datetime = ''", memberID).
		Count(&count).Error
	return count, err
}

// GetMemoList retrieves memos for a member
func (r *G5MemoRepository) GetMemoList(memberID string, memoType string, offset, limit int) ([]domain.G5Memo, int64, error) {
	var memos []domain.G5Memo
	var total int64

	query := r.db.Model(&domain.G5Memo{})

	if memoType == "recv" {
		query = query.Where("me_recv_mb_id = ? AND me_type = 'recv'", memberID)
	} else if memoType == "send" {
		query = query.Where("me_send_mb_id = ? AND me_type = 'send'", memberID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("me_id DESC").
		Offset(offset).
		Limit(limit).
		Find(&memos).Error; err != nil {
		return nil, 0, err
	}

	return memos, total, nil
}
