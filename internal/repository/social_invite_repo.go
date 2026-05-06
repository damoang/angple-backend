package repository

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	gnuboard "github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

type SocialInviteRepository struct {
	db *gorm.DB
}

func NewSocialInviteRepository(db *gorm.DB) *SocialInviteRepository {
	return &SocialInviteRepository{db: db}
}

func (r *SocialInviteRepository) Create(invite *domain.SocialInvite) error {
	return r.db.Create(invite).Error
}

func (r *SocialInviteRepository) FindByToken(token string) (*domain.SocialInvite, error) {
	var invite domain.SocialInvite
	if err := r.db.Where("token = ?", token).First(&invite).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("초대 토큰 조회 실패: %w", err)
	}
	return &invite, nil
}

func (r *SocialInviteRepository) MarkUsed(token string, usedBy string) error {
	now := time.Now()
	result := r.db.Model(&domain.SocialInvite{}).
		Where("token = ? AND used_at IS NULL", token).
		Updates(map[string]interface{}{
			"used_at": now,
			"used_by": usedBy,
		})
	if result.Error != nil {
		return fmt.Errorf("초대 사용 처리 실패: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("초대를 찾을 수 없거나 이미 사용되었습니다")
	}
	return nil
}

func (r *SocialInviteRepository) FindMemberByID(mbID string) (*gnuboard.G5Member, error) {
	var member gnuboard.G5Member
	if err := r.db.Where("mb_id = ?", mbID).First(&member).Error; err != nil {
		return nil, fmt.Errorf("회원을 찾을 수 없습니다: %w", err)
	}
	return &member, nil
}

func (r *SocialInviteRepository) FindSocialProfiles(mbID string) ([]domain.SocialProfile, error) {
	var profiles []domain.SocialProfile
	if err := r.db.Where("mb_id = ?", mbID).Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

func (r *SocialInviteRepository) UpdateSocialProfileOwner(mpNo int, newMbID string) error {
	result := r.db.Model(&domain.SocialProfile{}).Where("mp_no = ?", mpNo).Update("mb_id", newMbID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("소셜 프로필을 찾을 수 없습니다: mp_no=%d", mpNo)
	}
	return nil
}

func (r *SocialInviteRepository) AppendMemo(mbID string, text string) error {
	return r.db.Exec(
		"UPDATE g5_member SET mb_memo = CONCAT(?, '\n', mb_memo) WHERE mb_id = ?",
		text,
		mbID,
	).Error
}

func (r *SocialInviteRepository) WriteRecoveryLog(mbID, adminID, action, details string) error {
	return r.db.Create(&domain.RecoveryLog{
		MbID:    mbID,
		AdminID: adminID,
		Action:  action,
		Details: details,
	}).Error
}
