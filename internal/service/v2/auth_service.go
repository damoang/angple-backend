package v2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"slices"
	"strconv"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"github.com/damoang/angple-backend/internal/memberlevel"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/redis/go-redis/v9"
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
	redis      *redis.Client
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

// SetRedis sets the Redis client used for one-time (jti) app-login code replay protection.
func (s *V2AuthService) SetRedis(rc *redis.Client) {
	s.redis = rc
}

// V2LoginResponse represents v2 login response
//
//nolint:revive
type V2LoginResponse struct {
	User         *v2domain.V2User `json:"user"`
	AccessToken  string           `json:"access_token"`
	RefreshToken string           `json:"refresh_token"`
	// WithdrawalGrace 가 non-nil 이면 대상이 탈퇴 숙려중(취소 가능)이다. 정상 로그인 성공이 아니며
	// 프론트는 취소 UI 를 노출해야 한다. 취소(DELETE /members/me/leave) 호출을 위해 토큰은 함께 발급된다.
	WithdrawalGrace *WithdrawalGraceInfo `json:"withdrawal_grace,omitempty"`
}

// WithdrawalGraceInfo 는 숙려중 로그인 시 프론트가 참조하는 상태 정보다.
type WithdrawalGraceInfo struct {
	LeaveDate     string `json:"leave_date"`
	Deadline      string `json:"deadline"`
	DaysRemaining int    `json:"days_remaining"`
}

