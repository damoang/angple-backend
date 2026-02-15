package advertising

import (
	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/plugins/advertising/domain"
	"github.com/damoang/angple-backend/internal/plugins/advertising/handler"
	"github.com/damoang/angple-backend/internal/plugins/advertising/repository"
	"github.com/damoang/angple-backend/internal/plugins/advertising/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// init 플러그인 팩토리 자동 등록
func init() {
	plugin.RegisterFactory("advertising", func() plugin.Plugin {
		return New()
	}, Manifest)
}

// AdvertisingPlugin 광고 관리 플러그인
type AdvertisingPlugin struct { //nolint:revive // plugin identifier naming convention
	db            *gorm.DB
	redis         *redis.Client
	logger        plugin.Logger
	config        map[string]interface{}
	publicHandler *handler.PublicHandler
	adminHandler  *handler.AdminHandler
}

// Manifest 광고 플러그인 매니페스트
var Manifest = &plugin.PluginManifest{
	Name:        "advertising",
	Version:     "1.0.0",
	Title:       "Advertising",
	Description: "GAM, AdSense 광고 및 축하 배너 관리 플러그인",
	Author:      "Angple",
	License:     "Proprietary",
	Requires: plugin.Requires{
		Angple: ">=1.0.0",
	},
	// Admin 메뉴 정의
	Menus: []plugin.MenuConfig{
		{
			Title:         "광고 관리",
			URL:           "/admin/advertising",
			Icon:          "megaphone",
			ShowInSidebar: true,
			OrderNum:      80,
			ViewLevel:     9, // Admin 전용
		},
		{
			Title:         "광고 단위",
			URL:           "/admin/advertising/units",
			Icon:          "layout-grid",
			ParentPath:    "/admin/advertising",
			ShowInSidebar: true,
			OrderNum:      1,
			ViewLevel:     9,
		},
		{
			Title:         "축하 배너",
			URL:           "/admin/advertising/banners",
			Icon:          "party-popper",
			ParentPath:    "/admin/advertising",
			ShowInSidebar: true,
			OrderNum:      2,
			ViewLevel:     9,
		},
	},
	Settings: []plugin.SettingConfig{
		{Key: "gam_network_code", Type: "string", Default: "22996793498", Label: "GAM 네트워크 코드"},
		{Key: "adsense_client_id", Type: "string", Default: "ca-pub-5124617752473025", Label: "AdSense 클라이언트 ID"},
		{Key: "enable_gam", Type: "boolean", Default: true, Label: "GAM 활성화"},
		{Key: "enable_adsense_fallback", Type: "boolean", Default: true, Label: "AdSense Fallback 활성화"},
	},
	Routes: []plugin.RouteConfig{
		// Public API - GAM/AdSense
		{Path: "/gam/config", Method: "GET", Handler: "GetGAMConfig", Auth: "none"},
		{Path: "/adsense/config", Method: "GET", Handler: "GetAdsenseConfig", Auth: "none"},
		{Path: "/units/:position", Method: "GET", Handler: "GetAdByPosition", Auth: "none"},
		{Path: "/rotation-index", Method: "GET", Handler: "GetRotationIndex", Auth: "optional"},

		// Public API - Banners
		{Path: "/banners/today", Method: "GET", Handler: "GetTodayBanners", Auth: "none"},
		{Path: "/banners/date/:date", Method: "GET", Handler: "GetBannersByDate", Auth: "none"},

		// Admin API - Ad Units
		{Path: "/admin/units", Method: "GET", Handler: "ListAdUnits", Auth: "required"},
		{Path: "/admin/units", Method: "POST", Handler: "CreateAdUnit", Auth: "required"},
		{Path: "/admin/units/:id", Method: "GET", Handler: "GetAdUnit", Auth: "required"},
		{Path: "/admin/units/:id", Method: "PUT", Handler: "UpdateAdUnit", Auth: "required"},
		{Path: "/admin/units/:id", Method: "DELETE", Handler: "DeleteAdUnit", Auth: "required"},

		// Admin API - Banners
		{Path: "/admin/banners", Method: "GET", Handler: "ListBanners", Auth: "required"},
		{Path: "/admin/banners", Method: "POST", Handler: "CreateBanner", Auth: "required"},
		{Path: "/admin/banners/:id", Method: "GET", Handler: "GetBanner", Auth: "required"},
		{Path: "/admin/banners/:id", Method: "PUT", Handler: "UpdateBanner", Auth: "required"},
		{Path: "/admin/banners/:id", Method: "DELETE", Handler: "DeleteBanner", Auth: "required"},
	},
}

// New 플러그인 인스턴스 생성
func New() *AdvertisingPlugin {
	return &AdvertisingPlugin{}
}

// Name 플러그인 이름 반환
func (p *AdvertisingPlugin) Name() string {
	return "advertising"
}

// Migrate DB 마이그레이션 실행
func (p *AdvertisingPlugin) Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&domain.AdUnit{},
		&domain.AdRotationConfig{},
		&domain.CelebrationBanner{},
	)
}

