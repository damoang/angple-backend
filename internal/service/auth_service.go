package service

import (
	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
	"github.com/damoang/angple-backend/pkg/auth"
	"github.com/damoang/angple-backend/pkg/jwt"
)

// AuthService authentication business logic
type AuthService interface {
	Login(userID, password string) (*LoginResponse, error)
	RefreshToken(refreshToken string) (*TokenPair, error)
}

type authService struct {
	memberRepo repository.MemberRepository
	jwtManager *jwt.Manager
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
func NewAuthService(memberRepo repository.MemberRepository, jwtManager *jwt.Manager) AuthService {
	return &authService{
		memberRepo: memberRepo,
		jwtManager: jwtManager,
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

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         member.ToResponse(),
	}, nil
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
