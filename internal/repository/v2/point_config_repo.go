package v2

import (
	"encoding/json"
	"sync"
	"time"

	"gorm.io/gorm"
)

// pointConfigCache caches PointConfig to avoid hitting site_settings on every point operation
var (
	pointConfigCacheMu    sync.RWMutex
	pointConfigCacheVal   *PointConfig
	pointConfigCacheExpAt time.Time
)

const pointConfigCacheTTL = 30 * time.Second

// PointConfigRepository handles point configuration data access
type PointConfigRepository interface {
	// GetPointConfig returns the current point configuration (cached 30s)
	GetPointConfig() (*PointConfig, error)
	// UpdatePointConfig updates the point configuration
	UpdatePointConfig(config *PointConfig) error
}

type pointConfigRepository struct {
	db *gorm.DB
}

// NewPointConfigRepository creates a new PointConfigRepository
func NewPointConfigRepository(db *gorm.DB) PointConfigRepository {
	return &pointConfigRepository{db: db}
}

// GetPointConfig reads point configuration from site_settings.settings_json (cached 30s)
func (r *pointConfigRepository) GetPointConfig() (*PointConfig, error) {
	// Check cache first
	pointConfigCacheMu.RLock()
	if pointConfigCacheVal != nil && time.Now().Before(pointConfigCacheExpAt) {
		cached := *pointConfigCacheVal
		pointConfigCacheMu.RUnlock()
		return &cached, nil
	}
	pointConfigCacheMu.RUnlock()

	config, err := r.getPointConfigFromDB()
	if err != nil {
		return nil, err
	}

	// Update cache
	pointConfigCacheMu.Lock()
	cp := *config
	pointConfigCacheVal = &cp
	pointConfigCacheExpAt = time.Now().Add(pointConfigCacheTTL)
	pointConfigCacheMu.Unlock()

	return config, nil
}

// getPointConfigFromDB fetches PointConfig directly from database
func (r *pointConfigRepository) getPointConfigFromDB() (*PointConfig, error) {
	var row siteSettingsJSON
	err := r.db.Select("settings_json").Where("site_id = ?", defaultSiteID).First(&row).Error
	if err != nil {
		return DefaultPointConfig(), nil
	}

	if row.SettingsJSON == nil || *row.SettingsJSON == "" || *row.SettingsJSON == "null" {
		return DefaultPointConfig(), nil
	}

	var wrapper settingsJSONWrapper
	if err := json.Unmarshal([]byte(*row.SettingsJSON), &wrapper); err != nil {
		return DefaultPointConfig(), nil
	}

	if wrapper.PointConfig == nil {
		return DefaultPointConfig(), nil
	}

	// Fill defaults for zero values
	if wrapper.PointConfig.ExpiryDays == 0 {
		wrapper.PointConfig.ExpiryDays = 180
	}

	return wrapper.PointConfig, nil
}

// UpdatePointConfig writes point configuration to site_settings.settings_json (preserving other fields)
func (r *pointConfigRepository) UpdatePointConfig(config *PointConfig) error {
	// Invalidate cache
	defer func() {
		pointConfigCacheMu.Lock()
		pointConfigCacheVal = nil
		pointConfigCacheMu.Unlock()
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

	existing["point_config"] = config
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
