package domain

import (
	"time"
)

// User 사용자 도메인 모델 (Laravel users 테이블과 매핑)
type User struct {
	ID              uint       `gorm:"column:id;primaryKey" json:"id"`
	Name            string     `gorm:"column:name;type:varchar(255)" json:"name"`
	Email           string     `gorm:"column:email;type:varchar(255);uniqueIndex" json:"email"`
	EmailVerifiedAt *time.Time `gorm:"column:email_verified_at" json:"email_verified_at,omitempty"`
	Password        string     `gorm:"column:password;type:varchar(255)" json:"-"` // JSON에서 제외
	RememberToken   *string    `gorm:"column:remember_token;type:varchar(100)" json:"-"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt       *time.Time `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`

	// 추가 필드 (Laravel에 있을 수 있는 필드들)
	ProfileImageURL *string    `gorm:"column:profile_image_url;type:varchar(500)" json:"profile_image_url,omitempty"`
	Bio             *string    `gorm:"column:bio;type:text" json:"bio,omitempty"`
	LastLoginAt     *time.Time `gorm:"column:last_login_at" json:"last_login_at,omitempty"`
	IsActive        bool       `gorm:"column:is_active;default:true" json:"is_active"`

	// 관계 (Lazy Loading)
	Posts []Post `gorm:"foreignKey:UserID" json:"posts,omitempty"`
}

// TableName Laravel 테이블명 규칙
func (User) TableName() string {
	return "users"
}

// UserResponse API 응답용 구조체
type UserResponse struct {
	ID              uint      `json:"id"`
	Name            string    `json:"name"`
	Email           string    `json:"email"`
	ProfileImageURL *string   `json:"profile_image_url,omitempty"`
	Bio             *string   `json:"bio,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// ToResponse User를 UserResponse로 변환
func (u *User) ToResponse() *UserResponse {
	return &UserResponse{
		ID:              u.ID,
		Name:            u.Name,
		Email:           u.Email,
		ProfileImageURL: u.ProfileImageURL,
		Bio:             u.Bio,
		CreatedAt:       u.CreatedAt,
	}
}
