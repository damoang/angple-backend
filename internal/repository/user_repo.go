package repository

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// UserRepository 사용자 저장소 인터페이스
type UserRepository interface {
	Create(user *domain.User) error
	FindByID(id uint) (*domain.User, error)
	FindByEmail(email string) (*domain.User, error)
	Update(user *domain.User) error
	Delete(id uint) error
}

// userRepository GORM 구현체
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository 생성자
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

// Create 사용자 생성
func (r *userRepository) Create(user *domain.User) error {
	return r.db.Create(user).Error
}

// FindByID ID로 사용자 조회
func (r *userRepository) FindByID(id uint) (*domain.User, error) {
	var user domain.User
	err := r.db.Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByEmail 이메일로 사용자 조회
func (r *userRepository) FindByEmail(email string) (*domain.User, error) {
	var user domain.User
	err := r.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update 사용자 정보 업데이트
func (r *userRepository) Update(user *domain.User) error {
	return r.db.Save(user).Error
}

// Delete 사용자 삭제 (소프트 삭제)
func (r *userRepository) Delete(id uint) error {
	return r.db.Delete(&domain.User{}, id).Error
}
