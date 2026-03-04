package v2

import (
	"encoding/json"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

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
	LoginXP int `json:"login_xp"` // XP granted per daily login (default: 500)
}

// DefaultXPConfig returns the default XP configuration
func DefaultXPConfig() *XPConfig {
	return &XPConfig{LoginXP: 500}
}

// MemberXPInfo represents a member's XP summary for admin listing
type MemberXPInfo struct {
	MbID    string `json:"mb_id"`
	MbNick  string `json:"mb_nick"`
	AsExp   int    `json:"as_exp"`
	AsLevel int    `json:"as_level"`
	MbLevel int    `json:"mb_level"`
}

// ExpRepository handles experience point data access
type ExpRepository interface {
	// GetSummary returns exp summary for a user (by mb_id)
	GetSummary(mbID string) (*ExpSummary, error)
	// GetHistory returns exp history with pagination
	GetHistory(mbID string, page, limit int) ([]gnuboard.ExpHistory, int64, error)
	// AddExp adds experience points to a user
	AddExp(mbID string, point int, content, relTable, relID, action string) error
	// HasTodayAction checks if the user already has a specific action logged today
	HasTodayAction(mbID, action string) (bool, error)
	// ListMembersWithXP returns paginated member list with XP info for admin
	ListMembersWithXP(search string, page, limit int) ([]MemberXPInfo, int64, error)
	// GetXPConfig returns the current XP configuration
	GetXPConfig() (*XPConfig, error)
	// UpdateXPConfig updates the XP configuration
	UpdateXPConfig(config *XPConfig) error
}

type expRepository struct {
	db *gorm.DB
}

// NewExpRepository creates a new ExpRepository
func NewExpRepository(db *gorm.DB) ExpRepository {
	return &expRepository{db: db}
}

// Level thresholds (cumulative exp required for each level)
var levelThresholds = []int{
	0,      // Level 1
	1000,   // Level 2
	3000,   // Level 3
	6000,   // Level 4
	10000,  // Level 5
	15000,  // Level 6
	21000,  // Level 7
	28000,  // Level 8
	36000,  // Level 9
	45000,  // Level 10
	55000,  // Level 11
	66000,  // Level 12
	78000,  // Level 13
	91000,  // Level 14
	105000, // Level 15
}

func calculateLevelInfo(totalExp int) (currentLevel, nextLevel, nextLevelExp, expToNext, progress int) {
	currentLevel = 1
	for i, threshold := range levelThresholds {
		if totalExp >= threshold {
			currentLevel = i + 1
		} else {
			break
		}
	}

	// Calculate next level info
	if currentLevel >= len(levelThresholds) {
		// Max level reached
		nextLevel = currentLevel
		nextLevelExp = levelThresholds[len(levelThresholds)-1]
		expToNext = 0
		progress = 100
	} else {
		nextLevel = currentLevel + 1
		nextLevelExp = levelThresholds[currentLevel]
		prevLevelExp := 0
		if currentLevel > 1 {
			prevLevelExp = levelThresholds[currentLevel-1]
		}
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

func (r *expRepository) AddExp(mbID string, point int, content, relTable, relID, action string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Update member exp
		if err := tx.Model(&gnuboard.G5Member{}).
			Where("mb_id = ?", mbID).
			UpdateColumn("as_exp", gorm.Expr("as_exp + ?", point)).Error; err != nil {
			return err
		}

		// Get updated exp to check level
		var member gnuboard.G5Member
		if err := tx.Select("as_exp, as_level").Where("mb_id = ?", mbID).First(&member).Error; err != nil {
			return err
		}

		// Check if level up is needed
		newLevel, _, _, _, _ := calculateLevelInfo(member.AsExp)
		if newLevel > member.AsLevel {
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
}

// HasTodayAction checks if the user already has a specific action logged today
func (r *expRepository) HasTodayAction(mbID, action string) (bool, error) {
	today := time.Now().Format("2006-01-02")
	var count int64
	err := r.db.Model(&gnuboard.G5NaXP{}).
		Where("mb_id = ? AND xp_rel_action = ? AND DATE(xp_datetime) = ?", mbID, action, today).
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

const defaultSiteID = "default"

// siteSettingsJSON is a helper struct to read/write settings_json from site_settings
type siteSettingsJSON struct {
	SettingsJSON *string `gorm:"column:settings_json"`
}

func (siteSettingsJSON) TableName() string {
	return "site_settings"
}

// settingsJSONWrapper wraps the full settings_json content (preserves unknown fields)
type settingsJSONWrapper struct {
	XPConfig *XPConfig              `json:"xp_config,omitempty"`
	Extra    map[string]interface{} `json:"-"`
}

// GetXPConfig reads XP configuration from site_settings.settings_json
func (r *expRepository) GetXPConfig() (*XPConfig, error) {
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

	// Validate: if login_xp is 0, return default
	if wrapper.XPConfig.LoginXP == 0 {
		wrapper.XPConfig.LoginXP = 500
	}

	return wrapper.XPConfig, nil
}

// UpdateXPConfig writes XP configuration to site_settings.settings_json (preserving other fields)
func (r *expRepository) UpdateXPConfig(config *XPConfig) error {
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
