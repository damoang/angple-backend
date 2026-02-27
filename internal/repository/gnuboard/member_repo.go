package gnuboard

import (
	"fmt"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

// MemberRepository provides access to g5_member table
type MemberRepository interface {
	FindByID(mbID string) (*gnuboard.G5Member, error)
	FindByNo(mbNo int) (*gnuboard.G5Member, error)
	FindByEmail(email string) (*gnuboard.G5Member, error)
	FindNicksByIDs(mbIDs []string) (map[string]string, error)
	FindAll(page, limit int, keyword string) ([]*gnuboard.G5Member, int64, error)
	Create(member *gnuboard.G5Member) error
	Update(member *gnuboard.G5Member) error
	UpdatePassword(mbID string, hashedPassword string) error
	Count() (int64, error)
}

type memberRepository struct {
	db *gorm.DB
}

// NewMemberRepository creates a new Gnuboard MemberRepository
func NewMemberRepository(db *gorm.DB) MemberRepository {
	return &memberRepository{db: db}
}

// FindByID finds a member by mb_id (username)
func (r *memberRepository) FindByID(mbID string) (*gnuboard.G5Member, error) {
	var member gnuboard.G5Member
	err := r.db.Where("mb_id = ?", mbID).First(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// FindByNo finds a member by mb_no (primary key)
func (r *memberRepository) FindByNo(mbNo int) (*gnuboard.G5Member, error) {
	var member gnuboard.G5Member
	err := r.db.Where("mb_no = ?", mbNo).First(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// FindByEmail finds a member by email
func (r *memberRepository) FindByEmail(email string) (*gnuboard.G5Member, error) {
	var member gnuboard.G5Member
	err := r.db.Where("mb_email = ?", email).First(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// FindNicksByIDs batch-loads nicknames for given mb_id values
// Returns: map["user1"]"닉네임"
func (r *memberRepository) FindNicksByIDs(mbIDs []string) (map[string]string, error) {
	if len(mbIDs) == 0 {
		return map[string]string{}, nil
	}

	type row struct {
		MbID   string `gorm:"column:mb_id"`
		MbNick string `gorm:"column:mb_nick"`
	}
	var rows []row
	err := r.db.Table("g5_member").
		Select("mb_id, mb_nick").
		Where("mb_id IN ?", mbIDs).
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

// FindAll returns all members with pagination and optional keyword search
func (r *memberRepository) FindAll(page, limit int, keyword string) ([]*gnuboard.G5Member, int64, error) {
	var members []*gnuboard.G5Member
	var total int64

	query := r.db.Model(&gnuboard.G5Member{}).
		Where("mb_leave_date = ''").
		Where("mb_intercept_date = ''")

	if keyword != "" {
		like := fmt.Sprintf("%%%s%%", keyword)
		query = query.Where("mb_id LIKE ? OR mb_nick LIKE ? OR mb_email LIKE ?", like, like, like)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	err := query.Order("mb_no DESC").Offset(offset).Limit(limit).Find(&members).Error
	return members, total, err
}

// Create creates a new member
func (r *memberRepository) Create(member *gnuboard.G5Member) error {
	return r.db.Create(member).Error
}

// Update updates an existing member
func (r *memberRepository) Update(member *gnuboard.G5Member) error {
	return r.db.Save(member).Error
}

// UpdatePassword updates only the password field
func (r *memberRepository) UpdatePassword(mbID string, hashedPassword string) error {
	return r.db.Model(&gnuboard.G5Member{}).
		Where("mb_id = ?", mbID).
		Update("mb_password", hashedPassword).Error
}

// Count returns the total number of active members
func (r *memberRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&gnuboard.G5Member{}).
		Where("mb_leave_date = ''").
		Where("mb_intercept_date = ''").
		Count(&count).Error
	return count, err
}
