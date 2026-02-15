package middleware

import (
	"fmt"
	"sync"

	"gorm.io/gorm"
)

// TenantDBResolver resolves the correct database connection for a tenant
// based on the site's DB strategy (shared, schema, dedicated)
type TenantDBResolver struct {
	defaultDB *gorm.DB
	schemaDbs sync.Map // map[string]*gorm.DB — per-schema connections
}

// NewTenantDBResolver creates a new resolver
func NewTenantDBResolver(defaultDB *gorm.DB) *TenantDBResolver {
	return &TenantDBResolver{
		defaultDB: defaultDB,
	}
}

// ResolveDB returns the appropriate *gorm.DB for a tenant
func (r *TenantDBResolver) ResolveDB(_, dbStrategy, schemaName string) *gorm.DB {
	switch dbStrategy {
	case "schema":
		return r.resolveSchema(schemaName)
	case "dedicated":
		// dedicated DB 연결은 별도 설정 필요 (추후)
		return r.defaultDB
	default: // "shared"
		return r.defaultDB
	}
}

// resolveSchema returns a session scoped to the given schema
func (r *TenantDBResolver) resolveSchema(schemaName string) *gorm.DB {
	if schemaName == "" {
		return r.defaultDB
	}

	// 캐시된 세션 확인
	if db, ok := r.schemaDbs.Load(schemaName); ok {
		if gormDB, assertOK := db.(*gorm.DB); assertOK {
			return gormDB
		}
	}

	// 새 세션 생성 (USE schema_name)
	db := r.defaultDB.Session(&gorm.Session{})
	if err := db.Exec(fmt.Sprintf("USE `%s`", schemaName)).Error; err != nil {
		return r.defaultDB
	}
	r.schemaDbs.Store(schemaName, db)
	return db
}

// CreateSchema creates a new database schema for a tenant
func (r *TenantDBResolver) CreateSchema(schemaName string) error {
	sql := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", schemaName)
	return r.defaultDB.Exec(sql).Error
}

// DropSchema drops a tenant's database schema
func (r *TenantDBResolver) DropSchema(schemaName string) error {
	sql := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", schemaName)
	r.schemaDbs.Delete(schemaName)
	return r.defaultDB.Exec(sql).Error
}

// PlanLimits defines resource limits per plan
type PlanLimits struct {
	MaxStorage     int64 `json:"max_storage_mb"`   // MB
	MaxBandwidth   int64 `json:"max_bandwidth_mb"` // MB/month
	MaxPosts       int   `json:"max_posts"`
	MaxBoards      int   `json:"max_boards"`
	MaxUsers       int   `json:"max_users"`
	MaxAPICalls    int   `json:"max_api_calls_per_day"`
	MaxFileSize    int64 `json:"max_file_size_mb"` // MB per file
	CustomDomain   bool  `json:"custom_domain"`
	SSLEnabled     bool  `json:"ssl_enabled"`
	PluginsAllowed bool  `json:"plugins_allowed"`
	MaxPlugins     int   `json:"max_plugins"`
}

// GetPlanLimits returns resource limits for a given plan
func GetPlanLimits(plan string) PlanLimits {
	switch plan {
	case "free":
		return PlanLimits{
			MaxStorage:     500,
			MaxBandwidth:   5000,
			MaxPosts:       1000,
			MaxBoards:      5,
			MaxUsers:       100,
			MaxAPICalls:    10000,
			MaxFileSize:    5,
			CustomDomain:   false,
			SSLEnabled:     true,
			PluginsAllowed: false,
			MaxPlugins:     0,
		}
	case "pro":
		return PlanLimits{
			MaxStorage:     5000,
			MaxBandwidth:   50000,
			MaxPosts:       50000,
			MaxBoards:      20,
			MaxUsers:       1000,
			MaxAPICalls:    100000,
			MaxFileSize:    20,
			CustomDomain:   true,
			SSLEnabled:     true,
			PluginsAllowed: true,
			MaxPlugins:     5,
		}
	case "business":
		return PlanLimits{
			MaxStorage:     50000,
			MaxBandwidth:   500000,
			MaxPosts:       500000,
			MaxBoards:      100,
			MaxUsers:       10000,
			MaxAPICalls:    1000000,
			MaxFileSize:    50,
			CustomDomain:   true,
			SSLEnabled:     true,
			PluginsAllowed: true,
			MaxPlugins:     20,
		}
	case "enterprise":
		return PlanLimits{
			MaxStorage:     -1, // unlimited
			MaxBandwidth:   -1,
			MaxPosts:       -1,
			MaxBoards:      -1,
			MaxUsers:       -1,
			MaxAPICalls:    -1,
			MaxFileSize:    100,
			CustomDomain:   true,
			SSLEnabled:     true,
			PluginsAllowed: true,
			MaxPlugins:     -1,
		}
	default:
		return GetPlanLimits("free")
	}
}
