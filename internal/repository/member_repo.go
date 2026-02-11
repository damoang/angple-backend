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
	FindByNickname(nickname string) (*domain.Member, error)
	FindNicksByIDs(userIDs []string) (map[string]string, error)

	// Write operations
	Create(member *domain.Member) error
	Update(id int, member *domain.Member) error
	UpdateLoginTime(userID string) error
	UpdatePassword(userID string, hashedPassword string) error

	// Validation operations
	ExistsByUserID(userID string) (bool, error)
	ExistsByEmail(email string) (bool, error)
	ExistsByNickname(nickname string, excludeUserID string) (bool, error)
	ExistsByPhone(phone string, excludeUserID string) (bool, error)
	ExistsByEmailExcluding(email string, excludeUserID string) (bool, error)

	// Admin operations
	FindAll(page, limit int, keyword string) ([]*domain.Member, int64, error)
	UpdateFields(id int, fields map[string]interface{}) error
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

// FindByNickname finds member by nickname
func (r *memberRepository) FindByNickname(nickname string) (*domain.Member, error) {
	var member domain.Member
	err := r.db.Where("mb_nick = ?", nickname).First(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// ExistsByNickname checks if nickname exists (excluding specified user)
func (r *memberRepository) ExistsByNickname(nickname string, excludeUserID string) (bool, error) {
	var count int64
	query := r.db.Model(&domain.Member{}).Where("mb_nick = ?", nickname)
	if excludeUserID != "" {
		query = query.Where("mb_id <> ?", excludeUserID)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

// ExistsByPhone checks if phone number exists (excluding specified user)
func (r *memberRepository) ExistsByPhone(phone string, excludeUserID string) (bool, error) {
	var count int64
	query := r.db.Model(&domain.Member{}).Where("mb_hp = ?", phone)
	if excludeUserID != "" {
		query = query.Where("mb_id <> ?", excludeUserID)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

// ExistsByEmailExcluding checks if email exists (excluding specified user)
func (r *memberRepository) ExistsByEmailExcluding(email string, excludeUserID string) (bool, error) {
	var count int64
	query := r.db.Model(&domain.Member{}).Where("mb_email = ?", email)
	if excludeUserID != "" {
		query = query.Where("mb_id <> ?", excludeUserID)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

// FindAll returns paginated member list with optional keyword search
func (r *memberRepository) FindAll(page, limit int, keyword string) ([]*domain.Member, int64, error) {
	var members []*domain.Member
	var total int64

	query := r.db.Model(&domain.Member{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("mb_id LIKE ? OR mb_nick LIKE ? OR mb_email LIKE ? OR mb_name LIKE ?", like, like, like, like)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("mb_no DESC").Offset(offset).Limit(limit).Find(&members).Error; err != nil {
		return nil, 0, err
	}
	return members, total, nil
}

// FindNicksByIDs batch-loads nicknames for given user IDs (N+1 prevention)
func (r *memberRepository) FindNicksByIDs(userIDs []string) (map[string]string, error) {
	if len(userIDs) == 0 {
		return map[string]string{}, nil
	}
	type row struct {
		MbID   string `gorm:"column:mb_id"`
		MbNick string `gorm:"column:mb_nick"`
	}
	var rows []row
	err := r.db.Table("g5_member").
		Select("mb_id, mb_nick").
		Where("mb_id IN ?", userIDs).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.MbID] = r.MbNick
	}
	return m, nil
}

// UpdateFields updates specific fields of a member
func (r *memberRepository) UpdateFields(id int, fields map[string]interface{}) error {
	return r.db.Model(&domain.Member{}).Where("mb_no = ?", id).Updates(fields).Error
}
