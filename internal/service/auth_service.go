package service

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/repository"
	"github.com/damoang/angple-backend/pkg/auth"
	"github.com/damoang/angple-backend/pkg/jwt"
)

// RegisterRequest represents a registration request
type RegisterRequest struct {
	UserID   string `json:"user_id" binding:"required"`
	Password string `json:"password" binding:"required"`
	Name     string `json:"name" binding:"required"`
	Nickname string `json:"nickname" binding:"required"`
	Email    string `json:"email" binding:"required"`
}

// AuthService authentication business logic
type AuthService interface {
	Login(userID, password string) (*LoginResponse, error)
	RefreshToken(refreshToken string) (*TokenPair, error)
	Register(req *RegisterRequest) (*domain.MemberResponse, error)
	Withdraw(userID string) error
}

type authService struct {
	memberRepo repository.MemberRepository
	jwtManager *jwt.Manager
	hooks      *plugin.HookManager
}

// LoginResponse login response
type LoginResponse struct {
	User         *domain.MemberResponse `json:"user"`
	AccessToken  string                 `json:"access_token"`
	RefreshToken string                 `json:"refresh_token"`
}

// TokenPair token pair
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// NewAuthService creates a new AuthService
func NewAuthService(memberRepo repository.MemberRepository, jwtManager *jwt.Manager, hooks *plugin.HookManager) AuthService {
	return &authService{
		memberRepo: memberRepo,
		jwtManager: jwtManager,
		hooks:      hooks,
	}
}

// Login authenticates user and returns tokens
func (s *authService) Login(userID, password string) (*LoginResponse, error) {
	// 1. Find member
	member, err := s.memberRepo.FindByUserID(userID)
	if err != nil {
		return nil, common.ErrInvalidCredentials
	}

	// 2. Verify password (legacy Gnuboard hash)
	if !auth.VerifyGnuboardPassword(password, member.Password) {
		return nil, common.ErrInvalidCredentials
	}

	// 3. Generate JWT tokens
	accessToken, err := s.jwtManager.GenerateAccessToken(member.UserID, member.Nickname, member.Level)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.jwtManager.GenerateRefreshToken(member.UserID)
	if err != nil {
		return nil, err
	}

	// 4. Update login time (async)
	go s.memberRepo.UpdateLoginTime(member.UserID) //nolint:errcheck // 비동기 로그인 시간 업데이트, 실패해도 무시

	// 5. Fire after_login hook
	if s.hooks != nil {
		s.hooks.Do(plugin.HookUserAfterLogin, map[string]interface{}{
			"user_id":  member.UserID,
			"nickname": member.Nickname,
			"level":    member.Level,
		})
	}

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         member.ToResponse(),
	}, nil
}

// Register creates a new member account
func (s *authService) Register(req *RegisterRequest) (*domain.MemberResponse, error) {
	// 중복 체크
	exists, err := s.memberRepo.ExistsByUserID(req.UserID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, common.ErrUserAlreadyExists
	}

	exists, err = s.memberRepo.ExistsByEmail(req.Email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("이미 사용 중인 이메일입니다")
	}

	exists, err = s.memberRepo.ExistsByNickname(req.Nickname, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("이미 사용 중인 닉네임입니다")
	}

	// 비밀번호 해싱 (MySQL PASSWORD() 호환)
	hashedPassword := auth.HashPassword(req.Password)

	member := &domain.Member{
		UserID:    req.UserID,
		Password:  hashedPassword,
		Name:      req.Name,
		Nickname:  req.Nickname,
		Email:     req.Email,
		Level:     2, // 기본 회원 레벨
		Point:     0,
		CreatedAt: time.Now(),
	}

	if err := s.memberRepo.Create(member); err != nil {
		return nil, err
	}

	return member.ToResponse(), nil
}

// Withdraw marks a member as withdrawn (data preserved)
func (s *authService) Withdraw(userID string) error {
	member, err := s.memberRepo.FindByUserID(userID)
	if err != nil {
		return common.ErrUserNotFound
	}

	if member.LeaveDate != "" {
		return fmt.Errorf("이미 탈퇴한 회원입니다")
	}

	// 탈퇴일 기록 (데이터 보존, YYYYMMDD 형식)
	leaveDate := time.Now().Format("20060102")
	member.LeaveDate = leaveDate

	return s.memberRepo.Update(member.ID, member)
}

// RefreshToken creates new access token from refresh token
func (s *authService) RefreshToken(refreshToken string) (*TokenPair, error) {
	// 1. Verify refresh token
	claims, err := s.jwtManager.VerifyToken(refreshToken)
	if err != nil {
		return nil, common.ErrInvalidToken
	}

	// 2. Get member info for new access token
	member, err := s.memberRepo.FindByUserID(claims.UserID)
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	// 3. Generate new access token
	accessToken, err := s.jwtManager.GenerateAccessToken(member.UserID, member.Nickname, member.Level)
	if err != nil {
		return nil, err
	}

	// 4. Generate new refresh token
	newRefreshToken, err := s.jwtManager.GenerateRefreshToken(member.UserID)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
	}, nil
}
