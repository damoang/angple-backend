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

	// 메뉴 정의 (선택) - 플러그인이 Admin UI에 메뉴 등록
	Menus []MenuConfig `yaml:"menus"`
}

// MenuConfig 플러그인 메뉴 설정
type MenuConfig struct {
	Title         string `yaml:"title"`           // 메뉴 제목
	URL           string `yaml:"url"`             // 메뉴 URL
	Icon          string `yaml:"icon"`            // Lucide 아이콘 이름
	ParentPath    string `yaml:"parent_path"`     // 부모 메뉴 URL (없으면 루트)
	ShowInSidebar bool   `yaml:"show_in_sidebar"` // 사이드바 노출 여부
	ShowInHeader  bool   `yaml:"show_in_header"`  // 헤더 노출 여부
	OrderNum      int    `yaml:"order_num"`       // 정렬 순서
	ViewLevel     int    `yaml:"view_level"`      // 보기 권한 레벨 (기본 1)
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
	Manifest   *PluginManifest
	Path       string
	Status     PluginStatus
	Error      error
	Instance   Plugin
	IsBuiltIn  bool // 내장 플러그인 여부
	LoadedAt   int64
	MigratedAt int64
}

// Plugin 플러그인 인터페이스 - 모든 플러그인이 구현해야 함
type Plugin interface {
	// Name 플러그인 이름 반환
	Name() string

	// Migrate DB 마이그레이션 실행 (테이블 생성/업데이트)
	Migrate(db *gorm.DB) error

	// Initialize 플러그인 초기화
	Initialize(ctx *PluginContext) error

	// RegisterRoutes 라우트 등록
	RegisterRoutes(router gin.IRouter)

	// Shutdown 플러그인 종료
	Shutdown() error
}

// PluginContext 플러그인에 전달되는 컨텍스트
type PluginContext struct {
	DB         *gorm.DB
	Redis      *redis.Client
	Config     map[string]interface{}
	Logger     Logger
	BasePath   string
	JWTManager interface{} // JWT 매니저 (순환 의존 방지를 위해 interface{} 사용)
}

// Logger 플러그인용 로거 인터페이스
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// SettingGetter 플러그인 설정 조회 인터페이스 (순환 의존 방지)
type SettingGetter interface {
	GetSettingsAsMap(pluginName string) (map[string]interface{}, error)
}

// PermissionSyncer 플러그인 권한 동기화 인터페이스 (순환 의존 방지)
type PermissionSyncer interface {
	SyncPermissions(pluginName string, permissions []Permission) error
	DeletePermissions(pluginName string) error
	CheckPermission(pluginName, permissionID string, userLevel int) (bool, error)
}

// PluginReloader 설정 변경 시 플러그인 재초기화를 위한 인터페이스
type PluginReloader interface {
	ReloadPlugin(name string) error
}

// HealthCheckable 선택적 인터페이스 - 플러그인 상태 점검
type HealthCheckable interface {
	HealthCheck() error
}

// PluginHealth 플러그인 헬스 체크 결과
type PluginHealth struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // healthy, unhealthy, disabled
	Message string `json:"message,omitempty"`
}

// HookAware 선택적 인터페이스 - Hook을 등록하고 싶은 플러그인이 구현
type HookAware interface {
	RegisterHooks(hm *HookManager)
}

// Schedulable 선택적 인터페이스 - 주기적 작업을 등록하고 싶은 플러그인이 구현
type Schedulable interface {
	RegisterSchedules(scheduler *Scheduler)
}

// RateLimitable 선택적 인터페이스 - API 레이트 리밋을 설정하고 싶은 플러그인이 구현
type RateLimitable interface {
	ConfigureRateLimit(limiter *RateLimiter)
}

// EventAware 선택적 인터페이스 - 이벤트 버스 구독
type EventAware interface {
	RegisterEvents(bus *EventBus)
}

// LifecycleAware 선택적 인터페이스 - 설치/제거/활성화/비활성화 이벤트 수신
type LifecycleAware interface {
	OnInstall() error
	OnUninstall() error
	OnEnable() error
	OnDisable() error
}
