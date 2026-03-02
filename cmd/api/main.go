package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/config"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/handler"
	v1handler "github.com/damoang/angple-backend/internal/handler/v1"
	v2handler "github.com/damoang/angple-backend/internal/handler/v2"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/migration"
	"github.com/damoang/angple-backend/internal/plugin"
	pluginstoreHandler "github.com/damoang/angple-backend/internal/pluginstore/handler"
	pluginstoreRepo "github.com/damoang/angple-backend/internal/pluginstore/repository"
	pluginstoreSvc "github.com/damoang/angple-backend/internal/pluginstore/service"
	"github.com/damoang/angple-backend/internal/repository"
	gnuboard "github.com/damoang/angple-backend/internal/domain/gnuboard"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	v2routes "github.com/damoang/angple-backend/internal/routes/v2"
	"github.com/damoang/angple-backend/internal/service"
	v2svc "github.com/damoang/angple-backend/internal/service/v2"
	"github.com/damoang/angple-backend/internal/ws"
	pkgcache "github.com/damoang/angple-backend/pkg/cache"
	pkges "github.com/damoang/angple-backend/pkg/elasticsearch"
	"github.com/damoang/angple-backend/pkg/i18n"
	"github.com/damoang/angple-backend/pkg/jwt"
	pkglogger "github.com/damoang/angple-backend/pkg/logger"
	pkgredis "github.com/damoang/angple-backend/pkg/redis"
	pkgstorage "github.com/damoang/angple-backend/pkg/storage"
	"github.com/damoang/angple-backend/internal/dantry"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// @title           Angple Backend API
// @version         2.0
// @description     Angple Community Platform - Open Source Backend API
//
// @license.name    MIT
//
// @host            localhost:8082
// @BasePath        /api/v2
//
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description JWT Authorization header using the Bearer scheme. Example: "Bearer {token}"

// getConfigPath returns config file path based on APP_ENV environment variable
func getConfigPath() string {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "local"
	}
	return fmt.Sprintf("configs/config.%s.yaml", env)
}

