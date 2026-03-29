package v2

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

// xpConfigCache caches XPConfig to avoid hitting site_settings on every write/comment
var (
	xpConfigCacheMu    sync.RWMutex
	xpConfigCacheVal   *XPConfig
	xpConfigCacheExpAt time.Time
)

const xpConfigCacheTTL = 30 * time.Second

// ExpSummary represents experience point summary statistics
type ExpSummary struct {
	TotalExp     int `json:"total_exp"`
	CurrentLevel int `json:"current_level"`
	NextLevel    int `json:"next_level"`
	NextLevelExp int `json:"next_level_exp"`
	ExpToNext    int `json:"exp_to_next"`
	Progress     int `json:"level_progress"` // percentage 0-100
}

// XPConfig represents configurable XP settings (stored in site_settings.settings_json)
type XPConfig struct {
	LoginXP        int  `json:"login_xp"`        // XP granted per daily login (default: 500)
	WriteXP        int  `json:"write_xp"`        // XP granted per post (default: 100)
	CommentXP      int  `json:"comment_xp"`      // XP granted per comment (default: 50)
	LoginEnabled   bool `json:"login_enabled"`   // Enable login XP (default: true)
	WriteEnabled   bool `json:"write_enabled"`   // Enable write XP (default: true)
	CommentEnabled bool `json:"comment_enabled"` // Enable comment XP (default: true)
	XPBasePoint    int  `json:"xp_base_point"`   // Base XP per level step (default: 1000, nariya xp_point)
	XPRate         int  `json:"xp_rate"`         // Level growth rate (default: 2, nariya xp_rate)
	MaxLevel       int  `json:"max_level"`       // Maximum level cap (default: 5000, nariya xp_max)
}

// DefaultXPConfig returns the default XP configuration
func DefaultXPConfig() *XPConfig {
	return &XPConfig{
		LoginXP:        500,
		WriteXP:        100,
		CommentXP:      50,
		LoginEnabled:   true,
		WriteEnabled:   true,
		CommentEnabled: true,
		XPBasePoint:    1000,
		XPRate:         2,
		MaxLevel:       5000,
	}
}

// MemberXPInfo represents a member's XP summary for admin listing
type MemberXPInfo struct {
	MbID    string `json:"mb_id"`
	MbNick  string `json:"mb_nick"`
	AsExp   int    `json:"as_exp"`
	AsLevel int    `json:"as_level"`
	MbLevel int    `json:"mb_level"`
}

// AddExpResult contains the result of an AddExp operation
type AddExpResult struct {
	LevelUp  bool `json:"level_up"`
	OldLevel int  `json:"old_level"`
	NewLevel int  `json:"new_level"`
}

// ExpRepository handles experience point data access
type ExpRepository interface {
	// GetSummary returns exp summary for a user (by mb_id)
	GetSummary(mbID string) (*ExpSummary, error)
	// GetHistory returns exp history with pagination
	GetHistory(mbID string, page, limit int) ([]gnuboard.ExpHistory, int64, error)
	// AddExp adds experience points to a user and returns level change info
	AddExp(mbID string, point int, content, relTable, relID, action string) (*AddExpResult, error)
	// HasTodayAction checks if the user already has a specific action logged today
	HasTodayAction(mbID, action string) (bool, error)
	// ListMembersWithXP returns paginated member list with XP info for admin
	ListMembersWithXP(search string, page, limit int) ([]MemberXPInfo, int64, error)
	// GetXPConfig returns the current XP configuration
	GetXPConfig() (*XPConfig, error)
	// UpdateXPConfig updates the XP configuration
	UpdateXPConfig(config *XPConfig) error
	// IncrementLoginDays increments mb_login_days by 1
	IncrementLoginDays(mbID string) error
}

type expRepository struct {
	db *gorm.DB
}

// NewExpRepository creates a new ExpRepository
func NewExpRepository(db *gorm.DB) ExpRepository {
	return &expRepository{db: db}
}

