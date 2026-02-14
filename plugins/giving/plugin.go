// 나눔 플러그인 진입점
package giving

import (
	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/plugins/giving/handlers"
	"github.com/damoang/angple-backend/plugins/giving/repository"
	"github.com/damoang/angple-backend/plugins/giving/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// init 플러그인 팩토리 자동 등록
func init() {
	plugin.RegisterFactory("giving", func() plugin.Plugin {
		return New()
	}, Manifest)
}

// Manifest 나눔 플러그인 매니페스트
var Manifest = &plugin.PluginManifest{
	Name:        "giving",
	Version:     "1.0.0",
	Title:       "나눔 게시판",
	Description: "포인트 기반 번호 응모 나눔 시스템",
	Author:      "Damoang Team",
	License:     "Proprietary",
	Requires: plugin.Requires{
		Angple: ">=1.0.0",
	},
	Menus: []plugin.MenuConfig{
		{
			Title:         "나눔 관리",
			URL:           "/admin/plugins/giving",
			Icon:          "gift",
			ShowInSidebar: true,
			OrderNum:      40,
			ViewLevel:     9,
		},
	},
	Settings: []plugin.SettingConfig{
		{Key: "max_bid_numbers", Type: "number", Default: 100, Label: "최대 응모 번호 수", Min: intPtr(1), Max: intPtr(1000)},
		{Key: "commission_rate", Type: "number", Default: 50, Label: "수수료율(%)", Min: intPtr(0), Max: intPtr(100)},
	},
	Routes: []plugin.RouteConfig{
		{Path: "/list", Method: "GET", Handler: "ListGivings", Auth: "none"},
		{Path: "/:id", Method: "GET", Handler: "GetGivingDetail", Auth: "optional"},
		{Path: "/:id/visualization", Method: "GET", Handler: "GetVisualization", Auth: "none"},
		{Path: "/:id/live-status", Method: "GET", Handler: "GetLiveStatus", Auth: "none"},
		{Path: "/:id/bid", Method: "POST", Handler: "CreateBid", Auth: "required"},
		{Path: "/:id/bid", Method: "GET", Handler: "GetMyBids", Auth: "required"},
		{Path: "/admin/:id/pause", Method: "POST", Handler: "PauseGiving", Auth: "required"},
		{Path: "/admin/:id/resume", Method: "POST", Handler: "ResumeGiving", Auth: "required"},
		{Path: "/admin/:id/force-stop", Method: "POST", Handler: "ForceStopGiving", Auth: "required"},
		{Path: "/admin/:id/stats", Method: "GET", Handler: "GetAdminStats", Auth: "required"},
	},
}

func intPtr(v int) *int { return &v }

// GivingPlugin 나눔 플러그인
type GivingPlugin struct {
	db            *gorm.DB
	logger        plugin.Logger
	publicHandler *handlers.PublicHandler
	adminHandler  *handlers.AdminHandler
}

func New() *GivingPlugin             { return &GivingPlugin{} }
func (p *GivingPlugin) Name() string { return "giving" }

func (p *GivingPlugin) Migrate(db *gorm.DB) error {
	return nil
}

func (p *GivingPlugin) Initialize(ctx *plugin.PluginContext) error {
	p.db = ctx.DB
	p.logger = ctx.Logger

	repo := repository.NewGivingRepository(p.db)
	svc := service.NewGivingService(repo)
	p.publicHandler = handlers.NewPublicHandler(svc)
	p.adminHandler = handlers.NewAdminHandler(svc)

	p.logger.Info("Giving plugin initialized")
	return nil
}

func (p *GivingPlugin) RegisterRoutes(router gin.IRouter) {
	router.GET("/list", p.publicHandler.ListGivings)
	router.GET("/:id", p.publicHandler.GetGivingDetail)
	router.GET("/:id/visualization", p.publicHandler.GetVisualization)
	router.GET("/:id/live-status", p.publicHandler.GetLiveStatus)
	router.POST("/:id/bid", p.publicHandler.CreateBid)
	router.GET("/:id/bid", p.publicHandler.GetMyBids)

	admin := router.Group("/admin")
	admin.POST("/:id/pause", p.adminHandler.PauseGiving)
	admin.POST("/:id/resume", p.adminHandler.ResumeGiving)
	admin.POST("/:id/force-stop", p.adminHandler.ForceStopGiving)
	admin.GET("/:id/stats", p.adminHandler.GetAdminStats)

	p.logger.Info("Giving routes registered")
}

func (p *GivingPlugin) Shutdown() error {
	p.logger.Info("Giving plugin shutdown")
	return nil
}