// checkWithdrawal 는 g5_member.mb_leave_date 로 회원 탈퇴 상태를 판정한다.
// s.db 미설정(테스트 등) 시에는 WithdrawalNone 으로 fail-open 한다.
func (s *V2AuthService) checkWithdrawal(username string, now time.Time) (common.WithdrawalState, time.Time) {
	if s.db == nil {
		return common.WithdrawalNone, time.Time{}
	}
	var leaveDate string
	err := s.db.Table("g5_member").Select("mb_leave_date").
		Where("mb_id = ?", username).Row().Scan(&leaveDate)
	if err != nil {
		return common.WithdrawalNone, time.Time{}
	}
	return common.ClassifyWithdrawal(leaveDate, now)
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

	// 탈퇴 숙려기간 분기: mb_leave_date 세팅됨 → 정상 로그인 대신 상태 반환.
	//   - 숙려중(30일 미경과): 취소 가능. 토큰은 발급하되 WithdrawalGrace 표시(리프레시 쿠키는 핸들러에서 미설정).
	//   - 확정(30일 경과): 이미 익명화 → 로그인 불가.
	switch state, deadline := s.checkWithdrawal(user.Username, time.Now()); state {
	case common.WithdrawalConfirmed:
		return nil, common.ErrAccountWithdrawn
	case common.WithdrawalGrace:
		userIDStr := strconv.FormatUint(user.ID, 10)
		accessToken, err := s.jwtManager.GenerateAccessToken(userIDStr, user.Username, user.Nickname, int(user.Level))
		if err != nil {
			return nil, fmt.Errorf("generate access token: %w", err)
		}
		refreshToken, err := s.jwtManager.GenerateRefreshToken(userIDStr)
		if err != nil {
			return nil, fmt.Errorf("generate refresh token: %w", err)
		}
		days := int(time.Until(deadline).Hours() / 24)
		if days < 0 {
			days = 0
		}
		return &V2LoginResponse{
			User:         user,
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			WithdrawalGrace: &WithdrawalGraceInfo{
				LeaveDate:     deadline.AddDate(0, 0, -common.WithdrawalGraceDays).Format("20060102"),
				Deadline:      deadline.Format("2006-01-02"),
				DaysRemaining: days,
			},
		}, nil
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

// AppLoginAudience is the audience claim of one-time app-login codes minted by
// the damoang.net web frontend after a social OAuth login (shared JWT_SECRET).
const AppLoginAudience = "app-login"

// AppExchangeLogin exchanges a short-lived app-login code for a v2 token pair.
// The code is an HS256 JWT minted by the web (same JWT_SECRET) with
// aud="app-login", sub=<g5 mb_id> and nickname/email/level claims.
// If the member has no v2_users row yet, one is auto-provisioned from claims.
func (s *V2AuthService) AppExchangeLogin(code string) (*V2LoginResponse, error) {
	claims, err := s.jwtManager.VerifyToken(code)
	if err != nil {
		return nil, common.ErrUnauthorized
	}

	// Only accept dedicated app-login codes (not access/refresh tokens).
	if !slices.Contains(claims.Audience, AppLoginAudience) {
		return nil, common.ErrUnauthorized
	}

	// One-time use: reject replay of the same code within its lifetime (jti).
	// SetNX succeeds on first use; redis.Nil means the jti was already consumed → reject.
	// Fail-open on other Redis errors — replay protection is defense-in-depth, login availability wins.
	if jti := claims.ID; jti != "" && s.redis != nil {
		if _, err := s.redis.SetArgs(context.Background(), "applogin:jti:"+jti, "1",
			redis.SetArgs{Mode: "NX", TTL: 90 * time.Second}).Result(); errors.Is(err, redis.Nil) {
			return nil, common.ErrUnauthorized
		}
	}

	mbID := claims.Subject
	if mbID == "" {
		return nil, common.ErrUnauthorized
	}

	user, err := s.userRepo.FindByUsername(mbID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("find user %s: %w", mbID, err)
		}
		// Auto-provision: member exists in g5_member (web verified) but not in v2_users.
		level := claims.Level
		if level < 1 {
			level = 1
		}
		if level > 255 {
			level = 255
		}
		nickname := claims.Nickname
		if nickname == "" {
			nickname = mbID
		}
		user = &v2domain.V2User{
			Username: mbID,
			Nickname: nickname,
			Email:    claims.Email,
			Level:    uint8(level), // #nosec G115 -- level clamped to [1,255] above
			Status:   "active",
		}
		if createErr := s.userRepo.Create(user); createErr != nil {
			return nil, fmt.Errorf("provision user %s: %w", mbID, createErr)
		}
	}

	if user.Status == "inactive" || user.Status == "banned" {
		return nil, common.ErrUnauthorized
	}

	if s.expRepo != nil {
		s.grantLoginXP(user.Username)
	}

	userIDStr := strconv.FormatUint(user.ID, 10)
	accessToken, err := s.jwtManager.GenerateAccessToken(userIDStr, user.Username, user.Nickname, int(user.Level))
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

// refreshWebIssuedToken validates a refresh token minted by the damoang.net web frontend.
//
// 웹 토큰은 sub=<g5 mb_id> 만 담고 user_id 클레임이 없다. 서명(shared JWT_SECRET)만으로
// 수용하면 로그아웃·로테이션으로 폐기된 토큰까지 통과하므로, 웹이 관리하는 저장소
// (angple_refresh_tokens: sha256(token) 해시, revoked_at, expires_at)를 반드시 대조한다.
func (s *V2AuthService) refreshWebIssuedToken(refreshToken, mbID string) (*v2domain.V2User, error) {
	if mbID == "" || s.db == nil {
		return nil, common.ErrUnauthorized
	}

	sum := sha256.Sum256([]byte(refreshToken))
	tokenHash := hex.EncodeToString(sum[:])

	// 저장소 대조: 미폐기·미만료 + sub 일치. 만료 비교는 UTC_TIMESTAMP() 로 한다 —
	// 컬럼은 UTC 로 저장되는데 DSN loc=Asia/Seoul 이라 time.Now() 를 바인딩하면 드라이버가
	// KST 벽시계로 보내 저장값과 9시간 어긋난다(토큰이 9시간 일찍 만료 판정). DB 자체 UTC 로 비교.
	var row struct {
		MbID string
	}
	err := s.db.Raw(
		`SELECT mb_id FROM angple_refresh_tokens
		 WHERE token_hash = ? AND revoked_at IS NULL AND expires_at > UTC_TIMESTAMP()
		 LIMIT 1`, tokenHash,
	).Scan(&row).Error
	if err != nil {
		log.Printf("[WARN] 웹 발급 refresh 토큰 저장소 조회 실패: %v", err)
		return nil, common.ErrUnauthorized
	}
	// 미등록·폐기·만료 토큰은 거부. sub 위조 대비로 저장소의 mb_id 와 일치도 요구한다.
	if row.MbID == "" || row.MbID != mbID {
		return nil, common.ErrUnauthorized
	}

	// 단일 사용: 소비 즉시 폐기한다(원자적 claim). revoked_at IS NULL 조건이 동시 재생·재사용을
	// 한 번만 통과시킨다(RowsAffected 로 판정). 폐기하지 않으면 이 토큰이 만료까지 재생 가능하고,
	// 첫 갱신 후 쿠키가 Go 네이티브 토큰(저장소 밖)으로 교체돼 로그아웃(웹 저장소 폐기)이 이
	// 토큰에 닿지 못한다 — 로그아웃한 세션의 refresh 토큰이 7일간 살아있는 회귀가 된다.
	res := s.db.Exec(
		`UPDATE angple_refresh_tokens SET revoked_at = UTC_TIMESTAMP()
		 WHERE token_hash = ? AND revoked_at IS NULL`, tokenHash,
	)
	if res.Error != nil {
		log.Printf("[WARN] 웹 발급 refresh 토큰 폐기 실패: %v", res.Error)
		return nil, common.ErrUnauthorized
	}
	if res.RowsAffected == 0 {
		// 동시 요청이 먼저 소비(재사용 탐지) — 거부.
		return nil, common.ErrUnauthorized
	}

	user, err := s.userRepo.FindByUsername(mbID)
	if err != nil {
		return nil, common.ErrUnauthorized
	}
	return user, nil
}

// RefreshToken validates a refresh token and issues new token pair
func (s *V2AuthService) RefreshToken(refreshToken string) (*V2LoginResponse, error) {
	claims, err := s.jwtManager.VerifyToken(refreshToken)
	if err != nil {
		return nil, common.ErrUnauthorized
	}

	var user *v2domain.V2User
	if userID, parseErr := strconv.ParseUint(claims.UserID, 10, 64); parseErr == nil {
		user, err = s.userRepo.FindByID(userID)
		if err != nil {
			return nil, common.ErrUnauthorized
		}
	} else {
		// 웹(damoang.net) 로그인이 발급한 refresh 토큰은 user_id 없이 sub=<g5 mb_id> 만 담는다
		// (shared JWT_SECRET, AppExchangeLogin 과 같은 신뢰 도메인). 이 규격을 받아주지 않으면
		// 웹 세션은 자동 갱신이 영구 실패한다.
		user, err = s.refreshWebIssuedToken(refreshToken, claims.Subject)
		if err != nil {
			return nil, err
		}
	}

	// 탈퇴 게이트(세션 유지 경로): 확정 계정은 refresh 로도 무한 우회할 수 없도록 차단한다.
	state, deadline := s.checkWithdrawal(user.Username, time.Now())
	if state == common.WithdrawalConfirmed {
		return nil, common.ErrAccountWithdrawn
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

	resp := &V2LoginResponse{
		User:         user,
		AccessToken:  newAccess,
		RefreshToken: newRefresh,
	}
	// 숙려중이면 정상 세션 갱신이 아니라 취소 가능 상태를 표시한다(핸들러가 쿠키 미설정 + 상태 반환).
	if state == common.WithdrawalGrace {
		days := int(time.Until(deadline).Hours() / 24)
		if days < 0 {
			days = 0
		}
		resp.WithdrawalGrace = &WithdrawalGraceInfo{
			LeaveDate:     deadline.AddDate(0, 0, -common.WithdrawalGraceDays).Format("20060102"),
			Deadline:      deadline.Format("2006-01-02"),
			DaysRemaining: days,
		}
	}
	return resp, nil
}

func (s *V2AuthService) GetCurrentUser(userID uint64) (*v2domain.V2User, error) {
	return s.userRepo.FindByID(userID)
}

// CheckWithdrawalStatus 는 username(mb_id) 기준 탈퇴 상태와 숙려 만료 시각을 반환한다(SSO /auth/me 분기용).
func (s *V2AuthService) CheckWithdrawalStatus(username string, now time.Time) (common.WithdrawalState, time.Time) {
	return s.checkWithdrawal(username, now)
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
