package marketplace

import (
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/handler"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/repository"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/service"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// init 플러그인 팩토리 자동 등록
func init() {
	plugin.RegisterFactory("marketplace", func() plugin.Plugin {
		return New()
	}, Manifest)
}

// Manifest 플러그인 매니페스트
var Manifest = &plugin.PluginManifest{
	Name:        "marketplace",
	Version:     "1.0.0",
	Title:       "중고거래",
	Description: "중고 상품 거래 플러그인",
	Author:      "SDK Corporation",
	Requires: plugin.Requires{
		Angple: ">=1.0.0",
	},
	Migrations: []plugin.Migration{
		{File: "001_init.up.sql", Version: 1},
	},
	Routes: []plugin.RouteConfig{
		// 공개 API
		{Path: "/items", Method: "GET", Handler: "ListItems", Auth: "optional"},
		{Path: "/items/:id", Method: "GET", Handler: "GetItem", Auth: "optional"},
		{Path: "/categories", Method: "GET", Handler: "ListCategories", Auth: "none"},
		{Path: "/categories/tree", Method: "GET", Handler: "ListCategoryTree", Auth: "none"},
		{Path: "/categories/:id", Method: "GET", Handler: "GetCategory", Auth: "none"},
		// 인증 필요 API
		{Path: "/items", Method: "POST", Handler: "CreateItem", Auth: "required"},
		{Path: "/items/:id", Method: "PUT", Handler: "UpdateItem", Auth: "required"},
		{Path: "/items/:id", Method: "DELETE", Handler: "DeleteItem", Auth: "required"},
		{Path: "/items/:id/status", Method: "PATCH", Handler: "UpdateStatus", Auth: "required"},
		{Path: "/items/:id/bump", Method: "POST", Handler: "BumpItem", Auth: "required"},
		{Path: "/items/:id/wish", Method: "POST", Handler: "ToggleWish", Auth: "required"},
		{Path: "/my/items", Method: "GET", Handler: "ListMyItems", Auth: "required"},
		{Path: "/my/wishes", Method: "GET", Handler: "ListWishes", Auth: "required"},
		// 관리자 API
		{Path: "/admin/categories", Method: "POST", Handler: "CreateCategory", Auth: "admin"},
		{Path: "/admin/categories/:id", Method: "PUT", Handler: "UpdateCategory", Auth: "admin"},
		{Path: "/admin/categories/:id", Method: "DELETE", Handler: "DeleteCategory", Auth: "admin"},
	},
	Settings: []plugin.SettingConfig{
		{
			Key:     "max_images",
			Type:    "number",
			Default: 10,
			Label:   "최대 이미지 개수",
		},
		{
			Key:     "bump_cooldown_hours",
			Type:    "number",
			Default: 24,
			Label:   "끌올 쿨다운 (시간)",
		},
		{
			Key:     "auto_hide_days",
			Type:    "number",
			Default: 30,
			Label:   "자동 숨김 (일)",
		},
	},
}

// Plugin 마켓플레이스 플러그인
type Plugin struct {
	db         *gorm.DB
	jwtManager *jwt.Manager

	// Repositories
	itemRepo     repository.ItemRepository
	categoryRepo repository.CategoryRepository
	wishRepo     repository.WishRepository

	// Services
	itemService     service.ItemService
	categoryService service.CategoryService
	wishService     service.WishService

	// Handlers
	itemHandler     *handler.ItemHandler
	categoryHandler *handler.CategoryHandler
	wishHandler     *handler.WishHandler
}

// New 플러그인 인스턴스 생성 (빈 인스턴스, Initialize에서 초기화)
func New() *Plugin {
	return &Plugin{}
}

// Name 플러그인 이름 반환
func (p *Plugin) Name() string {
	return "marketplace"
}

// Initialize 플러그인 초기화
func (p *Plugin) Initialize(ctx *plugin.PluginContext) error {
	p.db = ctx.DB

	// JWT Manager 타입 변환
	if jm, ok := ctx.JWTManager.(*jwt.Manager); ok {
		p.jwtManager = jm
	}

	// Repository 초기화
	p.itemRepo = repository.NewItemRepository(p.db)
	p.categoryRepo = repository.NewCategoryRepository(p.db)
	p.wishRepo = repository.NewWishRepository(p.db)

	// Service 초기화
	p.itemService = service.NewItemService(p.itemRepo, p.categoryRepo, p.wishRepo)
	p.categoryService = service.NewCategoryService(p.categoryRepo)
	p.wishService = service.NewWishService(p.wishRepo, p.itemRepo)

	// Handler 초기화
	p.itemHandler = handler.NewItemHandler(p.itemService)
	p.categoryHandler = handler.NewCategoryHandler(p.categoryService)
	p.wishHandler = handler.NewWishHandler(p.wishService)

	return nil
}

// Shutdown 플러그인 종료
func (p *Plugin) Shutdown() error {
	return nil
}

// GetManifest 매니페스트 반환
func (p *Plugin) GetManifest() *plugin.PluginManifest {
	return Manifest
}

// Migrate DB 마이그레이션 실행
func (p *Plugin) Migrate(_ *gorm.DB) error {
	// 마이그레이션은 SQL 파일로 처리
	return nil
}

// RegisterRoutes 라우트 등록 (plugin.Plugin 인터페이스 구현)
func (p *Plugin) RegisterRoutes(router gin.IRouter) {
	rg, ok := router.(*gin.RouterGroup)
	if !ok {
		return
	}
	// 공개 API
	rg.GET("/items", p.itemHandler.ListItems)
	rg.GET("/items/:id", p.itemHandler.GetItem)
	rg.GET("/categories", p.categoryHandler.ListCategories)
	rg.GET("/categories/tree", p.categoryHandler.ListCategoryTree)
	rg.GET("/categories/:id", p.categoryHandler.GetCategory)

	// 인증 필요 API
	authRequired := rg.Group("")
	authRequired.Use(middleware.JWTAuth(p.jwtManager))
	{
		authRequired.POST("/items", p.itemHandler.CreateItem)
		authRequired.PUT("/items/:id", p.itemHandler.UpdateItem)
		authRequired.DELETE("/items/:id", p.itemHandler.DeleteItem)
		authRequired.PATCH("/items/:id/status", p.itemHandler.UpdateStatus)
		authRequired.POST("/items/:id/bump", p.itemHandler.BumpItem)
		authRequired.POST("/items/:id/wish", p.wishHandler.ToggleWish)
		authRequired.GET("/my/items", p.itemHandler.ListMyItems)
		authRequired.GET("/my/wishes", p.wishHandler.ListWishes)
	}

	// 관리자 API (현재는 인증만 체크, 추후 관리자 권한 체크 추가)
	adminRequired := rg.Group("/admin")
	adminRequired.Use(middleware.JWTAuth(p.jwtManager))
	{
		adminRequired.POST("/categories", p.categoryHandler.CreateCategory)
		adminRequired.PUT("/categories/:id", p.categoryHandler.UpdateCategory)
		adminRequired.DELETE("/categories/:id", p.categoryHandler.DeleteCategory)
	}
}
