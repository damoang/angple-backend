package v2

import (
	"fmt"
	"time"

	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// UserRepository v2 user data access
type UserRepository interface {
	FindByID(id uint64) (*v2.V2User, error)
	FindByUsername(username string) (*v2.V2User, error)
	FindByEmail(email string) (*v2.V2User, error)
	Create(user *v2.V2User) error
	Update(user *v2.V2User) error
	FindAll(page, limit int, keyword string) ([]*v2.V2User, int64, error)
	Count() (int64, error)
	CountSince(since time.Time) (int64, error)
	FindNicksByIDs(ids []string) (map[string]string, error)
}

type userRepository struct {
	db *gorm.DB
}

// NewUserRepository creates a new v2 UserRepository
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) FindByID(id uint64) (*v2.V2User, error) {
	var user v2.V2User
	err := r.db.Where("id = ?", id).First(&user).Error
	return &user, err
}

func (r *userRepository) FindByUsername(username string) (*v2.V2User, error) {
	var user v2.V2User
	err := r.db.Where("username = ?", username).First(&user).Error
	return &user, err
}

func (r *userRepository) FindByEmail(email string) (*v2.V2User, error) {
	var user v2.V2User
	err := r.db.Where("email = ?", email).First(&user).Error
	return &user, err
}

func (r *userRepository) Create(user *v2.V2User) error {
	return r.db.Create(user).Error
}

func (r *userRepository) Update(user *v2.V2User) error {
	return r.db.Save(user).Error
}

func (r *userRepository) FindAll(page, limit int, keyword string) ([]*v2.V2User, int64, error) {
	var users []*v2.V2User
	var total int64

	query := r.db.Model(&v2.V2User{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("username LIKE ? OR nickname LIKE ? OR email LIKE ?", like, like, like)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (r *userRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&v2.V2User{}).Count(&count).Error
	return count, err
}

func (r *userRepository) CountSince(since time.Time) (int64, error) {
	var count int64
	err := r.db.Model(&v2.V2User{}).Where("created_at >= ?", since).Count(&count).Error
	return count, err
}

// FindNicksByIDs batch-loads nicknames for given v2_users.id values
// 반환: map["3"]"닉네임"
func (r *userRepository) FindNicksByIDs(ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}

	type row struct {
		ID       uint64 `gorm:"column:id"`
		Nickname string `gorm:"column:nickname"`
	}
	var rows []row
	err := r.db.Table("v2_users").
		Select("id, nickname").
		Where("id IN ?", ids).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[fmt.Sprintf("%d", r.ID)] = r.Nickname
	}
	return m, nil
}
