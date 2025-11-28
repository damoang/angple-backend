package repository

import (
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// MemberRepository member data access interface
type MemberRepository interface {
	// Read operations
	FindByUserID(userID string) (*domain.Member, error)
	FindByEmail(email string) (*domain.Member, error)
	FindByID(id int) (*domain.Member, error)

	// Write operations
	Create(member *domain.Member) error
	Update(id int, member *domain.Member) error
	UpdateLoginTime(userID string) error
	UpdatePassword(userID string, hashedPassword string) error

	// Validation operations
	ExistsByUserID(userID string) (bool, error)
	ExistsByEmail(email string) (bool, error)
}

type memberRepository struct {
	db *gorm.DB
}

// NewMemberRepository creates a new MemberRepository
func NewMemberRepository(db *gorm.DB) MemberRepository {
	return &memberRepository{db: db}
}

// FindByUserID finds member by user ID
func (r *memberRepository) FindByUserID(userID string) (*domain.Member, error) {
	var member domain.Member
	err := r.db.Where("mb_id = ?", userID).First(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// FindByEmail finds member by email
func (r *memberRepository) FindByEmail(email string) (*domain.Member, error) {
	var member domain.Member
	err := r.db.Where("mb_email = ?", email).First(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// FindByID finds member by ID
func (r *memberRepository) FindByID(id int) (*domain.Member, error) {
	var member domain.Member
	err := r.db.Where("mb_no = ?", id).First(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// Create creates a new member
func (r *memberRepository) Create(member *domain.Member) error {
	member.CreatedAt = time.Now()
	return r.db.Create(member).Error
}

// Update updates member information
func (r *memberRepository) Update(id int, member *domain.Member) error {
	return r.db.Model(&domain.Member{}).
		Where("mb_no = ?", id).
		Updates(member).Error
}

// UpdateLoginTime updates last login time
func (r *memberRepository) UpdateLoginTime(userID string) error {
	return r.db.Model(&domain.Member{}).
		Where("mb_id = ?", userID).
		Update("mb_today_login", time.Now()).Error
}

// UpdatePassword updates member password
func (r *memberRepository) UpdatePassword(userID string, hashedPassword string) error {
	return r.db.Model(&domain.Member{}).
		Where("mb_id = ?", userID).
		Update("mb_password", hashedPassword).Error
}

// ExistsByUserID checks if user ID exists
func (r *memberRepository) ExistsByUserID(userID string) (bool, error) {
	var count int64
	err := r.db.Model(&domain.Member{}).
		Where("mb_id = ?", userID).
		Count(&count).Error
	return count > 0, err
}

// ExistsByEmail checks if email exists
func (r *memberRepository) ExistsByEmail(email string) (bool, error) {
	var count int64
	err := r.db.Model(&domain.Member{}).
		Where("mb_email = ?", email).
		Count(&count).Error
	return count > 0, err
}