func main() {
	dotenvFiles := config.LoadDotEnv()

	// 로거 초기화
	pkglogger.Init()
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "local"
	}
	pkglogger.InitStructured(env)
	pkglogger.Info("APP_ENV=%s, loaded env files: %v", env, dotenvFiles)

	// 설정 로드
	configPath := getConfigPath()
	pkglogger.Info("Loading config from: %s", configPath)
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	config.LogResolved(cfg)

	// MySQL 연결
	db, err := initDB(cfg)
	if err != nil {
		pkglogger.Info("Warning: Failed to connect to database: %v (continuing without DB)", err)
		db = nil
	} else {
		pkglogger.Info("Connected to MySQL")
		if err := migration.Run(db); err != nil {
			pkglogger.Info("Migration warning: %v", err)
		}
		if err := migration.RunV2Schema(db); err != nil {
			pkglogger.Info("V2 schema migration warning: %v", err)
		}
		if env == "" || env == "development" || env == "local" {
			if err := migration.MigrateV2Data(db); err != nil {
				pkglogger.Info("V2 data migration warning: %v", err)
			}
		}
	}

	// Redis 연결
	redisClient, err := pkgredis.NewClient(
		cfg.Redis.Host,
		cfg.Redis.Port,
		cfg.Redis.Password,
		cfg.Redis.DB,
		cfg.Redis.PoolSize,
	)
	if err != nil {
		pkglogger.Info("Warning: Failed to connect to Redis: %v (continuing without Redis)", err)
		redisClient = nil
	} else {
		pkglogger.Info("Connected to Redis")
	}

	// Cache Service
	var cacheService pkgcache.Service
	if redisClient != nil {
		cacheService = pkgcache.NewService(redisClient)
		pkglogger.Info("Cache service initialized")
	}
	// cacheService is used in v1 API handlers

	// Elasticsearch 연결
	var esClient *pkges.Client
	if cfg.Elasticsearch.Enabled && len(cfg.Elasticsearch.Addresses) > 0 {
		var esErr error
		esClient, esErr = pkges.NewClient(cfg.Elasticsearch.Addresses, cfg.Elasticsearch.Username, cfg.Elasticsearch.Password)
		if esErr != nil {
			pkglogger.Info("Warning: Elasticsearch connection failed: %v (continuing without ES)", esErr)
			esClient = nil
		} else {
			pkglogger.Info("Connected to Elasticsearch")
		}
	}

	// S3-compatible storage
	var s3Client *pkgstorage.S3Client
	if cfg.Storage.Enabled && cfg.Storage.Bucket != "" {
		var s3Err error
		s3Client, s3Err = pkgstorage.NewS3Client(pkgstorage.S3Config{
			Endpoint:        cfg.Storage.Endpoint,
			Region:          cfg.Storage.Region,
			AccessKeyID:     cfg.Storage.AccessKeyID,
			SecretAccessKey: cfg.Storage.SecretAccessKey,
			Bucket:          cfg.Storage.Bucket,
			CDNURL:          cfg.Storage.CDNURL,
			BasePath:        cfg.Storage.BasePath,
			ForcePathStyle:  cfg.Storage.ForcePathStyle,
		})
		if s3Err != nil {
			pkglogger.Info("Warning: S3 storage init failed: %v (continuing without S3)", s3Err)
			s3Client = nil
		} else {
			pkglogger.Info("Connected to S3 storage")
		}
	}

	// WebSocket Hub
	wsHub := ws.NewHub(redisClient)
	go wsHub.Run()

	// JWT Manager
	jwtManager := jwt.NewManager(
		cfg.JWT.Secret,
		cfg.JWT.ExpiresIn,
		cfg.JWT.RefreshIn,
	)

	// Plugin HookManager
	pluginLogger := plugin.NewDefaultLogger("plugin")
	_ = plugin.NewHookManager(pluginLogger)

	// Gin 라우터 생성
	router := gin.Default()

	// CORS 설정
	allowOrigins := cfg.CORS.AllowOrigins
	if allowOrigins == "" {
		allowOrigins = "http://localhost:3000"
	}

	corsConfig := cors.Config{
		AllowOrigins:     []string{allowOrigins},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-API-Key", "X-CSRF-Token", "X-Request-ID"},
		AllowCredentials: true,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		ExposeHeaders:    []string{"X-Request-ID", "X-RateLimit-Remaining", "X-Cache"},
		MaxAge:           86400,
	}
	if len(corsConfig.AllowOrigins) == 1 && corsConfig.AllowOrigins[0] != "" {
		corsConfig.AllowOrigins = splitAndTrim(allowOrigins, ",")
	}
	router.Use(cors.New(corsConfig))

	// i18n Bundle
	i18nBundle := i18n.NewBundle(i18n.LocaleKo)
	for locale, msgs := range i18n.DefaultMessages() {
		i18nBundle.LoadMessages(locale, msgs)
	}
	if _, err := os.Stat("i18n"); err == nil {
		if err := i18nBundle.LoadDir("i18n"); err != nil {
			log.Printf("warning: i18n LoadDir failed: %v", err)
		}
	}
	_ = i18nBundle

	// Middleware
	router.Use(middleware.I18n())
	router.Use(middleware.SecurityHeaders())
	router.Use(middleware.InputSanitizer())
	router.Use(middleware.Metrics())
	router.Use(middleware.RequestLogger())

	if redisClient != nil && !cfg.IsDevelopment() {
		router.Use(middleware.RateLimit(redisClient, middleware.DefaultRateLimitConfig()))
		// TODO: WriteRateLimit 구현 후 활성화
		// router.Use(middleware.WriteRateLimit(redisClient, middleware.WriteRateLimitConfig(), "/api/v2/media"))
	}

	// Prometheus metrics
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Health Check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "angple-backend",
			"time":    time.Now().Unix(),
		})
	})

	// Swagger UI
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// v2 API routes (only if DB is connected)
	if db != nil {
		v2UserRepo := v2repo.NewUserRepository(db)
		siteRepo := repository.NewSiteRepository(db)

		// Gnuboard repositories for v1 API (g5_* tables)
		gnuBoardRepo := gnurepo.NewBoardRepository(db)
		gnuWriteRepo := gnurepo.NewWriteRepository(db)
		gnuFileRepo := gnurepo.NewFileRepository(db)
		_ = gnurepo.NewMemberRepository(db) // For future auth integration

		// v2 Core API
		v2PostRepo := v2repo.NewPostRepository(db)
		v2CommentRepo := v2repo.NewCommentRepository(db)
		v2BoardRepo := v2repo.NewBoardRepository(db)
		v2PointRepo := v2repo.NewPointRepository(db)

		// 리비전
		v2RevisionRepo := v2repo.NewRevisionRepository(db)

		// 권한 체크
		permChecker := middleware.NewDBBoardPermissionChecker(v2BoardRepo)
		v2Handler := v2handler.NewV2Handler(v2UserRepo, v2PostRepo, v2CommentRepo, v2BoardRepo, permChecker)
		v2Handler.SetPointRepository(v2PointRepo)
		v2Handler.SetRevisionRepository(v2RevisionRepo)

		// TODO: 경험치 서비스 구현 후 활성화
		// expSvc := v2svc.NewExpService(v2UserRepo)
		// v2Handler.SetExpService(expSvc)

		v2routes.Setup(router, v2Handler, jwtManager, permChecker)
		v2routes.SetupAdminPosts(router, v2Handler, jwtManager)

		// MyPage routes (point, exp)
		v2ExpRepo := v2repo.NewExpRepository(db)
		pointHandler := v2handler.NewPointHandler(v2PointRepo)
		expHandler := v2handler.NewExpHandler(v2ExpRepo)
		v2routes.SetupMyPage(router, pointHandler, expHandler, jwtManager)

		// DisciplineLog routes (uses gnuboard g5_write_disciplinelog table)
		disciplineLogHandler := v2handler.NewDisciplineLogHandler(gnuWriteRepo)
		v2routes.SetupDisciplineLog(router, disciplineLogHandler, jwtManager)

		// v2 Auth
		v2AuthSvc := v2svc.NewV2AuthService(v2UserRepo, jwtManager)
		v2AuthHandler := v2handler.NewV2AuthHandler(v2AuthSvc)
		v2routes.SetupAuth(router, v2AuthHandler, jwtManager)

		// v1 compatibility routes (frontend calls /api/v1/*)
		v1Auth := router.Group("/api/v1/auth")
		v1Auth.POST("/login", v2AuthHandler.Login)
		v1Auth.POST("/refresh", v2AuthHandler.RefreshToken)
		v1Auth.POST("/logout", v2AuthHandler.Logout)
		v1Auth.GET("/me", middleware.JWTAuth(jwtManager), v2AuthHandler.GetMe)
		v1Auth.GET("/profile", middleware.JWTAuth(jwtManager), v2AuthHandler.GetMe)
		router.GET("/api/v1/menus/sidebar", func(c *gin.Context) {
			var menus []domain.Menu
			if err := db.Where("is_active = ? AND show_in_sidebar = ?", true, true).
				Order("order_num ASC, id ASC").Find(&menus).Error; err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
				return
			}
			menuMap := make(map[int64]*domain.Menu, len(menus))
			var roots []*domain.Menu
			for i := range menus {
				menus[i].Children = []*domain.Menu{}
				menuMap[menus[i].ID] = &menus[i]
			}
			for i := range menus {
				if menus[i].ParentID != nil {
					if parent, ok := menuMap[*menus[i].ParentID]; ok {
						parent.Children = append(parent.Children, &menus[i])
						continue
					}
				}
				roots = append(roots, &menus[i])
			}
			result := make([]domain.MenuResponse, 0, len(roots))
			for _, r := range roots {
				result = append(result, r.ToResponse())
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
		})

		// ========== Admin Menu API ==========
		adminMenus := router.Group("/api/v1/admin/menus")
		adminMenus.Use(middleware.JWTAuth(jwtManager))
		adminMenus.Use(middleware.RequireAdmin())

		// GET /api/v1/admin/menus - 전체 메뉴 조회 (트리 구조)
		adminMenus.GET("", func(c *gin.Context) {
			var menus []domain.Menu
			if err := db.Order("order_num ASC, id ASC").Find(&menus).Error; err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
				return
			}

			// Build tree structure
			menuMap := make(map[int64]*domain.Menu, len(menus))
			var roots []*domain.Menu
			for i := range menus {
				menus[i].Children = []*domain.Menu{}
				menuMap[menus[i].ID] = &menus[i]
			}
			for i := range menus {
				if menus[i].ParentID != nil && *menus[i].ParentID != 0 {
					if parent, ok := menuMap[*menus[i].ParentID]; ok {
						parent.Children = append(parent.Children, &menus[i])
						continue
					}
				}
				roots = append(roots, &menus[i])
			}

			result := make([]domain.AdminMenuResponse, 0, len(roots))
			for _, r := range roots {
				result = append(result, r.ToAdminResponse())
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
		})

		// POST /api/v1/admin/menus - 메뉴 생성
		adminMenus.POST("", func(c *gin.Context) {
			var req domain.CreateMenuRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
				return
			}

			// Calculate depth
			depth := 0
			if req.ParentID != nil && *req.ParentID != 0 {
				var parent domain.Menu
				if err := db.First(&parent, *req.ParentID).Error; err == nil {
					depth = parent.Depth + 1
				}
			}

			// Get max order_num
			var maxOrder int
			db.Model(&domain.Menu{}).Select("COALESCE(MAX(order_num), 0)").Scan(&maxOrder)

			menu := domain.Menu{
				ParentID:      req.ParentID,
				Title:         req.Title,
				URL:           req.URL,
				Icon:          req.Icon,
				Shortcut:      req.Shortcut,
				Description:   req.Description,
				Target:        req.Target,
				Depth:         depth,
				OrderNum:      maxOrder + 1,
				ViewLevel:     req.ViewLevel,
				ShowInHeader:  req.ShowInHeader,
				ShowInSidebar: req.ShowInSidebar,
				IsActive:      req.IsActive,
			}

			if err := db.Create(&menu).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
				return
			}

			c.JSON(http.StatusCreated, gin.H{"success": true, "data": menu.ToAdminResponse()})
		})

		// PUT /api/v1/admin/menus/:id - 메뉴 수정
		adminMenus.PUT("/:id", func(c *gin.Context) {
			id, err := strconv.ParseInt(c.Param("id"), 10, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "Invalid menu ID"}})
				return
			}

			var menu domain.Menu
			if err := db.First(&menu, id).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"message": "Menu not found"}})
				return
			}

			var req domain.UpdateMenuRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
				return
			}

			// Update fields if provided
			if req.Title != nil {
				menu.Title = *req.Title
			}
			if req.URL != nil {
				menu.URL = *req.URL
			}
			if req.Icon != nil {
				menu.Icon = *req.Icon
			}
			if req.Shortcut != nil {
				menu.Shortcut = *req.Shortcut
			}
			if req.Description != nil {
				menu.Description = *req.Description
			}
			if req.Target != nil {
				menu.Target = *req.Target
			}
			if req.ShowInHeader != nil {
				menu.ShowInHeader = *req.ShowInHeader
			}
			if req.ShowInSidebar != nil {
				menu.ShowInSidebar = *req.ShowInSidebar
			}
			if req.ViewLevel != nil {
				menu.ViewLevel = *req.ViewLevel
			}
			if req.IsActive != nil {
				menu.IsActive = *req.IsActive
			}

			// 트랜잭션으로 메뉴 수정 및 하위 메뉴 연동 처리
			tx := db.Begin()
			if err := tx.Save(&menu).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
				return
			}

			// 부모 메뉴가 숨겨지면 하위 메뉴도 함께 숨김 처리
			cascadeUpdates := make(map[string]interface{})
			if req.IsActive != nil && !*req.IsActive {
				cascadeUpdates["is_active"] = false
			}
			if req.ShowInHeader != nil && !*req.ShowInHeader {
				cascadeUpdates["show_in_header"] = false
			}
			if req.ShowInSidebar != nil && !*req.ShowInSidebar {
				cascadeUpdates["show_in_sidebar"] = false
			}

			if len(cascadeUpdates) > 0 {
				// 재귀적으로 모든 하위 메뉴의 ID를 수집
				var collectChildIDs func(parentID int64) []int64
				collectChildIDs = func(parentID int64) []int64 {
					var childIDs []int64
					var children []domain.Menu
					tx.Where("parent_id = ?", parentID).Find(&children)
					for _, child := range children {
						childIDs = append(childIDs, child.ID)
						childIDs = append(childIDs, collectChildIDs(child.ID)...)
					}
					return childIDs
				}

				childIDs := collectChildIDs(menu.ID)
				if len(childIDs) > 0 {
					if err := tx.Model(&domain.Menu{}).Where("id IN ?", childIDs).Updates(cascadeUpdates).Error; err != nil {
						tx.Rollback()
						c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
						return
					}
				}
			}

			tx.Commit()
			c.JSON(http.StatusOK, gin.H{"success": true, "data": menu.ToAdminResponse()})
		})

		// DELETE /api/v1/admin/menus/:id - 메뉴 삭제
		adminMenus.DELETE("/:id", func(c *gin.Context) {
			id, err := strconv.ParseInt(c.Param("id"), 10, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "Invalid menu ID"}})
				return
			}

			// Delete menu and its children
			if err := db.Where("id = ? OR parent_id = ?", id, id).Delete(&domain.Menu{}).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
				return
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		})

		// POST /api/v1/admin/menus/reorder - 메뉴 순서 변경
		adminMenus.POST("/reorder", func(c *gin.Context) {
			var req domain.ReorderMenusRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
				return
			}

			// Update order in transaction
			tx := db.Begin()
			for _, item := range req.Items {
				updates := map[string]any{
					"order_num": item.OrderNum,
					"parent_id": item.ParentID,
				}
				// Calculate depth
				depth := 0
				if item.ParentID != nil && *item.ParentID != 0 {
					var parent domain.Menu
					if err := tx.First(&parent, *item.ParentID).Error; err == nil {
						depth = parent.Depth + 1
					}
				}
				updates["depth"] = depth

				if err := tx.Model(&domain.Menu{}).Where("id = ?", item.ID).Updates(updates).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
					return
				}
			}
			tx.Commit()

			c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		})

		router.GET("/api/v1/boards/:slug/notices", func(c *gin.Context) {
			slug := c.Param("slug")
			ctx := c.Request.Context()

			// Try cache first
			if cacheService != nil {
				if cached, err := cacheService.GetNotices(ctx, slug); err == nil {
					c.Header("X-Cache", "HIT")
					c.Data(http.StatusOK, "application/json", cached)
					return
				}
			}

			// Get board to find notice IDs
			board, err := gnuBoardRepo.FindByID(slug)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
				return
			}

			// Parse notice IDs from bo_notice
			noticeIDs := gnurepo.ParseNoticeIDs(board.BoNotice)
			if len(noticeIDs) == 0 {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
				return
			}

			// Get notice posts from g5_write_{slug}
			notices, err := gnuWriteRepo.FindNotices(slug, noticeIDs)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
				return
			}

			// Transform to v1 format (all are notices)
			noticeIDMap := v1handler.BuildNoticeIDMap(noticeIDs)
			items := v1handler.TransformToV1Posts(notices, noticeIDMap)

			response := gin.H{"success": true, "data": items}

			// Cache the response
			if cacheService != nil {
				_ = cacheService.SetNotices(ctx, slug, response)
			}

			c.Header("X-Cache", "MISS")
			c.JSON(http.StatusOK, response)
		})
		router.GET("/api/ads/celebration/today", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		})
		// v1 notifications (stub for now)
		router.GET("/api/v1/notifications/unread-count", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"count": 0}})
		})
		// v1 members memo — 관리자가 회원에 대해 남긴 메모 (g5_member_memo)
		router.GET("/api/v1/members/:id/memo", middleware.OptionalJWTAuth(jwtManager), func(c *gin.Context) {
			// 관리자만 메모 조회 가능
			userLevel := middleware.GetUserLevel(c)
			if userLevel < 10 {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
				return
			}
			currentUserID := middleware.GetUserID(c)
			targetID := c.Param("id")

			type memoRow struct {
				ID       int    `gorm:"column:id" json:"id"`
				MemberID string `gorm:"column:member_id" json:"member_id"`
				TargetID string `gorm:"column:target_member_id" json:"target_member_id"`
				Memo     string `gorm:"column:memo" json:"memo"`
				Color    string `gorm:"column:color" json:"color"`
			}
			var memo memoRow
			err := db.Table("g5_member_memo").
				Where("member_id = ? AND target_member_id = ?", currentUserID, targetID).
				First(&memo).Error
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": memo})
		})

		// Admin member memo CRUD
		adminMemoGroup := router.Group("/api/v1/admin/members")
		adminMemoGroup.Use(middleware.JWTAuth(jwtManager), middleware.RequireAdmin())

		// GET /api/v1/admin/members/:mbId/memos — 특정 회원에 대한 메모 목록
		adminMemoGroup.GET("/:mbId/memos", func(c *gin.Context) {
			targetID := c.Param("mbId")

			type memoResult struct {
				ID         int     `gorm:"column:id" json:"id"`
				MemberID   string  `gorm:"column:member_id" json:"member_id"`
				TargetID   string  `gorm:"column:target_member_id" json:"target_member_id"`
				Memo       string  `gorm:"column:memo" json:"memo"`
				MemoDetail *string `gorm:"column:memo_detail" json:"memo_detail"`
				Color      string  `gorm:"column:color" json:"color"`
				CreatedAt  string  `gorm:"column:created_at" json:"created_at"`
				UpdatedAt  *string `gorm:"column:updated_at" json:"updated_at"`
			}
			var memos []memoResult
			if err := db.Table("g5_member_memo").
				Where("target_member_id = ?", targetID).
				Order("created_at DESC").
				Find(&memos).Error; err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": memos})
		})

		// POST /api/v1/admin/members/:mbId/memos — 메모 작성
		adminMemoGroup.POST("/:mbId/memos", func(c *gin.Context) {
			targetID := c.Param("mbId")
			currentUserID := middleware.GetUserID(c)

			var req struct {
				Memo       string `json:"memo" binding:"required"`
				MemoDetail string `json:"memo_detail"`
				Color      string `json:"color"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "memo is required"})
				return
			}

			color := req.Color
			if color == "" {
				color = "yellow"
			}

			memo := map[string]interface{}{
				"member_id":        currentUserID,
				"target_member_id": targetID,
				"memo":             req.Memo,
				"memo_detail":      req.MemoDetail,
				"color":            color,
			}
			if err := db.Table("g5_member_memo").Create(memo).Error; err != nil {
				// 이미 존재하면 UPDATE
				if err := db.Table("g5_member_memo").
					Where("member_id = ? AND target_member_id = ?", currentUserID, targetID).
					Updates(map[string]interface{}{
						"memo":        req.Memo,
						"memo_detail": req.MemoDetail,
						"color":       color,
					}).Error; err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "메모 저장 실패"})
					return
				}
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "메모 저장 완료"})
		})

		// PUT /api/v1/admin/memos/:id — 메모 수정
		router.PUT("/api/v1/admin/memos/:id", middleware.JWTAuth(jwtManager), middleware.RequireAdmin(), func(c *gin.Context) {
			memoID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid memo ID"})
				return
			}

			var req struct {
				Memo       string `json:"memo"`
				MemoDetail string `json:"memo_detail"`
				Color      string `json:"color"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "요청 형식 오류"})
				return
			}

			updates := map[string]interface{}{}
			if req.Memo != "" {
				updates["memo"] = req.Memo
			}
			if req.MemoDetail != "" {
				updates["memo_detail"] = req.MemoDetail
			}
			if req.Color != "" {
				updates["color"] = req.Color
			}

			if len(updates) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "수정할 내용이 없습니다"})
				return
			}

			if err := db.Table("g5_member_memo").Where("id = ?", memoID).Updates(updates).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "메모 수정 실패"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "메모 수정 완료"})
		})

		// DELETE /api/v1/admin/memos/:id — 메모 삭제
		router.DELETE("/api/v1/admin/memos/:id", middleware.JWTAuth(jwtManager), middleware.RequireAdmin(), func(c *gin.Context) {
			memoID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid memo ID"})
				return
			}

			if err := db.Table("g5_member_memo").Where("id = ?", memoID).Delete(nil).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "메모 삭제 실패"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "메모 삭제 완료"})
		})

		// GET /api/v1/admin/members/memos/bulk — 다수 회원 메모 존재 여부 (게시판 목록용)
		router.GET("/api/v1/admin/members/memos/bulk", middleware.JWTAuth(jwtManager), middleware.RequireAdmin(), func(c *gin.Context) {
			memberIDs := c.Query("member_ids")
			if memberIDs == "" {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": map[string]any{}})
				return
			}

			currentUserID := middleware.GetUserID(c)
			ids := strings.Split(memberIDs, ",")

			type memoCheck struct {
				TargetID string `gorm:"column:target_member_id"`
				Memo     string `gorm:"column:memo"`
				Color    string `gorm:"column:color"`
			}
			var memos []memoCheck
			db.Table("g5_member_memo").
				Select("target_member_id, memo, color").
				Where("member_id = ? AND target_member_id IN ?", currentUserID, ids).
				Find(&memos)

			result := make(map[string]gin.H, len(memos))
			for _, m := range memos {
				result[m.TargetID] = gin.H{"memo": m.Memo, "color": m.Color}
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
		})
		// 추천 글 / 위젯 (프론트엔드 홈페이지용)
		router.GET("/api/v1/recommended/index-widgets", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"news_tabs":    []any{},
				"economy_tabs": []any{},
				"gallery":      []any{},
				"group_tabs": gin.H{
					"all":   []any{},
					"24h":   []any{},
					"week":  []any{},
					"month": []any{},
				},
			})
		})
		// v1 boards routes → use Gnuboard g5_* tables
		v1Boards := router.Group("/api/v1/boards")
		v1Boards.Use(middleware.OptionalJWTAuth(jwtManager))

		// GET /api/v1/boards/:slug - Get board info from g5_board
		v1Boards.GET("/:slug", func(c *gin.Context) {
			slug := c.Param("slug")
			ctx := c.Request.Context()

			// Try cache first
			if cacheService != nil {
				if cached, err := cacheService.GetBoard(ctx, slug); err == nil {
					c.Header("X-Cache", "HIT")
					c.Data(http.StatusOK, "application/json", cached)
					return
				}
			}

			board, err := gnuBoardRepo.FindByID(slug)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Board not found"})
				return
			}

			response := gin.H{
				"success": true,
				"data":    v1handler.TransformToV1Board(board),
			}

			// Cache the response
			if cacheService != nil {
				_ = cacheService.SetBoard(ctx, slug, response)
			}

			c.Header("X-Cache", "MISS")
			c.JSON(http.StatusOK, response)
		})

		// GET /api/v1/boards/:slug/posts - Get posts from g5_write_{slug}
		v1Boards.GET("/:slug/posts", func(c *gin.Context) {
			slug := c.Param("slug")
			ctx := c.Request.Context()

			page, err2 := strconv.Atoi(c.DefaultQuery("page", "1"))
			if err2 != nil || page < 1 {
				page = 1
			}
			limit, err3 := strconv.Atoi(c.DefaultQuery("limit", "20"))
			if err3 != nil || limit < 1 || limit > 100 {
				limit = 20
			}

			// Try cache first
			if cacheService != nil {
				if cached, err := cacheService.GetPosts(ctx, slug, page, limit); err == nil {
					c.Header("X-Cache", "HIT")
					c.Data(http.StatusOK, "application/json", cached)
					return
				}
			}

			// Check board exists in g5_board
			board, err := gnuBoardRepo.FindByID(slug)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}, "meta": gin.H{"total": 0, "page": 1, "limit": 20}})
				return
			}

			// Get posts from g5_write_{slug}
			posts, total, err := gnuWriteRepo.FindPosts(slug, page, limit)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}, "meta": gin.H{"total": 0, "page": page, "limit": limit}})
				return
			}

			// Build notice ID map from board settings
			noticeIDs := gnurepo.ParseNoticeIDs(board.BoNotice)
			noticeIDMap := v1handler.BuildNoticeIDMap(noticeIDs)

			// Transform to v1 format
			items := v1handler.TransformToV1Posts(posts, noticeIDMap)

			response := gin.H{
				"success": true,
				"data":    items,
				"meta":    gin.H{"board_id": slug, "page": page, "limit": limit, "total": total},
			}

			// Cache the response
			if cacheService != nil {
				_ = cacheService.SetPosts(ctx, slug, page, limit, response)
			}

			c.Header("X-Cache", "MISS")
			c.JSON(http.StatusOK, response)
		})

		// GET /api/v1/boards/:slug/posts/:id - Get single post from g5_write_{slug}
		v1Boards.GET("/:slug/posts/:id", func(c *gin.Context) {
			slug := c.Param("slug")
			id, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid post ID"})
				return
			}

			// Check board exists
			board, err := gnuBoardRepo.FindByID(slug)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Board not found"})
				return
			}

			// Get post from g5_write_{slug}
			post, err := gnuWriteRepo.FindPostByID(slug, id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Post not found"})
				return
			}

			// Increment view count
			if vcErr := gnuWriteRepo.IncrementHit(slug, id); vcErr != nil {
				log.Printf("IncrementHit error: %v", vcErr)
			}

			// Check if this post is a notice
			noticeIDs := gnurepo.ParseNoticeIDs(board.BoNotice)
			isNotice := false
			for _, nid := range noticeIDs {
				if nid == id {
					isNotice = true
					break
				}
			}

			postDetail := v1handler.TransformToV1PostDetail(post, isNotice)

			// 비밀글 접근 제어: 작성자 또는 관리자만 내용 열람 가능
			if strings.Contains(post.WrOption, "secret") {
				currentUserID := middleware.GetUserID(c)
				currentUserLevel := middleware.GetUserLevel(c)
				isAuthor := currentUserID != "" && currentUserID == post.MbID
				isAdmin := currentUserLevel >= 10
				if !isAuthor && !isAdmin {
					postDetail["content"] = ""
				}
			}

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    postDetail,
			})
		})

		// GET /api/v1/boards/:slug/posts/:id/comments - Get comments from g5_write_{slug}
		v1Boards.GET("/:slug/posts/:id/comments", func(c *gin.Context) {
			slug := c.Param("slug")
			id, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid post ID"})
				return
			}

			// Get comments from g5_write_{slug} where wr_parent = id and wr_is_comment = 1
			comments, err := gnuWriteRepo.FindComments(slug, id)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    v1handler.TransformToV1Comments(comments),
			})
		})

		// GET /api/v1/boards/:slug/posts/:id/files - Get attached files from g5_board_file
		v1Boards.GET("/:slug/posts/:id/files", func(c *gin.Context) {
			slug := c.Param("slug")
			postID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid post ID"})
				return
			}

			// Get files from g5_board_file
			files, err := gnuFileRepo.GetFilesByPost(slug, postID)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
				return
			}

			// Build base URL for file paths
			baseURL := cfg.Storage.CDNURL
			if baseURL == "" {
				// Fallback to legacy PHP path
				baseURL = "https://damoang.net"
			}

			// Transform to response format
			var fileResponses []gnuboard.FileResponse
			for _, f := range files {
				fileResponses = append(fileResponses, f.ToFileResponse(baseURL))
			}

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    fileResponses,
			})
		})

		// GET /api/v1/boards/:slug/posts/:id/likers - Get users who liked the post
		v1Boards.GET("/:slug/posts/:id/likers", func(c *gin.Context) {
			slug := c.Param("slug")
			postID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid post ID"})
				return
			}

			page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
			limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
			if page < 1 {
				page = 1
			}
			if limit < 1 || limit > 100 {
				limit = 20
			}
			offset := (page - 1) * limit

			// LikerInfo struct for response
			type LikerInfo struct {
				MbID    string     `json:"mb_id"`
				MbName  string     `json:"mb_name"`
				MbNick  string     `json:"mb_nick"`
				BgIP    string     `json:"bg_ip,omitempty"`
				LikedAt *time.Time `json:"liked_at"`
			}

			var likers []LikerInfo
			var total int64

			if db == nil {
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"data": gin.H{
						"likers": []any{},
						"total":  0,
					},
				})
				return
			}

			// Count total likers
			db.Table("g5_board_good").
				Where("bo_table = ? AND wr_id = ? AND bg_flag = ?", slug, postID, "good").
				Count(&total)

			// Query likers with member info
			db.Table("g5_board_good bg").
				Select("bg.mb_id, COALESCE(m.mb_name, '') as mb_name, COALESCE(m.mb_nick, bg.mb_id) as mb_nick, bg.bg_ip, bg.bg_datetime as liked_at").
				Joins("LEFT JOIN g5_member m ON bg.mb_id = m.mb_id").
				Where("bg.bo_table = ? AND bg.wr_id = ? AND bg.bg_flag = ?", slug, postID, "good").
				Order("bg.bg_datetime DESC").
				Offset(offset).Limit(limit).
				Scan(&likers)

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"likers": likers,
					"total":  total,
				},
			})
		})

		// GET /api/v1/boards/:slug/posts/:id/revisions - Get post revision history (stub)
		v1Boards.GET("/:slug/posts/:id/revisions", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    []any{},
			})
		})

		// POST routes still use v2 handlers for now (will be migrated later)
		v1Boards.POST("/:slug/posts", middleware.JWTAuth(jwtManager), v2Handler.CreatePost)
		v1Boards.POST("/:slug/posts/:id/comments", middleware.JWTAuth(jwtManager), v2Handler.CreateComment)

		// PATCH /api/v1/boards/:slug/posts/:id/soft-delete - Soft delete post
		v1Boards.PATCH("/:slug/posts/:id/soft-delete", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			slug := c.Param("slug")
			postID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid post ID"})
				return
			}

			// Q&A 게시판 삭제 제한 체크
			if slug == "qa" {
				comments, err := gnuWriteRepo.FindComments(slug, postID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "댓글 조회 실패"})
					return
				}
				if len(comments) > 0 {
					c.JSON(http.StatusForbidden, gin.H{
						"success": false,
						"error":   "질문게시판은 답변이 있으면 삭제가 불가능합니다",
					})
					return
				}
			}

			// 게시글 조회 및 권한 확인 (삭제된 게시글 포함)
			post, err := gnuWriteRepo.FindPostByIDIncludeDeleted(slug, postID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "게시글을 찾을 수 없습니다"})
				return
			}

			// 이미 삭제된 경우
			if post.WrDeletedAt != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "이미 삭제된 게시글입니다"})
				return
			}

			// 작성자 또는 관리자 확인
			userID := middleware.GetUserID(c)
			userLevel := middleware.GetUserLevel(c)
			if post.MbID != userID && userLevel < 10 {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "삭제 권한이 없습니다"})
				return
			}

			// 게시글 소프트 삭제 (댓글도 함께 소프트 삭제됨)
			if err := gnuWriteRepo.SoftDeletePost(slug, postID, userID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "게시글 삭제 실패"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "삭제 완료"})
		})

		// POST /api/v1/boards/:slug/posts/:id/restore - Restore soft deleted post (admin only)
		v1Boards.POST("/:slug/posts/:id/restore", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			slug := c.Param("slug")
			postID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid post ID"})
				return
			}

			// 관리자 확인
			userLevel := middleware.GetUserLevel(c)
			if userLevel < 10 {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "관리자만 복구할 수 있습니다"})
				return
			}

			// 게시글 조회 (삭제된 게시글 포함)
			post, err := gnuWriteRepo.FindPostByIDIncludeDeleted(slug, postID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "게시글을 찾을 수 없습니다"})
				return
			}

			// 삭제되지 않은 경우
			if post.WrDeletedAt == nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "삭제된 게시글이 아닙니다"})
				return
			}

			// 게시글 복구
			if err := gnuWriteRepo.RestorePost(slug, postID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "게시글 복구 실패"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "복구 완료"})
		})

		// DELETE /api/v1/boards/:slug/posts/:id/permanent - Permanently delete post (admin only)
		v1Boards.DELETE("/:slug/posts/:id/permanent", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			slug := c.Param("slug")
			postID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid post ID"})
				return
			}

			// 관리자 확인
			userLevel := middleware.GetUserLevel(c)
			if userLevel < 10 {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "관리자만 영구 삭제할 수 있습니다"})
				return
			}

			// 게시글 조회 (삭제된 게시글 포함)
			_, err = gnuWriteRepo.FindPostByIDIncludeDeleted(slug, postID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "게시글을 찾을 수 없습니다"})
				return
			}

			// 게시글 영구 삭제
			if err := gnuWriteRepo.DeletePost(slug, postID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "게시글 영구 삭제 실패"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "영구 삭제 완료"})
		})

		// DELETE /api/v1/boards/:slug/posts/:id/comments/:comment_id - Soft delete comment
		v1Boards.DELETE("/:slug/posts/:id/comments/:comment_id", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			slug := c.Param("slug")
			commentID, err := strconv.Atoi(c.Param("comment_id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid comment ID"})
				return
			}

			// 댓글 조회
			comment, err := gnuWriteRepo.FindCommentByID(slug, commentID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "댓글을 찾을 수 없습니다"})
				return
			}

			// 작성자 또는 관리자 확인
			userID := middleware.GetUserID(c)
			userLevel := middleware.GetUserLevel(c)
			if comment.MbID != userID && userLevel < 10 {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "삭제 권한이 없습니다"})
				return
			}

			// 댓글 소프트 삭제
			if err := gnuWriteRepo.SoftDeleteComment(slug, commentID, userID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "댓글 삭제 실패"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "삭제 완료"})
		})

		// Board display settings (for admin layout switcher)
		v2DisplaySettingsRepo := v2repo.NewBoardDisplaySettingsRepository(db)
		displaySettingsHandler := v2handler.NewDisplaySettingsHandler(v2BoardRepo, v2DisplaySettingsRepo)
		v1Boards.GET("/:slug/display-settings", displaySettingsHandler.GetDisplaySettings)
		v1Boards.PUT("/:slug/display-settings", middleware.JWTAuth(jwtManager), displaySettingsHandler.UpdateDisplaySettings)

		// POST /api/v1/boards/:slug/posts/:id/move - Move post to another board (admin only)
		v1Boards.POST("/:slug/posts/:id/move", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			srcBoard := c.Param("slug")
			postID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid post ID"})
				return
			}

			// 관리자 확인
			userLevel := middleware.GetUserLevel(c)
			if userLevel < 10 {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "관리자만 게시글을 이동할 수 있습니다"})
				return
			}

			var req struct {
				TargetBoardID string `json:"target_board_id" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "target_board_id is required"})
				return
			}

			if srcBoard == req.TargetBoardID {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "같은 게시판으로는 이동할 수 없습니다"})
				return
			}

			// 원본 게시판/대상 게시판 존재 확인
			if !gnuBoardRepo.Exists(srcBoard) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "원본 게시판을 찾을 수 없습니다"})
				return
			}
			if !gnuBoardRepo.Exists(req.TargetBoardID) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "대상 게시판을 찾을 수 없습니다"})
				return
			}
			if !gnuWriteRepo.TableExists(req.TargetBoardID) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "대상 게시판 테이블이 존재하지 않습니다"})
				return
			}

			// 원본 게시글 조회 (전체 컬럼)
			post, err := gnuWriteRepo.FindPostByID(srcBoard, postID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "게시글을 찾을 수 없습니다"})
				return
			}

			// 댓글 조회
			comments, _ := gnuWriteRepo.FindCommentsIncludeDeleted(srcBoard, postID)

			// 대상 테이블에 새 wr_num 할당
			newWrNum, err := gnuWriteRepo.GetNextWrNum(req.TargetBoardID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "wr_num 생성 실패"})
				return
			}

			// 트랜잭션으로 이동 처리
			tx := db.Begin()

			// 1. 대상 테이블에 게시글 INSERT (새 ID로)
			newPost := *post
			newPost.WrID = 0 // auto increment
			newPost.WrNum = newWrNum
			if err := tx.Table(fmt.Sprintf("g5_write_%s", req.TargetBoardID)).Create(&newPost).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "게시글 이동 실패 (INSERT)"})
				return
			}

			// 2. 댓글도 이동
			for _, comment := range comments {
				newComment := *comment
				newComment.WrID = 0
				newComment.WrParent = newPost.WrID
				if err := tx.Table(fmt.Sprintf("g5_write_%s", req.TargetBoardID)).Create(&newComment).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "댓글 이동 실패"})
					return
				}
			}

			// 3. 첨부파일 이동 (g5_board_file의 bo_table 업데이트)
			tx.Table("g5_board_file").
				Where("bo_table = ? AND wr_id = ?", srcBoard, postID).
				Updates(map[string]interface{}{
					"bo_table": req.TargetBoardID,
					"wr_id":    newPost.WrID,
				})

			// 4. 원본 게시글 + 댓글 삭제
			tx.Table(fmt.Sprintf("g5_write_%s", srcBoard)).Where("wr_parent = ? AND wr_is_comment = 1", postID).Delete(&gnuboard.G5Write{})
			tx.Table(fmt.Sprintf("g5_write_%s", srcBoard)).Where("wr_id = ?", postID).Delete(&gnuboard.G5Write{})

			if err := tx.Commit().Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "이동 트랜잭션 커밋 실패"})
				return
			}

			// 캐시 무효화
			if cacheService != nil {
				ctx := c.Request.Context()
				_ = cacheService.InvalidateBoard(ctx, srcBoard)
				_ = cacheService.InvalidateBoard(ctx, req.TargetBoardID)
			}

			c.JSON(http.StatusOK, gin.H{
				"success":         true,
				"new_post_id":     newPost.WrID,
				"target_board_id": req.TargetBoardID,
			})
		})

		// GET /api/v1/board-groups - Get board groups with boards
		router.GET("/api/v1/board-groups", func(c *gin.Context) {
			type boardGroupRow struct {
				GrID      string `gorm:"column:gr_id"`
				GrSubject string `gorm:"column:gr_subject"`
				GrOrder   int    `gorm:"column:gr_order"`
			}
			var groups []boardGroupRow
			if err := db.Table("g5_board_group").Order("gr_order, gr_id").Find(&groups).Error; err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
				return
			}

			// 전체 게시판 조회
			boards, _ := gnuBoardRepo.FindAll()

			// 그룹별로 게시판 분류
			boardsByGroup := make(map[string][]gin.H)
			for _, b := range boards {
				boardsByGroup[b.GrID] = append(boardsByGroup[b.GrID], gin.H{
					"board_id": b.BoTable,
					"subject":  b.BoSubject,
				})
			}

			result := make([]gin.H, 0, len(groups))
			for _, g := range groups {
				groupBoards := boardsByGroup[g.GrID]
				if groupBoards == nil {
					groupBoards = []gin.H{}
				}
				result = append(result, gin.H{
					"id":         g.GrID,
					"name":       g.GrSubject,
					"sort_order": g.GrOrder,
					"is_visible": true,
					"boards":     groupBoards,
				})
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
		})

		router.GET("/api/v1/recommended/ai/:period", func(c *gin.Context) {
			emptySection := func(id, name string) gin.H {
				return gin.H{"id": id, "name": name, "group_id": "", "count": 0, "posts": []any{}}
			}
			c.JSON(http.StatusOK, gin.H{
				"generated_at": "",
				"period":       c.Param("period"),
				"period_hours": 0,
				"sections": gin.H{
					"community": emptySection("community", "커뮤니티"),
					"group":     emptySection("group", "소모임"),
					"info":      emptySection("info", "정보"),
				},
			})
		})

		// v2 Admin
		v2AdminSvc := v2svc.NewAdminService(v2UserRepo, v2BoardRepo, v2PostRepo, v2CommentRepo)
		v2AdminHandler := v2handler.NewAdminHandler(v2AdminSvc)
		v2routes.SetupAdmin(router, v2AdminHandler, jwtManager)

		// v2 Scrap, Memo, Block, Message
		v2ScrapRepo := v2repo.NewScrapRepository(db)
		v2MemoRepo := v2repo.NewMemoRepository(db)
		v2BlockRepo := v2repo.NewBlockRepository(db)
		v2MessageRepo := v2repo.NewMessageRepository(db)
		v2routes.SetupScrap(router, v2handler.NewScrapHandler(v2ScrapRepo), jwtManager)
		v2routes.SetupMemo(router, v2handler.NewMemoHandler(v2MemoRepo), jwtManager)
		v2routes.SetupBlock(router, v2handler.NewBlockHandler(v2BlockRepo), jwtManager)
		v2routes.SetupMessage(router, v2handler.NewMessageHandler(v2MessageRepo), jwtManager)

		// Banner, Promotion, License (v1 + v2 dual routes)
		bannerRepo := v2repo.NewBannerRepository(db)
		v2routes.SetupBanner(router, v2handler.NewBannerHandler(bannerRepo))

		promotionRepo := v2repo.NewPromotionRepository(db)
		v2routes.SetupPromotion(router, v2handler.NewPromotionHandler(promotionRepo))

		v2routes.SetupLicense(router, v2handler.NewLicenseHandler())

		// Installation API
		v2InstallHandler := v2handler.NewInstallHandler(db)
		v2routes.SetupInstall(router, v2InstallHandler)

		// Tenant Management
		tenantDBResolver := middleware.NewTenantDBResolver(db)
		tenantSvc := service.NewTenantService(siteRepo, db, tenantDBResolver)
		tenantHandler := handler.NewTenantHandler(tenantSvc)

		adminTenants := router.Group("/api/v2/admin/tenants")
		adminTenants.GET("", tenantHandler.ListTenants)
		adminTenants.GET("/plans", middleware.CacheWithTTL(redisClient, 10*time.Minute), tenantHandler.GetPlanLimits)
		adminTenants.GET("/:id", tenantHandler.GetTenant)
		adminTenants.POST("/:id/suspend", tenantHandler.SuspendTenant)
		adminTenants.POST("/:id/unsuspend", tenantHandler.UnsuspendTenant)
		adminTenants.PUT("/:id/plan", tenantHandler.ChangePlan)
		adminTenants.GET("/:id/usage", tenantHandler.GetUsage)

		// SaaS Provisioning
		subRepo := repository.NewSubscriptionRepository(db)
		if err := subRepo.AutoMigrate(); err != nil {
			log.Printf("warning: subscription AutoMigrate failed: %v", err)
		}
		provisioningSvc := service.NewProvisioningService(siteRepo, subRepo, tenantDBResolver, db, "angple.com")
		provisioningHandler := handler.NewProvisioningHandler(provisioningSvc)

		saas := router.Group("/api/v2/saas")
		saas.GET("/pricing", middleware.CacheWithTTL(redisClient, 10*time.Minute), provisioningHandler.GetPricing)
		saas.POST("/communities", provisioningHandler.ProvisionCommunity)
		saas.DELETE("/communities/:id", provisioningHandler.DeleteCommunity)
		saas.GET("/communities/:id/subscription", provisioningHandler.GetSubscription)
		saas.PUT("/communities/:id/subscription/plan", provisioningHandler.ChangePlan)
		saas.POST("/communities/:id/subscription/cancel", provisioningHandler.CancelSubscription)
		saas.GET("/communities/:id/invoices", provisioningHandler.GetInvoices)

		// OAuth2 Social Login
		oauthService := service.NewOAuthService(db, jwtManager)
		if clientID := os.Getenv("NAVER_CLIENT_ID"); clientID != "" {
			oauthService.RegisterProvider(domain.OAuthProviderNaver, &domain.OAuthConfig{
				ClientID:     clientID,
				ClientSecret: os.Getenv("NAVER_CLIENT_SECRET"),
				RedirectURL:  os.Getenv("NAVER_REDIRECT_URL"),
			})
		}
		if clientID := os.Getenv("KAKAO_CLIENT_ID"); clientID != "" {
			oauthService.RegisterProvider(domain.OAuthProviderKakao, &domain.OAuthConfig{
				ClientID:     clientID,
				ClientSecret: os.Getenv("KAKAO_CLIENT_SECRET"),
				RedirectURL:  os.Getenv("KAKAO_REDIRECT_URL"),
			})
		}
		if clientID := os.Getenv("GOOGLE_CLIENT_ID"); clientID != "" {
			oauthService.RegisterProvider(domain.OAuthProviderGoogle, &domain.OAuthConfig{
				ClientID:     clientID,
				ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
				RedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
				Scopes:       []string{"openid", "email", "profile"},
			})
		}
		oauthHandler := handler.NewOAuthHandler(oauthService)

		oauth := router.Group("/api/v2/auth/oauth")
		oauth.GET("/:provider", oauthHandler.Redirect)
		oauth.GET("/:provider/callback", oauthHandler.Callback)

		apiKeys := router.Group("/api/v2/auth/api-keys", middleware.JWTAuth(jwtManager))
		apiKeys.POST("", oauthHandler.GenerateAPIKey)

		// Elasticsearch Search (optional)
		if esClient != nil {
			searchSvc := service.NewSearchService(esClient, db)
			searchHandler := handler.NewSearchHandler(searchSvc)

			search := router.Group("/api/v2/search")
			search.GET("", searchHandler.Search)
			search.GET("/autocomplete", searchHandler.Autocomplete)

			adminSearch := router.Group("/api/v2/admin/search")
			adminSearch.POST("/index", searchHandler.BulkIndex)
			adminSearch.POST("/index-post", searchHandler.IndexPost)
			adminSearch.DELETE("/index/:board_id/:post_id", searchHandler.DeletePostIndex)
		}

		// Media Pipeline (S3 storage, optional)
		if s3Client != nil {
			mediaSvc := service.NewMediaService(s3Client)
			mediaHandler := handler.NewMediaHandler(mediaSvc)

			// TODO: UploadRateLimitConfig 구현 후 활성화
			// uploadRateLimit := middleware.RateLimit(redisClient, middleware.UploadRateLimitConfig())
			media := router.Group("/api/v2/media", middleware.JWTAuth(jwtManager))
			media.POST("/images", mediaHandler.UploadImage)
			media.POST("/attachments", mediaHandler.UploadAttachment)
			media.POST("/videos", mediaHandler.UploadVideo)
			media.DELETE("/files", mediaHandler.DeleteFile)
		}

		// WebSocket
		wsHandler := handler.NewWSHandler(wsHub, cfg.CORS.AllowOrigins)
		router.GET("/ws/notifications", middleware.JWTAuth(jwtManager), wsHandler.Connect)

		// ========================================
		// 투표/설문 (Poll) API — g5_poll / g5_poll_etc
		// ========================================

		pollGroup := router.Group("/api/v1/polls")
		pollGroup.Use(middleware.OptionalJWTAuth(jwtManager))

		// GET /api/v1/polls — 투표 목록
		pollGroup.GET("", func(c *gin.Context) {
			var polls []gnuboard.G5Poll
			query := db.Table("g5_poll").Order("po_id DESC")

			// 비관리자는 활성 투표만
			userLevel := middleware.GetUserLevel(c)
			if userLevel < 10 {
				query = query.Where("po_use = 1")
			}

			if err := query.Find(&polls).Error; err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
				return
			}

			currentUserID := middleware.GetUserID(c)
			result := make([]gnuboard.PollResponse, 0, len(polls))
			for _, p := range polls {
				hasVoted := currentUserID != "" && strings.Contains(p.MbIDs, currentUserID)
				result = append(result, p.ToPollResponse(hasVoted))
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
		})

		// GET /api/v1/polls/latest — 최신 활성 투표 1개 (위젯용)
		pollGroup.GET("/latest", func(c *gin.Context) {
			var poll gnuboard.G5Poll
			if err := db.Table("g5_poll").Where("po_use = 1").Order("po_id DESC").First(&poll).Error; err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
				return
			}

			currentUserID := middleware.GetUserID(c)
			hasVoted := currentUserID != "" && strings.Contains(poll.MbIDs, currentUserID)
			c.JSON(http.StatusOK, gin.H{"success": true, "data": poll.ToPollResponse(hasVoted)})
		})

		// GET /api/v1/polls/:id — 투표 상세 + 결과
		pollGroup.GET("/:id", func(c *gin.Context) {
			pollID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid poll ID"})
				return
			}

			var poll gnuboard.G5Poll
			if err := db.Table("g5_poll").Where("po_id = ?", pollID).First(&poll).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "투표를 찾을 수 없습니다"})
				return
			}

			currentUserID := middleware.GetUserID(c)
			hasVoted := currentUserID != "" && strings.Contains(poll.MbIDs, currentUserID)
			c.JSON(http.StatusOK, gin.H{"success": true, "data": poll.ToPollResponse(hasVoted)})
		})

		// POST /api/v1/polls/:id/vote — 투표 참여
		pollGroup.POST("/:id/vote", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			pollID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid poll ID"})
				return
			}

			var req struct {
				OptionIndex int `json:"option_index" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "option_index is required"})
				return
			}

			if req.OptionIndex < 1 || req.OptionIndex > 9 {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "option_index must be 1-9"})
				return
			}

			var poll gnuboard.G5Poll
			if err := db.Table("g5_poll").Where("po_id = ?", pollID).First(&poll).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "투표를 찾을 수 없습니다"})
				return
			}

			if poll.PoUse != 1 {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "종료된 투표입니다"})
				return
			}

			// 레벨 체크
			userLevel := middleware.GetUserLevel(c)
			if userLevel < poll.PoLevel {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": fmt.Sprintf("레벨 %d 이상만 투표할 수 있습니다", poll.PoLevel)})
				return
			}

			// 중복 투표 체크
			currentUserID := middleware.GetUserID(c)
			if strings.Contains(poll.MbIDs, currentUserID) {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "이미 투표에 참여하셨습니다"})
				return
			}

			// 투표 카운트 증가
			cntColumn := fmt.Sprintf("po_cnt%d", req.OptionIndex)
			newMbIDs := poll.MbIDs
			if newMbIDs != "" {
				newMbIDs += ","
			}
			newMbIDs += currentUserID

			if err := db.Table("g5_poll").Where("po_id = ?", pollID).Updates(map[string]interface{}{
				cntColumn: gorm.Expr(cntColumn+" + 1"),
				"mb_ids":  newMbIDs,
			}).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "투표 처리 실패"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "투표 완료"})
		})

		// Admin Poll Management
		adminPollGroup := router.Group("/api/v1/admin/polls")
		adminPollGroup.Use(middleware.JWTAuth(jwtManager), middleware.RequireAdmin())

		// POST /api/v1/admin/polls — 투표 생성
		adminPollGroup.POST("", func(c *gin.Context) {
			var req struct {
				Subject string   `json:"subject" binding:"required"`
				Options []string `json:"options" binding:"required"`
				Level   int      `json:"level"`
				Point   int      `json:"point"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "subject and options are required"})
				return
			}

			if len(req.Options) < 2 || len(req.Options) > 9 {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "options must be 2-9 items"})
				return
			}

			poll := gnuboard.G5Poll{
				PoSubject: req.Subject,
				PoLevel:   req.Level,
				PoPoint:   req.Point,
				PoDate:    time.Now().Format("2006-01-02"),
				PoUse:     1,
			}

			// 옵션 설정
			optionFields := []*string{&poll.PoPoll1, &poll.PoPoll2, &poll.PoPoll3, &poll.PoPoll4, &poll.PoPoll5, &poll.PoPoll6, &poll.PoPoll7, &poll.PoPoll8, &poll.PoPoll9}
			for i, opt := range req.Options {
				*optionFields[i] = opt
			}

			if err := db.Table("g5_poll").Create(&poll).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "투표 생성 실패"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "data": poll.ToPollResponse(false)})
		})

		// PUT /api/v1/admin/polls/:id — 투표 수정 (활성/비활성)
		adminPollGroup.PUT("/:id", func(c *gin.Context) {
			pollID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid poll ID"})
				return
			}

			var req struct {
				Subject  *string `json:"subject"`
				IsActive *bool   `json:"is_active"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "요청 형식 오류"})
				return
			}

			updates := map[string]interface{}{}
			if req.Subject != nil {
				updates["po_subject"] = *req.Subject
			}
			if req.IsActive != nil {
				if *req.IsActive {
					updates["po_use"] = 1
				} else {
					updates["po_use"] = 0
				}
			}

			if len(updates) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "수정할 내용이 없습니다"})
				return
			}

			if err := db.Table("g5_poll").Where("po_id = ?", pollID).Updates(updates).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "투표 수정 실패"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "수정 완료"})
		})

		// DELETE /api/v1/admin/polls/:id — 투표 삭제
		adminPollGroup.DELETE("/:id", func(c *gin.Context) {
			pollID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid poll ID"})
				return
			}

			// 관련 etc 삭제 후 poll 삭제
			db.Table("g5_poll_etc").Where("po_id = ?", pollID).Delete(nil)
			if err := db.Table("g5_poll").Where("po_id = ?", pollID).Delete(nil).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "투표 삭제 실패"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "삭제 완료"})
		})

		// Dantry (에러 리포트 - ClickHouse)
		if chHost := os.Getenv("CLICKHOUSE_HOST"); chHost != "" {
			chPort := 9000
			if p, err := strconv.Atoi(os.Getenv("CLICKHOUSE_PORT")); err == nil {
				chPort = p
			}
			chClient, chErr := dantry.NewClickHouseClient(dantry.ClientConfig{
				Host:     chHost,
				Port:     chPort,
				Database: "error_logs",
				User:     os.Getenv("CLICKHOUSE_USER"),
				Password: os.Getenv("CLICKHOUSE_PASSWORD"),
			})
			if chErr != nil {
				log.Printf("Dantry ClickHouse connection failed: %v (dantry disabled)", chErr)
			} else {
				dantryRepo := dantry.NewRepository(chClient)
				dantryHandler := dantry.NewHandler(dantryRepo)
				dantryGroup := router.Group("/api/v1/dantry", middleware.JWTAuth(jwtManager))
				dantryGroup.GET("/errors", dantryHandler.ListErrors)
				dantryGroup.GET("/errors/grouped", dantryHandler.ListGrouped)
				dantryGroup.GET("/errors/members", dantryHandler.GetMembersByMessage)
				dantryGroup.GET("/errors/:id", dantryHandler.GetByID)
				dantryGroup.GET("/stats", dantryHandler.GetStats)
				dantryGroup.GET("/timeseries", dantryHandler.GetTimeseries)
				log.Println("Dantry error viewer initialized")
			}
		}

		// Plugin System
		installRepo := pluginstoreRepo.NewInstallationRepository(db)
		settingRepo := pluginstoreRepo.NewSettingRepository(db)
		eventRepo := pluginstoreRepo.NewEventRepository(db)

		catalogSvc := pluginstoreSvc.NewCatalogService(installRepo)
		factories := plugin.GetRegisteredFactories()
		for _, reg := range factories {
			catalogSvc.RegisterManifest(reg.Manifest)
		}

		permRepo := pluginstoreRepo.NewPermissionRepository(db)
		storeSvc := pluginstoreSvc.NewStoreService(installRepo, eventRepo, settingRepo, catalogSvc, pluginLogger)
		settingSvc := pluginstoreSvc.NewSettingService(settingRepo, eventRepo, catalogSvc)
		permSvc := pluginstoreSvc.NewPermissionService(permRepo, catalogSvc)

		pluginManager := plugin.NewManager("plugins", db, redisClient, pluginLogger, settingSvc, permSvc)
		pluginManager.GetRegistry().SetRouter(router)
		pluginManager.GetRegistry().SetJWTVerifier(plugin.NewDefaultJWTVerifier(
			func(token string) (string, string, int, error) {
				claims, err := jwtManager.VerifyToken(token)
				if err != nil {
					return "", "", 0, err
				}
				return claims.UserID, claims.Nickname, claims.Level, nil
			},
		))

		settingSvc.SetReloader(pluginManager)
		pluginManager.SetJWTManager(jwtManager)
		if err := pluginManager.RegisterAllFactories(); err != nil {
			pkglogger.Info("Failed to register plugin factories: %v", err)
		}
		if err := storeSvc.BootEnabledPlugins(pluginManager); err != nil {
			pkglogger.Info("Failed to boot enabled plugins: %v", err)
		}

		storeHandler := pluginstoreHandler.NewStoreHandler(storeSvc, catalogSvc, pluginManager)
		settingHandler := pluginstoreHandler.NewSettingHandler(settingSvc, pluginManager)
		permHandler := pluginstoreHandler.NewPermissionHandler(permSvc)

		adminPlugins := router.Group("/api/v2/admin/plugins")
		adminPlugins.Use(middleware.JWTAuth(jwtManager), middleware.RequireAdmin())
		{
			adminPlugins.GET("", storeHandler.ListPlugins)
			adminPlugins.GET("/dashboard", storeHandler.Dashboard)
			adminPlugins.GET("/health", storeHandler.HealthCheck)
			adminPlugins.GET("/schedules", storeHandler.ScheduledTasks)
			adminPlugins.GET("/rate-limits", storeHandler.RateLimitConfigs)
			adminPlugins.GET("/metrics", storeHandler.PluginMetrics)
			adminPlugins.GET("/event-subscriptions", storeHandler.EventSubscriptions)
			adminPlugins.GET("/overview", storeHandler.PluginOverview)
			adminPlugins.GET("/settings/export", settingHandler.ExportAllSettings)
			adminPlugins.POST("/settings/import", settingHandler.ImportSettings)
			adminPlugins.GET("/:name", storeHandler.GetPlugin)
			adminPlugins.POST("/:name/install", storeHandler.InstallPlugin)
			adminPlugins.POST("/:name/enable", storeHandler.EnablePlugin)
			adminPlugins.POST("/:name/disable", storeHandler.DisablePlugin)
			adminPlugins.DELETE("/:name", storeHandler.UninstallPlugin)
			adminPlugins.GET("/:name/settings", settingHandler.GetSettings)
			adminPlugins.PUT("/:name/settings", settingHandler.SaveSettings)
			adminPlugins.GET("/:name/settings/export", settingHandler.ExportSettings)
			adminPlugins.GET("/:name/events", storeHandler.GetEvents)
			adminPlugins.GET("/:name/permissions", permHandler.GetPermissions)
			adminPlugins.PUT("/:name/permissions/:permId", permHandler.UpdatePermission)
			adminPlugins.GET("/:name/health", storeHandler.HealthCheckSingle)
			adminPlugins.GET("/:name/metrics", storeHandler.PluginMetricsSingle)
			adminPlugins.GET("/:name/detail", storeHandler.PluginDetail)
		}

		// Marketplace
		marketplaceRepo := pluginstoreRepo.NewMarketplaceRepository(db)
		if err := marketplaceRepo.AutoMigrate(); err != nil {
			pkglogger.Info("Marketplace migration warning: %v", err)
		}
		marketplaceSvc := pluginstoreSvc.NewMarketplaceService(marketplaceRepo)
		marketplaceHandler := pluginstoreHandler.NewMarketplaceHandler(marketplaceSvc)

		mp := router.Group("/api/v2/marketplace")
		mp.GET("", marketplaceHandler.Browse)
		mp.GET("/:name", marketplaceHandler.GetPlugin)
		mp.GET("/:name/reviews", marketplaceHandler.GetReviews)
		mp.POST("/:name/reviews", marketplaceHandler.AddReview)
		mp.POST("/:name/download", marketplaceHandler.TrackDownload)

		mpDev := router.Group("/api/v2/marketplace/developers")
		mpDev.POST("/register", marketplaceHandler.RegisterDeveloper)
		mpDev.GET("/me", marketplaceHandler.GetMyProfile)
		mpDev.POST("/submissions", marketplaceHandler.SubmitPlugin)
		mpDev.GET("/submissions", marketplaceHandler.ListMySubmissions)

		mpAdmin := router.Group("/api/v2/admin/marketplace")
		mpAdmin.GET("/submissions/pending", marketplaceHandler.ListPendingSubmissions)
		mpAdmin.POST("/submissions/:id/review", marketplaceHandler.ReviewSubmission)

		pluginManager.StartScheduler()
		pkglogger.Info("Plugin Store & Marketplace initialized")
	} else {
		pkglogger.Info("Skipping API route setup (no DB connection)")
	}

	// v1 API catch-all: 미매핑 v1 엔드포인트에 대해 404 대신 빈 성공 응답 반환
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/") {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})

	// 서버 시작
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	pkglogger.Info("Server listening on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// splitAndTrim splits a string by delimiter and trims spaces
func splitAndTrim(s string, delimiter string) []string {
	parts := []string{}
	for _, part := range splitString(s, delimiter) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func splitString(s string, delimiter string) []string {
	result := []string{}
	current := ""
	for _, char := range s {
		if string(char) == delimiter {
			result = append(result, current)
			current = ""
		} else {
			current += string(char)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n') {
		end--
	}

	return s[start:end]
}

// initDB MySQL 연결 초기화
func initDB(cfg *config.Config) (*gorm.DB, error) {
	mysqlCfg, err := mysqldriver.ParseDSN(cfg.Database.GetDSN())
	if err != nil {
		return nil, fmt.Errorf("DSN 파싱 실패: %w", err)
	}
	if mysqlCfg.Params == nil {
		mysqlCfg.Params = map[string]string{}
	}
	mysqlCfg.Params["time_zone"] = "'+09:00'"

	db, err := gorm.Open(mysql.Open(mysqlCfg.FormatDSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Info),
	})
	if err != nil {
		return nil, err
	}

	db.Exec("SET SESSION sql_mode = ''")
	db.Exec("SET NAMES utf8mb4")
	db.Exec("SET CHARACTER SET utf8mb4")
	db.Exec("SET character_set_connection=utf8mb4")

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetime) * time.Second)

	return db, nil
}
