package v2

import (
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/damoang/angple-backend/pkg/auth"
	"github.com/damoang/angple-backend/pkg/jwt"
	"golang.org/x/crypto/bcrypt"
)

// V2AuthService handles v2 authentication with bcrypt + legacy password support
//
//nolint:revive
type V2AuthService struct {
	userRepo       v2repo.UserRepository
	jwtManager     *jwt.Manager
	damoangManager *jwt.DamoangManager
}

// NewV2AuthService creates a new V2AuthService
func NewV2AuthService(userRepo v2repo.UserRepository, jwtManager *jwt.Manager, damoangManager *jwt.DamoangManager) *V2AuthService {
	return &V2AuthService{
		userRepo:       userRepo,
		jwtManager:     jwtManager,
		damoangManager: damoangManager,
	}
}

// V2LoginResponse represents v2 login response
//
//nolint:revive
type V2LoginResponse struct {
	User         *v2domain.V2User `json:"user"`
	AccessToken  string           `json:"access_token"`
	RefreshToken string           `json:"refresh_token"`
}

// Login authenticates a user against v2_users table.
// Supports both bcrypt (new) and legacy gnuboard password hashing (migrated users).
func (s *V2AuthService) Login(username, password string) (*V2LoginResponse, error) {
	user, err := s.userRepo.FindByUsername(username)
	if err != nil {
		return nil, common.ErrInvalidCredentials
	}

	if user.Status == "banned" {
		return nil, errors.New("account is banned")
	}
	if user.Status == "inactive" {
		return nil, errors.New("account is inactive")
	}

	// Try bcrypt first (new accounts), then legacy gnuboard hashing (migrated)
	if !verifyPassword(password, user.Password) {
		return nil, common.ErrInvalidCredentials
	}

	// If the password is legacy format, upgrade to bcrypt (best-effort, non-blocking)
	if !isBcryptHash(user.Password) {
		if upgraded, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost); hashErr == nil {
			user.Password = string(upgraded)
			if updateErr := s.userRepo.Update(user); updateErr != nil {
				log.Printf("[v2-auth] password upgrade failed for user %s: %v", username, updateErr)
			}
		}
	}

	// Generate JWT tokens
	userIDStr := strconv.FormatUint(user.ID, 10)
	accessToken, err := s.jwtManager.GenerateAccessToken(userIDStr, user.Nickname, int(user.Level))
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}
	refreshToken, err := s.jwtManager.GenerateRefreshToken(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	return &V2LoginResponse{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// RefreshToken validates a refresh token and issues new token pair
func (s *V2AuthService) RefreshToken(refreshToken string) (*V2LoginResponse, error) {
	claims, err := s.jwtManager.VerifyToken(refreshToken)
	if err != nil {
		return nil, common.ErrUnauthorized
	}

	userID, err := strconv.ParseUint(claims.UserID, 10, 64)
	if err != nil {
		return nil, common.ErrUnauthorized
	}

	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, common.ErrUnauthorized
	}

	userIDStr := strconv.FormatUint(user.ID, 10)
	newAccess, err := s.jwtManager.GenerateAccessToken(userIDStr, user.Nickname, int(user.Level))
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}
	newRefresh, err := s.jwtManager.GenerateRefreshToken(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	return &V2LoginResponse{
		User:         user,
		AccessToken:  newAccess,
		RefreshToken: newRefresh,
	}, nil
}

// GetCurrentUser returns the user for the given ID
func (s *V2AuthService) GetCurrentUser(userID uint64) (*v2domain.V2User, error) {
	return s.userRepo.FindByID(userID)
}

// ExchangeGnuboardJWT verifies a damoang.net JWT and issues angple JWT tokens
func (s *V2AuthService) ExchangeGnuboardJWT(gnuJwt string) (*V2LoginResponse, error) {
	// Verify damoang.net JWT
	claims, err := s.damoangManager.VerifyToken(gnuJwt)
	if err != nil {
		return nil, fmt.Errorf("invalid damoang_jwt: %w", err)
	}

	// Get user info from claims
	mbID := claims.GetUserID()
	if mbID == "" {
		return nil, errors.New("mb_id not found in token")
	}
	mbName := claims.GetUserName()
	mbLevel := claims.GetUserLevel()

	// Find or create user by username (mb_id)
	user, err := s.userRepo.FindByUsername(mbID)
	if err != nil {
		// User doesn't exist in v2_users, create from JWT claims
		log.Printf("[ExchangeJWT] User %s not found in v2_users, creating...", mbID)
		user = &v2domain.V2User{
			Username: mbID,
			Nickname: mbName,
			Email:    claims.MbEmail,
			Level:    uint8(mbLevel),
			Status:   "active",
		}
		if createErr := s.userRepo.Create(user); createErr != nil {
			return nil, fmt.Errorf("failed to create user: %w", createErr)
		}
		log.Printf("[ExchangeJWT] Created user %s with ID %d", mbID, user.ID)
	}

	if user.Status == "banned" {
		return nil, errors.New("account is banned")
	}
	if user.Status == "inactive" {
		return nil, errors.New("account is inactive")
	}

	// Generate angple JWT tokens
	userIDStr := strconv.FormatUint(user.ID, 10)
	accessToken, err := s.jwtManager.GenerateAccessToken(userIDStr, user.Nickname, int(user.Level))
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}
	refreshToken, err := s.jwtManager.GenerateRefreshToken(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	return &V2LoginResponse{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// verifyPassword checks password against bcrypt or legacy gnuboard hash
func verifyPassword(plain, hashed string) bool {
	// Try bcrypt first
	if isBcryptHash(hashed) {
		return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)) == nil
	}
	// Fallback to legacy gnuboard password verification
	return auth.VerifyGnuboardPassword(plain, hashed)
}

// isBcryptHash checks if the hash is bcrypt format ($2a$, $2b$, $2y$)
func isBcryptHash(hash string) bool {
	return len(hash) == 60 && (hash[:4] == "$2a$" || hash[:4] == "$2b$" || hash[:4] == "$2y$")
}
