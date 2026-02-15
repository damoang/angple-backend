package repository

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// SingoUserRepository — singo_users 읽기 전용 접근
type SingoUserRepository struct {
	db *gorm.DB
}

func NewSingoUserRepository(db *gorm.DB) *SingoUserRepository {
	return &SingoUserRepository{db: db}
}

// FindByMbID — mb_id로 Singo 사용자 조회
func (r *SingoUserRepository) FindByMbID(mbID string) (*domain.SingoUser, error) {
	var user domain.SingoUser
	err := r.db.Where("mb_id = ?", mbID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByMbNo — g5_member.mb_no(숫자 PK)로 조회 (JWT user_id가 g5_member.mb_no인 경우)
func (r *SingoUserRepository) FindByMbNo(mbNo string) (*domain.SingoUser, error) {
	var user domain.SingoUser
	err := r.db.Table("singo_users").
		Joins("JOIN g5_member ON singo_users.mb_id COLLATE utf8mb4_unicode_ci = g5_member.mb_id").
		Where("g5_member.mb_no = ?", mbNo).
		Select("singo_users.*").
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByV2UserID — v2_users.id(숫자 PK)로 조회 (JWT user_id가 v2_users.id인 경우)
func (r *SingoUserRepository) FindByV2UserID(v2ID string) (*domain.SingoUser, error) {
	var user domain.SingoUser
	err := r.db.Table("singo_users").
		Joins("JOIN v2_users ON singo_users.mb_id COLLATE utf8mb4_unicode_ci = v2_users.username").
		Where("v2_users.id = ?", v2ID).
		Select("singo_users.*").
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindAll — 모든 Singo 사용자 조회 (검토자 현황용)
func (r *SingoUserRepository) FindAll() ([]domain.SingoUser, error) {
	var users []domain.SingoUser
	err := r.db.Find(&users).Error
	return users, err
}

// FindAllWithNick — 모든 Singo 사용자 + 닉네임 조회
func (r *SingoUserRepository) FindAllWithNick() ([]domain.SingoUserWithNick, error) {
	var users []domain.SingoUserWithNick
	err := r.db.Table("singo_users").
		Select("singo_users.id, singo_users.mb_id, singo_users.role, COALESCE(g5_member.mb_nick, '') as mb_nick").
		Joins("LEFT JOIN g5_member ON singo_users.mb_id COLLATE utf8mb4_unicode_ci = g5_member.mb_id").
		Order("singo_users.id ASC").
		Find(&users).Error
	return users, err
}

// FindByMbIDWithNick — mb_id로 Singo 사용자 + 닉네임 조회
func (r *SingoUserRepository) FindByMbIDWithNick(mbID string) (*domain.SingoUserWithNick, error) {
	var user domain.SingoUserWithNick
	err := r.db.Table("singo_users").
		Select("singo_users.id, singo_users.mb_id, singo_users.role, COALESCE(g5_member.mb_nick, '') as mb_nick").
		Joins("LEFT JOIN g5_member ON singo_users.mb_id COLLATE utf8mb4_unicode_ci = g5_member.mb_id").
		Where("singo_users.mb_id = ?", mbID).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Create — Singo 사용자 추가
func (r *SingoUserRepository) Create(mbID, role string) error {
	user := domain.SingoUser{
		MbID: mbID,
		Role: role,
	}
	return r.db.Create(&user).Error
}

// UpdateRole — Singo 사용자 역할 변경
func (r *SingoUserRepository) UpdateRole(mbID, role string) error {
	return r.db.Model(&domain.SingoUser{}).
		Where("mb_id = ?", mbID).
		Update("role", role).Error
}

// DeleteByMbID — Singo 사용자 삭제
func (r *SingoUserRepository) DeleteByMbID(mbID string) error {
	return r.db.Where("mb_id = ?", mbID).Delete(&domain.SingoUser{}).Error
}