// Initialize 플러그인 초기화
func (p *AdvertisingPlugin) Initialize(ctx *plugin.PluginContext) error {
	p.db = ctx.DB
	p.redis = ctx.Redis
	p.logger = ctx.Logger
	p.config = ctx.Config

	// Repository 생성
	adRepo := repository.NewAdRepository(p.db)

	// Services 생성
	gamService := service.NewGAMService(adRepo, p.config, p.logger)
	adsenseService := service.NewAdsenseService(adRepo, p.config, p.logger)
	bannerService := service.NewBannerService(adRepo, p.logger)
	adUnitService := service.NewAdUnitService(adRepo, gamService, adsenseService, p.config, p.logger)

	// Handlers 생성
	p.publicHandler = handler.NewPublicHandler(gamService, adsenseService, adUnitService, bannerService)
	p.adminHandler = handler.NewAdminHandler(adUnitService, bannerService)

	p.logger.Info("advertising plugin initialized")
	return nil
}

// RegisterRoutes 라우트 등록
func (p *AdvertisingPlugin) RegisterRoutes(router gin.IRouter) {
	// ============ Public API ============

	// GAM/AdSense 설정
	router.GET("/gam/config", p.publicHandler.GetGAMConfig)
	router.GET("/adsense/config", p.publicHandler.GetAdsenseConfig)
	router.GET("/units/:position", p.publicHandler.GetAdByPosition)
	router.GET("/rotation-index", p.publicHandler.GetRotationIndex)

	// 축하 배너
	router.GET("/banners/today", p.publicHandler.GetTodayBanners)
	router.GET("/banners/date/:date", p.publicHandler.GetBannersByDate)

	// ============ Admin API ============

	// 광고 단위 관리
	router.GET("/admin/units", p.adminHandler.ListAdUnits)
	router.POST("/admin/units", p.adminHandler.CreateAdUnit)
	router.GET("/admin/units/:id", p.adminHandler.GetAdUnit)
	router.PUT("/admin/units/:id", p.adminHandler.UpdateAdUnit)
	router.DELETE("/admin/units/:id", p.adminHandler.DeleteAdUnit)

	// 축하 배너 관리
	router.GET("/admin/banners", p.adminHandler.ListBanners)
	router.POST("/admin/banners", p.adminHandler.CreateBanner)
	router.GET("/admin/banners/:id", p.adminHandler.GetBanner)
	router.PUT("/admin/banners/:id", p.adminHandler.UpdateBanner)
	router.DELETE("/admin/banners/:id", p.adminHandler.DeleteBanner)
}

// Shutdown 플러그인 종료
func (p *AdvertisingPlugin) Shutdown() error {
	p.logger.Info("advertising plugin shutdown")
	return nil
}
