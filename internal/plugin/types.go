package plugin

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// PluginManifest plugin.yaml 스키마
type PluginManifest struct {
	// 기본 정보 (필수)
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Author      string `yaml:"author"`
	License     string `yaml:"license"`
	Homepage    string `yaml:"homepage"`

	// 호환성 (필수)
	Requires Requires `yaml:"requires"`

	// 충돌 플러그인 (선택)
	Conflicts []string `yaml:"conflicts"`

	// DB 마이그레이션 (선택)
	Migrations []Migration `yaml:"migrations"`

	// Hook 등록 (선택)
	Hooks []HookRegistration `yaml:"hooks"`

	// API 라우트 (선택)
	Routes []RouteConfig `yaml:"routes"`

	// 설정 스키마 (선택)
	Settings []SettingConfig `yaml:"settings"`

	// 권한 정의 (선택)
	Permissions []Permission `yaml:"permissions"`
}

// Requires 호환성 요구사항
type Requires struct {
	Angple  string             `yaml:"angple"`
	Go      string             `yaml:"go"`
	Plugins []PluginDependency `yaml:"plugins"`
}

// PluginDependency 의존 플러그인
type PluginDependency struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// Migration DB 마이그레이션 정보
type Migration struct {
	File    string `yaml:"file"`
	Version int    `yaml:"version"`
}

// HookRegistration Hook 등록 정보
type HookRegistration struct {
	Event    string `yaml:"event"`
	Handler  string `yaml:"handler"`
	Priority int    `yaml:"priority"`
}

// RouteConfig API 라우트 설정
type RouteConfig struct {
	Path    string `yaml:"path"`
	Method  string `yaml:"method"`
	Handler string `yaml:"handler"`
	Auth    string `yaml:"auth"` // required | optional | none
}

// SettingConfig 설정 스키마
type SettingConfig struct {
	Key     string          `yaml:"key"`
	Type    string          `yaml:"type"`
	Default interface{}     `yaml:"default"`
	Label   string          `yaml:"label"`
	Min     *int            `yaml:"min,omitempty"`
	Max     *int            `yaml:"max,omitempty"`
	Options []SettingOption `yaml:"options,omitempty"`
}

// SettingOption 선택 옵션
type SettingOption struct {
	Value string `yaml:"value"`
	Label string `yaml:"label"`
}

// Permission 권한 정의
type Permission struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
}

// PluginStatus 플러그인 상태
type PluginStatus string

const (
	StatusDisabled PluginStatus = "disabled"
	StatusEnabled  PluginStatus = "enabled"
	StatusError    PluginStatus = "error"
)

// PluginInfo 로드된 플러그인 정보
type PluginInfo struct {
	Manifest    *PluginManifest
	Path        string
	Status      PluginStatus
	Error       error
	Instance    Plugin
	IsBuiltIn   bool // 내장 플러그인 여부
	LoadedAt    int64
	MigratedAt  int64
}

// Plugin 플러그인 인터페이스 - 모든 플러그인이 구현해야 함
type Plugin interface {
	// Name 플러그인 이름 반환
	Name() string

	// Initialize 플러그인 초기화
	Initialize(ctx *PluginContext) error

	// RegisterRoutes 라우트 등록
	RegisterRoutes(router gin.IRouter)

	// Shutdown 플러그인 종료
	Shutdown() error
}

// PluginContext 플러그인에 전달되는 컨텍스트
type PluginContext struct {
	DB       *gorm.DB
	Redis    *redis.Client
	Config   map[string]interface{}
	Logger   Logger
	BasePath string
}

// Logger 플러그인용 로거 인터페이스
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}
