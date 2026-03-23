package v2

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"github.com/damoang/angple-backend/internal/memberlevel"
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
// Supports both bcrypt (new accounts), then legacy gnuboard hashing (migrated users).
func (s *V2AuthService) Login(username, password string) (*V2LoginResponse, error) {
	user, err := s.userRepo.FindByUsername(username)
	if err != nil {
		return nil, common.ErrInvalidCredentials
	}

	if user.Status == "inactive" {
		return nil, errors.New("account is inactive")
	}

	if !verifyPassword(password, user.Password) {
		return nil, common.ErrInvalidCredentials
	}

	if !isBcryptHash(user.Password) {
		if upgraded, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost); hashErr == nil {
			user.Password = string(upgraded)
			if updateErr := s.userRepo.Update(user); updateErr != nil {
				log.Printf("[v2-auth] password upgrade failed for user %s: %v", username, updateErr)
			}
		}
	}

	if s.expRepo != nil {
		s.grantLoginXP(username)
	}

	level := int(user.Level)
	if promoted, newLevel := s.checkAndPromote(user.Username); promoted {
		level = newLevel
		user.Level = uint8(min(newLevel, 255)) //nolint:gosec
		_ = s.userRepo.Update(user)
	}

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

func (s *V2AuthService) GetCurrentUser(userID uint64) (*v2domain.V2User, error) {
	return s.userRepo.FindByID(userID)
}

func verifyPassword(plain, hashed string) bool {
	if isBcryptHash(hashed) {
		return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)) == nil
	}
	return false
}

func isBcryptHash(hash string) bool {
	return len(hash) == 60 && (hash[:4] == "$2a$" || hash[:4] == "$2b$" || hash[:4] == "$2y$")
}

func isEligibleForAutoPromotion(level, loginDays, exp int, certify string) bool {
	return level == 2 && loginDays >= 7 && exp >= 3000 && certify != ""
}

func (s *V2AuthService) checkAndPromote(mbID string) (bool, int) {
	if s.db == nil {
		return false, 0
	}

	var (
		previousLevel int
		promoted      bool
		newLevel      int
	)

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var member gnudomain.G5Member
		if err := tx.Table("g5_member").
			Select("mb_id, mb_level, mb_login_days, as_exp, as_level, mb_certify, mb_datetime").
			Where("mb_id = ?", mbID).
			Take(&member).Error; err != nil {
			return err
		}

		previousLevel = member.MbLevel
		newLevel = member.MbLevel
		if !isEligibleForAutoPromotion(member.MbLevel, member.MbLoginDays, member.AsExp, member.MbCertify) {
			return nil
		}

		result := tx.Table("g5_member").
			Where("mb_id = ? AND mb_level = ?", mbID, member.MbLevel).
			Update("mb_level", 3)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}

		if err := memberlevel.RecordPromotion(tx, &member, 3, memberlevel.ReasonAutoPromoteLoginAPI); err != nil {
			if memberlevel.IsMissingHistoryTableError(err) {
				log.Printf("[v2-auth] member level history table missing; promotion logged skipped for %s: %v", mbID, err)
			} else {
				return err
			}
		}

		promoted = true
		newLevel = 3
		return nil
	})
	if err != nil {
		log.Printf("[v2-auth] checkAndPromote failed for %s: %v", mbID, err)
		return false, previousLevel
	}
	if promoted {
		log.Printf("[v2-auth] auto-promoted %s from level %d to %d", mbID, previousLevel, newLevel)
		go s.sendPromotionNotification(mbID)
	}

	return promoted, newLevel
}

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
