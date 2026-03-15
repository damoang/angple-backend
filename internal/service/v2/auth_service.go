package v2

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/damoang/angple-backend/pkg/jwt"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// V2AuthService handles v2 authentication with bcrypt + legacy password support
//
//nolint:revive
type V2AuthService struct {
	userRepo   v2repo.UserRepository
	jwtManager *jwt.Manager
	expRepo    v2repo.ExpRepository
	notiRepo   gnurepo.NotiRepository
	db         *gorm.DB
}

// NewV2AuthService creates a new V2AuthService
func NewV2AuthService(userRepo v2repo.UserRepository, jwtManager *jwt.Manager, expRepo v2repo.ExpRepository) *V2AuthService {
	return &V2AuthService{
		userRepo:   userRepo,
		jwtManager: jwtManager,
		expRepo:    expRepo,
	}
}

// SetPromotionDeps sets dependencies needed for auto-promotion on login
func (s *V2AuthService) SetPromotionDeps(db *gorm.DB, notiRepo gnurepo.NotiRepository) {
	s.db = db
	s.notiRepo = notiRepo
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

	// 이용제한 사용자도 로그인 가능 (소명게시판 접근 허용)
	// banned 상태 체크 제거
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

	// Grant daily login XP synchronously (mb_login_days must be updated before promotion check)
	if s.expRepo != nil {
		s.grantLoginXP(username)
	}

	// Check auto-promotion (2→3) and update level if promoted
	level := int(user.Level)
	if promoted, newLevel := s.checkAndPromote(user.Username); promoted {
		level = newLevel
		user.Level = uint8(min(newLevel, 255)) //nolint:gosec // level values are small (2-10)
		// Update v2_users level (best-effort)
		_ = s.userRepo.Update(user)
	}

	// Generate JWT tokens
	userIDStr := strconv.FormatUint(user.ID, 10)
	accessToken, err := s.jwtManager.GenerateAccessToken(userIDStr, user.Username, user.Nickname, level)
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

// grantLoginXP grants daily login XP to a user (best-effort, panics are recovered)
func (s *V2AuthService) grantLoginXP(username string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[v2-auth] login XP panic recovered for user %s: %v", username, r)
		}
	}()

	xpConfig, cfgErr := s.expRepo.GetXPConfig()
	if cfgErr != nil {
		log.Printf("[v2-auth] XP config read failed for user %s: %v", username, cfgErr)
		return
	}
	if !xpConfig.LoginEnabled || xpConfig.LoginXP <= 0 {
		return
	}

	today := time.Now().Format("2006-01-02")

	already, err := s.expRepo.HasTodayAction(username, today)
	if err != nil {
		log.Printf("[v2-auth] login XP check failed for user %s: %v", username, err)
		return
	}
	if already {
		return
	}
	if _, addErr := s.expRepo.AddExp(username, xpConfig.LoginXP, today+" 로그인", "@login", username, today); addErr != nil {
		log.Printf("[v2-auth] login XP grant failed for user %s: %v", username, addErr)
	}

	// 서로 다른 날 로그인 횟수 증가 (자동등업 조건)
	if err := s.expRepo.IncrementLoginDays(username); err != nil {
		log.Printf("[v2-auth] login days increment failed for user %s: %v", username, err)
	}
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
	newAccess, err := s.jwtManager.GenerateAccessToken(userIDStr, user.Username, user.Nickname, int(user.Level))
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

// verifyPassword checks password against bcrypt hash
func verifyPassword(plain, hashed string) bool {
	if isBcryptHash(hashed) {
		return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)) == nil
	}
	return false
}

// isBcryptHash checks if the hash is bcrypt format ($2a$, $2b$, $2y$)
func isBcryptHash(hash string) bool {
	return len(hash) == 60 && (hash[:4] == "$2a$" || hash[:4] == "$2b$" || hash[:4] == "$2y$")
}

// checkAndPromote checks if the user meets auto-promotion criteria and promotes them.
// Currently supports 2→3 (앙님) promotion.
func (s *V2AuthService) checkAndPromote(mbID string) (bool, int) {
	if s.db == nil {
		return false, 0
	}

	var member struct {
		MbLevel   int `gorm:"column:mb_level"`
		LoginDays int `gorm:"column:mb_login_days"`
		Exp       int `gorm:"column:as_exp"`
	}
	if err := s.db.Table("g5_member").
		Select("mb_level, mb_login_days, as_exp").
		Where("mb_id = ?", mbID).
		First(&member).Error; err != nil {
		log.Printf("[v2-auth] checkAndPromote: member query failed for %s: %v", mbID, err)
		return false, 0
	}

	// 2→3 (앙님) 조건: 로그인 7일 이상 + 경험치 3000 이상
	if member.MbLevel == 2 && member.LoginDays >= 7 && member.Exp >= 3000 {
		if err := s.db.Table("g5_member").
			Where("mb_id = ?", mbID).
			Update("mb_level", 3).Error; err != nil {
			log.Printf("[v2-auth] checkAndPromote: level update failed for %s: %v", mbID, err)
			return false, member.MbLevel
		}
		log.Printf("[v2-auth] auto-promoted %s from level 2 to 3", mbID)
		go s.sendPromotionNotification(mbID)
		return true, 3
	}

	return false, member.MbLevel
}

// sendPromotionNotification sends a promotion notification (best-effort)
func (s *V2AuthService) sendPromotionNotification(mbID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[v2-auth] promotion notification panic for %s: %v", mbID, r)
		}
	}()

	if s.notiRepo == nil {
		return
	}

	noti := &gnurepo.Notification{
		MbID:          mbID,
		PhFromCase:    "promote",
		PhToCase:      "me",
		BoTable:       "@system",
		WrID:          0,
		RelMbID:       "system",
		RelMbNick:     "다모앙",
		RelMsg:        "💛 앙님(💛)으로 되었습니다. 앞으로도 다모앙에서 즐거운 시간 보내세요!",
		RelURL:        "/my",
		PhReaded:      "N",
		ParentSubject: "축하합니다.",
	}
	if err := s.notiRepo.Create(noti); err != nil {
		log.Printf("[v2-auth] promotion notification failed for %s: %v", mbID, err)
	}
}