// maxLevel is the highest level a user can reach (nariya xp_max)
const maxLevel = 5000

// levelExp returns the cumulative exp required for a given level (1-based).
// Nariya-compatible formula: xp_base * (n-1)² where xp_base=1000, xp_rate=2
//
//	Level 1: 0, Level 2: 1000, Level 3: 4000, Level 10: 81000, Level 40: 1521000
func levelExp(level int) int {
	if level <= 1 {
		return 0
	}
	n := level - 1
	return 1000 * n * n
}

func calculateLevelInfo(totalExp int) (currentLevel, nextLevel, nextLevelExp, expToNext, progress int) {
	// Binary search for current level: find highest level where levelExp(level) <= totalExp
	lo, hi := 1, maxLevel
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if levelExp(mid) <= totalExp {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	currentLevel = lo

	// Calculate next level info
	if currentLevel >= maxLevel {
		// Max level reached
		nextLevel = currentLevel
		nextLevelExp = levelExp(currentLevel)
		expToNext = 0
		progress = 100
	} else {
		nextLevel = currentLevel + 1
		nextLevelExp = levelExp(nextLevel)
		prevLevelExp := levelExp(currentLevel)
		expToNext = nextLevelExp - totalExp
		levelRange := nextLevelExp - prevLevelExp
		if levelRange > 0 {
			progress = (totalExp - prevLevelExp) * 100 / levelRange
		}
	}

	return
}

func (r *expRepository) GetSummary(mbID string) (*ExpSummary, error) {
	// Get current exp and level from member
	var member gnuboard.G5Member
	if err := r.db.Select("as_exp, as_level").Where("mb_id = ?", mbID).First(&member).Error; err != nil {
		return nil, err
	}

	totalExp := member.AsExp
	currentLevel, nextLevel, nextLevelExp, expToNext, progress := calculateLevelInfo(totalExp)

	return &ExpSummary{
		TotalExp:     totalExp,
		CurrentLevel: currentLevel,
		NextLevel:    nextLevel,
		NextLevelExp: nextLevelExp,
		ExpToNext:    expToNext,
		Progress:     progress,
	}, nil
}

func (r *expRepository) GetHistory(mbID string, page, limit int) ([]gnuboard.ExpHistory, int64, error) {
	// Count total
	var total int64
	if err := r.db.Model(&gnuboard.G5NaXP{}).Where("mb_id = ?", mbID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	var xpLogs []gnuboard.G5NaXP
	if err := r.db.Where("mb_id = ?", mbID).
		Order("xp_datetime DESC").
		Offset(offset).
		Limit(limit).
		Find(&xpLogs).Error; err != nil {
		return nil, 0, err
	}

	// Convert to ExpHistory
	history := make([]gnuboard.ExpHistory, len(xpLogs))
	for i, xp := range xpLogs {
		history[i] = xp.ToExpHistory()
	}

	return history, total, nil
}

// maxXPLevel is the highest XP level; users at this level stop earning XP from actions
var maxXPLevel = maxLevel

func (r *expRepository) AddExp(mbID string, point int, content, relTable, relID, action string) (*AddExpResult, error) {
	result := &AddExpResult{}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Get current level before update
		var member gnuboard.G5Member
		if err := tx.Select("as_exp, as_level").Where("mb_id = ?", mbID).First(&member).Error; err != nil {
			return err
		}
		result.OldLevel = member.AsLevel
		result.NewLevel = member.AsLevel

		// 최대 레벨 도달 시 자동 적립(양수) 차단 — 관리자 수동 지급/차감은 허용
		if member.AsLevel >= maxXPLevel && point > 0 && relTable != "@admin" {
			return nil // 적립 없이 조용히 반환
		}

		// 레벨 80 이상: 로그인(출석)으로만 XP 적립 가능
		if member.AsLevel >= 80 && point > 0 && relTable != "@login" && relTable != "@admin" {
			return nil
		}

		// Update member exp
		if err := tx.Model(&gnuboard.G5Member{}).
			Where("mb_id = ?", mbID).
			UpdateColumn("as_exp", gorm.Expr("as_exp + ?", point)).Error; err != nil {
			return err
		}

		// Check if level up is needed
		newExp := member.AsExp + point
		newLevel, _, _, _, _ := calculateLevelInfo(newExp)
		result.NewLevel = newLevel
		if newLevel > member.AsLevel {
			result.LevelUp = true
			if err := tx.Model(&gnuboard.G5Member{}).
				Where("mb_id = ?", mbID).
				UpdateColumn("as_level", newLevel).Error; err != nil {
				return err
			}
		}

		// Insert exp log
		log := &gnuboard.G5NaXP{
			MbID:        mbID,
			XpPoint:     point,
			XpContent:   content,
			XpRelTable:  relTable,
			XpRelID:     relID,
			XpRelAction: action,
		}
		return tx.Create(log).Error
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// HasTodayAction checks if the user already has a specific action logged today
// Uses range query on xp_datetime to leverage idx_mb_action_date index
func (r *expRepository) HasTodayAction(mbID, action string) (bool, error) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	var count int64
	err := r.db.Model(&gnuboard.G5NaXP{}).
		Where("mb_id = ? AND xp_rel_table = ? AND xp_rel_action = ? AND xp_datetime >= ? AND xp_datetime < ?",
			mbID, "@login", action, todayStart, tomorrowStart).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListMembersWithXP returns paginated member list with XP info for admin
func (r *expRepository) ListMembersWithXP(search string, page, limit int) ([]MemberXPInfo, int64, error) {
	query := r.db.Model(&gnuboard.G5Member{}).
		Select("mb_id, mb_nick, as_exp, as_level, mb_level")

	if search != "" {
		query = query.Where("mb_id LIKE ? OR mb_nick LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	var members []MemberXPInfo
	if err := query.Order("as_exp DESC").Offset(offset).Limit(limit).Find(&members).Error; err != nil {
		return nil, 0, err
	}

	return members, total, nil
}

// IncrementLoginDays increments mb_login_days by 1
func (r *expRepository) IncrementLoginDays(mbID string) error {
	return r.db.Model(&gnuboard.G5Member{}).
		Where("mb_id = ?", mbID).
		UpdateColumn("mb_login_days", gorm.Expr("mb_login_days + 1")).Error
}

const defaultSiteID = "default"

// siteSettingsJSON is a helper struct to read/write settings_json from site_settings
type siteSettingsJSON struct {
	SettingsJSON *string `gorm:"column:settings_json"`
}

func (siteSettingsJSON) TableName() string {
	return "site_settings"
}

// PointConfig represents configurable point expiry settings (stored in site_settings.settings_json)
type PointConfig struct {
	ExpiryEnabled bool `json:"expiry_enabled"` // Enable point expiry (default: false)
	ExpiryDays    int  `json:"expiry_days"`    // Days until points expire (default: 180)
}

// DefaultPointConfig returns the default point configuration
func DefaultPointConfig() *PointConfig {
	return &PointConfig{
		ExpiryEnabled: false,
		ExpiryDays:    180,
	}
}

// settingsJSONWrapper wraps the full settings_json content (preserves unknown fields)
type settingsJSONWrapper struct {
	XPConfig    *XPConfig              `json:"xp_config,omitempty"`
	PointConfig *PointConfig           `json:"point_config,omitempty"`
	Extra       map[string]interface{} `json:"-"`
}

// GetXPConfig reads XP configuration from site_settings.settings_json (cached 30s)
func (r *expRepository) GetXPConfig() (*XPConfig, error) {
	// Check cache first
	xpConfigCacheMu.RLock()
	if xpConfigCacheVal != nil && time.Now().Before(xpConfigCacheExpAt) {
		cached := *xpConfigCacheVal // copy
		xpConfigCacheMu.RUnlock()
		return &cached, nil
	}
	xpConfigCacheMu.RUnlock()

	config, err := r.getXPConfigFromDB()
	if err != nil {
		return nil, err
	}

	// Update cache
	xpConfigCacheMu.Lock()
	cp := *config
	xpConfigCacheVal = &cp
	xpConfigCacheExpAt = time.Now().Add(xpConfigCacheTTL)
	xpConfigCacheMu.Unlock()

	return config, nil
}

// getXPConfigFromDB fetches XPConfig directly from database
func (r *expRepository) getXPConfigFromDB() (*XPConfig, error) {
	var row siteSettingsJSON
	err := r.db.Select("settings_json").Where("site_id = ?", defaultSiteID).First(&row).Error
	if err != nil {
		// No row exists — return defaults
		return DefaultXPConfig(), nil
	}

	if row.SettingsJSON == nil || *row.SettingsJSON == "" || *row.SettingsJSON == "null" {
		return DefaultXPConfig(), nil
	}

	var wrapper settingsJSONWrapper
	if err := json.Unmarshal([]byte(*row.SettingsJSON), &wrapper); err != nil {
		return DefaultXPConfig(), nil
	}

	if wrapper.XPConfig == nil {
		return DefaultXPConfig(), nil
	}

	// Fill defaults for zero values on legacy configs (only LoginXP had a value before)
	if wrapper.XPConfig.LoginXP == 0 {
		wrapper.XPConfig.LoginXP = 500
	}
	if wrapper.XPConfig.WriteXP == 0 {
		wrapper.XPConfig.WriteXP = 100
	}
	if wrapper.XPConfig.CommentXP == 0 {
		wrapper.XPConfig.CommentXP = 50
	}
	if wrapper.XPConfig.XPBasePoint == 0 {
		wrapper.XPConfig.XPBasePoint = 1000
	}
	if wrapper.XPConfig.XPRate == 0 {
		wrapper.XPConfig.XPRate = 2
	}
	if wrapper.XPConfig.MaxLevel == 0 {
		wrapper.XPConfig.MaxLevel = 5000
	}

	return wrapper.XPConfig, nil
}

// UpdateXPConfig writes XP configuration to site_settings.settings_json (preserving other fields)
// Invalidates the XPConfig cache on success
func (r *expRepository) UpdateXPConfig(config *XPConfig) error {
	// Invalidate cache before update (will be repopulated on next read)
	defer func() {
		xpConfigCacheMu.Lock()
		xpConfigCacheVal = nil
		xpConfigCacheMu.Unlock()
	}()

	var row siteSettingsJSON
	err := r.db.Select("settings_json").Where("site_id = ?", defaultSiteID).First(&row).Error

	// Parse existing JSON to preserve other fields
	existing := make(map[string]interface{})
	if err == nil && row.SettingsJSON != nil && *row.SettingsJSON != "" && *row.SettingsJSON != "null" {
		if unmarshalErr := json.Unmarshal([]byte(*row.SettingsJSON), &existing); unmarshalErr != nil {
			existing = make(map[string]interface{})
		}
	}

	existing["xp_config"] = config
	jsonBytes, marshalErr := json.Marshal(existing)
	if marshalErr != nil {
		return marshalErr
	}
	jsonStr := string(jsonBytes)

	if err != nil {
		// Row doesn't exist — create it
		return r.db.Exec(
			"INSERT INTO site_settings (site_id, settings_json, active_theme) VALUES (?, ?, 'damoang-official')",
			defaultSiteID, jsonStr,
		).Error
	}

	// Update existing row
	return r.db.Table("site_settings").
		Where("site_id = ?", defaultSiteID).
		UpdateColumn("settings_json", jsonStr).Error
}
