package v2

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// BlockRepository v2 block data access interface
type BlockRepository interface {
	Create(mbID string, blockedMbID string, scope string) (*domain.MemberBlock, error)
	UpdateScope(mbID string, blockedMbID string, scope string) error
	Delete(mbID string, blockedMbID string) error
	FindByMember(mbID string, page, perPage int) ([]*BlockWithMember, int64, error)
	Exists(mbID string, blockedMbID string) (bool, error)
	// GetBlockedUserIDs 는 스코프 무관 전체 차단 대상을 반환한다(차단 목록 표시용).
	GetBlockedUserIDs(mbID string) ([]string, error)
	// GetContentBlockedUserIDs 는 글/댓글 숨김·알림 억제용으로, "쪽지만 차단"(message)은 제외하고
	// "all"·"content" 스코프만 반환한다(#12916).
	GetContentBlockedUserIDs(mbID string) ([]string, error)
	// IsMessageBlocked 는 쪽지 전송 차단 여부로, "all"·"message" 스코프만 참으로 본다(#12916).
	IsMessageBlocked(mbID string, blockedMbID string) (bool, error)
}

// BlockWithMember includes blocked member info
type BlockWithMember struct {
	BlockID   int       `json:"block_id"`
	UserID    string    `json:"user_id"`
	Nickname  string    `json:"nickname"`
	Scope     string    `json:"scope"`
	BlockedAt time.Time `json:"blocked_at"`
}

type blockRepository struct {
	db *gorm.DB
}

// NewBlockRepository creates a new v2 BlockRepository
func NewBlockRepository(db *gorm.DB) BlockRepository {
	return &blockRepository{db: db}
}

// Create adds a block with the given scope. Empty scope defaults to "all".
func (r *blockRepository) Create(mbID string, blockedMbID string, scope string) (*domain.MemberBlock, error) {
	if scope == "" {
		scope = domain.BlockScopeAll
	}
	block := &domain.MemberBlock{
		MbID:        mbID,
		BlockedMbID: blockedMbID,
		Scope:       scope,
		CreatedAt:   time.Now(),
	}
	if err := r.db.Create(block).Error; err != nil {
		return nil, err
	}
	return block, nil
}

// UpdateScope changes the scope of an existing block (재차단 없이 범위 변경 지원).
func (r *blockRepository) UpdateScope(mbID string, blockedMbID string, scope string) error {
	if scope == "" {
		scope = domain.BlockScopeAll
	}
	result := r.db.Model(&domain.MemberBlock{}).
		Where("mb_id = ? AND blocked_mb_id = ?", mbID, blockedMbID).
		Update("block_scope", scope)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("차단 기록을 찾을 수 없습니다")
	}
	return nil
}

// Delete removes a block
func (r *blockRepository) Delete(mbID string, blockedMbID string) error {
	result := r.db.Where("mb_id = ? AND blocked_mb_id = ?", mbID, blockedMbID).
		Delete(&domain.MemberBlock{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("차단 기록을 찾을 수 없습니다")
	}
	return nil
}

// FindByMember returns blocks with member info and pagination
func (r *blockRepository) FindByMember(mbID string, page, perPage int) ([]*BlockWithMember, int64, error) {
	var total int64
	r.db.Model(&domain.MemberBlock{}).Where("mb_id = ?", mbID).Count(&total)

	offset := (page - 1) * perPage
	var blocks []*domain.MemberBlock
	err := r.db.Where("mb_id = ?", mbID).
		Order("id DESC").
		Offset(offset).
		Limit(perPage).
		Find(&blocks).Error
	if err != nil {
		return nil, 0, err
	}

	// Batch fetch nicknames for all blocked members
	mbIDs := make([]string, len(blocks))
	for i, b := range blocks {
		mbIDs[i] = b.BlockedMbID
	}
	nickMap := make(map[string]string)
	if len(mbIDs) > 0 {
		var members []struct {
			MbID     string `gorm:"column:mb_id"`
			Nickname string `gorm:"column:mb_nick"`
		}
		r.db.Table("g5_member").Select("mb_id, mb_nick").Where("mb_id IN ?", mbIDs).Find(&members)
		for _, m := range members {
			nickMap[m.MbID] = m.Nickname
		}
	}

	results := make([]*BlockWithMember, len(blocks))
	for i, b := range blocks {
		scope := b.Scope
		if scope == "" {
			scope = domain.BlockScopeAll
		}
		results[i] = &BlockWithMember{
			BlockID:   b.ID,
			UserID:    b.BlockedMbID,
			Nickname:  nickMap[b.BlockedMbID],
			Scope:     scope,
			BlockedAt: b.CreatedAt,
		}
	}

	return results, total, nil
}

// Exists checks if a block exists
func (r *blockRepository) Exists(mbID string, blockedMbID string) (bool, error) {
	var count int64
	err := r.db.Model(&domain.MemberBlock{}).
		Where("mb_id = ? AND blocked_mb_id = ?", mbID, blockedMbID).
		Count(&count).Error
	return count > 0, err
}

// GetBlockedUserIDs returns all blocked user IDs for a member (스코프 무관, 표시용).
func (r *blockRepository) GetBlockedUserIDs(mbID string) ([]string, error) {
	var ids []string
	err := r.db.Model(&domain.MemberBlock{}).
		Where("mb_id = ?", mbID).
		Pluck("blocked_mb_id", &ids).Error
	return ids, err
}

// GetContentBlockedUserIDs returns user IDs blocked for content (글/댓글) 숨김·알림 억제.
// "쪽지만 차단"(message)은 제외한다. 빈 스코프(구버전 데이터)는 all 로 간주해 포함. (#12916)
func (r *blockRepository) GetContentBlockedUserIDs(mbID string) ([]string, error) {
	var ids []string
	err := r.db.Model(&domain.MemberBlock{}).
		Where("mb_id = ? AND (block_scope IN ? OR block_scope IS NULL OR block_scope = '')",
			mbID, []string{domain.BlockScopeAll, domain.BlockScopeContent}).
		Pluck("blocked_mb_id", &ids).Error
	return ids, err
}

// IsMessageBlocked reports whether receiver has blocked sender for messages.
// "all"·"message" 스코프만 참. 빈 스코프(구버전)는 all 로 간주. (#12916)
func (r *blockRepository) IsMessageBlocked(mbID string, blockedMbID string) (bool, error) {
	var count int64
	err := r.db.Model(&domain.MemberBlock{}).
		Where("mb_id = ? AND blocked_mb_id = ? AND (block_scope IN ? OR block_scope IS NULL OR block_scope = '')",
			mbID, blockedMbID, []string{domain.BlockScopeAll, domain.BlockScopeMessage}).
		Count(&count).Error
	return count > 0, err
}
