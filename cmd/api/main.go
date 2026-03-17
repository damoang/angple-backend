package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/config"
	"github.com/damoang/angple-backend/internal/cron"
	"github.com/damoang/angple-backend/internal/domain"
	gnuboard "github.com/damoang/angple-backend/internal/domain/gnuboard"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
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
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	v2routes "github.com/damoang/angple-backend/internal/routes/v2"
	"github.com/damoang/angple-backend/internal/service"
	v2svc "github.com/damoang/angple-backend/internal/service/v2"
	"github.com/damoang/angple-backend/internal/worker"
	"github.com/damoang/angple-backend/internal/ws"
	pkgcache "github.com/damoang/angple-backend/pkg/cache"
	pkges "github.com/damoang/angple-backend/pkg/elasticsearch"
	"github.com/damoang/angple-backend/pkg/i18n"
	"github.com/damoang/angple-backend/pkg/jwt"
	pkglogger "github.com/damoang/angple-backend/pkg/logger"
	pkgredis "github.com/damoang/angple-backend/pkg/redis"
	pkgsphinx "github.com/damoang/angple-backend/pkg/sphinx"
	pkgstorage "github.com/damoang/angple-backend/pkg/storage"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// memCachedPosts holds parsed post data in memory for fast filtering.
type memCachedPosts struct {
	items     []map[string]any // parsed post items (for filtering)
	jsonBytes []byte           // pre-serialized full response JSON (for zero-block users)
	meta      gin.H            // meta object for response reconstruction
	expiresAt time.Time
}

var postMemCache sync.Map // key: "posts:{boardID}:{page}:{limit}"

// filterItems filters blocked users' posts from parsed items slice, preserving notice posts.
func filterItems(items []map[string]any, blockedIDs []string) []map[string]any {
	blockedSet := make(map[string]struct{}, len(blockedIDs))
	for _, id := range blockedIDs {
		blockedSet[id] = struct{}{}
	}
	filtered := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if isNotice, _ := item["is_notice"].(bool); isNotice {
			filtered = append(filtered, item)
			continue
		}
		if authorID, ok := item["author_id"].(string); ok {
			if _, blocked := blockedSet[authorID]; blocked {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
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

	// MySQL 연결 (DNS 실패 등 일시적 장애에 대비하여 최대 5회 재시도)
	var db *gorm.DB
	for attempt := 1; attempt <= 5; attempt++ {
		db, err = initDB(cfg)
		if err == nil {
			break
		}
		pkglogger.Info("DB connection attempt %d/5 failed: %v", attempt, err)
		if attempt < 5 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second) // 2s, 4s, 6s, 8s backoff
		}
	}
	if err != nil {
		appEnv := os.Getenv("APP_ENV")
		if appEnv == "production" || appEnv == "staging" {
			log.Fatalf("CRITICAL: All DB connection attempts failed: %v (aborting — DB required in %s)", err, appEnv)
		}
		pkglogger.Info("CRITICAL: All DB connection attempts failed: %v (continuing without DB)", err)
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

	// IP Protection
	ipProtectCfg := middleware.LoadIPProtectionConfig()

	// Ban check middleware (제재 회원 글/댓글 작성 차단)
	banCheck := middleware.BanCheck(db)

	// Plugin HookManager
	pluginLogger := plugin.NewDefaultLogger("plugin")
	_ = plugin.NewHookManager(pluginLogger)

	// Gin 라우터 생성
	router := gin.Default()
	router.TrustedPlatform = "CF-Connecting-IP"

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

	// Health Check (DB ping 포함 — 커넥션 죽으면 K8s가 파드 재시작)
	router.GET("/health", func(c *gin.Context) {
		if db == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "error",
				"service": "angple-backend",
				"error":   "db not initialized",
				"time":    time.Now().Unix(),
			})
			return
		}
		sqlDB, err := db.DB()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "error",
				"service": "angple-backend",
				"error":   "db connection pool error",
				"time":    time.Now().Unix(),
			})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			// Stale connection 정리: idle 연결을 모두 닫아 다음 요청에서 새 연결 생성 유도
			sqlDB.SetMaxIdleConns(0)
			sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "error",
				"service": "angple-backend",
				"error":   "db ping failed",
				"time":    time.Now().Unix(),
			})
			return
		}
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

		// Sphinx full-text search (SphinxQL on port 9306)
		sphinxHost := os.Getenv("SPHINX_HOST")
		if sphinxHost == "" {
			sphinxHost = "127.0.0.1"
		}
		var sphinxClient *pkgsphinx.Client
		if sc, err := pkgsphinx.New(sphinxHost, 9306); err != nil {
			pkglogger.Info("Sphinx unavailable, falling back to MySQL LIKE: %v", err)
		} else {
			sphinxClient = sc
			pkglogger.Info("Sphinx connected (%s:9306)", sphinxHost)
		}

		// Gnuboard repositories for v1 API (g5_* tables)
		gnuBoardRepo := gnurepo.NewBoardRepository(db)
		var gnuWriteRepo gnurepo.WriteRepository
		if sphinxClient != nil && redisClient != nil {
			gnuWriteRepo = gnurepo.NewWriteRepositoryFull(db, sphinxClient, redisClient)
		} else if sphinxClient != nil {
			gnuWriteRepo = gnurepo.NewWriteRepositoryWithSphinx(db, sphinxClient)
		} else if redisClient != nil {
			gnuWriteRepo = gnurepo.NewWriteRepositoryWithRedis(db, redisClient)
		} else {
			gnuWriteRepo = gnurepo.NewWriteRepository(db)
		}
		gnuFileRepo := gnurepo.NewFileRepository(db)
		gnuTagRepo := gnurepo.NewTagRepository(db)
		gnuMemberRepo := gnurepo.NewMemberRepository(db)
		scheduledDeleteRepo := gnurepo.NewScheduledDeleteRepository(db)

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
		v2Handler.SetNotiRepository(gnurepo.NewNotiRepository(db))
		v2Handler.SetNotiPreferenceRepository(gnurepo.NewNotiPreferenceRepository(db))
		v2Handler.SetGnuDB(db)
		v2Handler.SetBlockRepository(v2repo.NewBlockRepository(db))

		// XP: DI into V2Handler (set after expRepo is created below)

		v2routes.Setup(router, v2Handler, jwtManager, permChecker, db)
		v2routes.SetupAdminPosts(router, v2Handler, jwtManager)

		// MyPage routes (point, exp, posts, comments, stats)
		v2ExpRepo := v2repo.NewExpRepository(db)
		gnuPointRepo := v2repo.NewGnuboardPointRepository(db)
		pointHandler := v2handler.NewPointHandler(gnuPointRepo)
		expHandler := v2handler.NewExpHandler(v2ExpRepo)
		expHandler.SetNotiRepository(gnurepo.NewNotiRepository(db))

		// Point config + write repos
		pointConfigRepo := v2repo.NewPointConfigRepository(db)
		gnuPointWriteRepo := v2repo.NewGnuboardPointWriteRepository(db)
		expHandler.SetPointConfigRepository(pointConfigRepo)
		v2Handler.SetGnuboardPointWriteRepository(gnuPointWriteRepo)
		v2Handler.SetPointConfigRepository(pointConfigRepo)

		// Inject expRepo into V2Handler for write/comment XP
		v2Handler.SetExpRepository(v2ExpRepo)
		myPageRepo := gnurepo.NewMyPageRepository(db, gnuBoardRepo)
		myPageHandler := handler.NewMyPageHandler(myPageRepo)
		if redisClient != nil {
			myPageHandler.SetRedisClient(redisClient)
		}
		v2routes.SetupMyPage(router, pointHandler, expHandler, myPageHandler, jwtManager)
		v2routes.SetupMemberActivity(router, myPageHandler)

		// Admin XP + Point config management routes
		v2routes.SetupAdminXP(router, expHandler, jwtManager)
		v2routes.SetupAdminPoint(router, expHandler, jwtManager)

		// DisciplineLog routes (uses gnuboard g5_write_disciplinelog table)
		disciplineLogHandler := v2handler.NewDisciplineLogHandler(gnuWriteRepo, db)
		v2routes.SetupDisciplineLog(router, disciplineLogHandler, jwtManager)

		// v2 Auth (with ExpRepo for daily login XP + auto-promotion on login)
		v2AuthSvc := v2svc.NewV2AuthService(v2UserRepo, jwtManager, v2ExpRepo)
		v2AuthSvc.SetPromotionDeps(db, gnurepo.NewNotiRepository(db))
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
			if err := db.Where("is_active = ? AND (show_in_sidebar = ? OR show_in_header = ?)", true, true, true).
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

			// Admin sees full (unmasked) IP
			if middleware.GetUserLevel(c) >= 10 {
				v1handler.OverrideIPForAdmin(items, notices)
			}

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
		// v1 notifications (g5_na_noti)
		notiRepo := gnurepo.NewNotiRepository(db)
		notiPrefRepo := gnurepo.NewNotiPreferenceRepository(db)
		notiHandler := handler.NewNotiHandler(notiRepo, notiPrefRepo)
		notiGroup := router.Group("/api/v1/notifications", middleware.JWTAuth(jwtManager))
		notiGroup.GET("/unread-count", notiHandler.GetUnreadCount)
		notiGroup.GET("", notiHandler.GetNotifications)
		notiGroup.GET("/grouped", notiHandler.GetGroupedNotifications)
		notiGroup.GET("/preferences", notiHandler.GetPreferences)
		notiGroup.PUT("/preferences", notiHandler.UpdatePreferences)
		notiGroup.POST("/:id/read", notiHandler.MarkAsRead)
		notiGroup.POST("/read-all", notiHandler.MarkAllAsRead)
		notiGroup.POST("/group/read", notiHandler.MarkGroupAsRead)
		notiGroup.DELETE("/:id", notiHandler.Delete)
		notiGroup.DELETE("/group", notiHandler.DeleteGroup)
		// v1 members memo — 회원이 다른 회원에 대해 남긴 메모 (g5_member_memo)
		memberMemoGroup := router.Group("/api/v1/members/:id/memo")
		memberMemoGroup.Use(middleware.JWTAuth(jwtManager))

		// GET /api/v1/members/:id/memo — 메모 조회
		memberMemoGroup.GET("", func(c *gin.Context) {
			currentUserID := middleware.GetUserID(c)
			targetID := c.Param("id")

			type memoRow struct {
				ID         int     `gorm:"column:id" json:"id"`
				MemberID   string  `gorm:"column:member_id" json:"member_id"`
				TargetID   string  `gorm:"column:target_member_id" json:"target_id"`
				Memo       string  `gorm:"column:memo" json:"content"`
				MemoDetail *string `gorm:"column:memo_detail" json:"memo_detail"`
				Color      string  `gorm:"column:color" json:"color"`
				UpdatedAt  *string `gorm:"column:updated_at" json:"updated_at"`
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

		// POST /api/v1/members/:id/memo — 메모 생성/수정 (upsert)
		memberMemoGroup.POST("", func(c *gin.Context) {
			currentUserID := middleware.GetUserID(c)
			targetID := c.Param("id")

			var req struct {
				Content    string `json:"content" binding:"required"`
				MemoDetail string `json:"memo_detail"`
				Color      string `json:"color"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "content is required"})
				return
			}

			color := req.Color
			if color == "" {
				color = "yellow"
			}

			memo := map[string]interface{}{
				"member_id":        currentUserID,
				"target_member_id": targetID,
				"memo":             req.Content,
				"memo_detail":      req.MemoDetail,
				"color":            color,
			}
			// Try INSERT, on duplicate key UPDATE
			err := db.Table("g5_member_memo").Create(memo).Error
			if err != nil {
				// Duplicate entry — update instead
				err = db.Table("g5_member_memo").
					Where("member_id = ? AND target_member_id = ?", currentUserID, targetID).
					Updates(map[string]interface{}{
						"memo":        req.Content,
						"memo_detail": req.MemoDetail,
						"color":       color,
					}).Error
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to save memo"})
					return
				}
			}

			// Return saved memo
			type memoRow struct {
				ID         int     `gorm:"column:id" json:"id"`
				MemberID   string  `gorm:"column:member_id" json:"member_id"`
				TargetID   string  `gorm:"column:target_member_id" json:"target_id"`
				Memo       string  `gorm:"column:memo" json:"content"`
				MemoDetail *string `gorm:"column:memo_detail" json:"memo_detail"`
				Color      string  `gorm:"column:color" json:"color"`
				UpdatedAt  *string `gorm:"column:updated_at" json:"updated_at"`
			}
			var saved memoRow
			db.Table("g5_member_memo").
				Where("member_id = ? AND target_member_id = ?", currentUserID, targetID).
				First(&saved)
			c.JSON(http.StatusOK, gin.H{"success": true, "data": saved})
		})

		// PUT /api/v1/members/:id/memo — 메모 수정 (POST와 동일한 upsert)
		memberMemoGroup.PUT("", func(c *gin.Context) {
			currentUserID := middleware.GetUserID(c)
			targetID := c.Param("id")

			var req struct {
				Content    string `json:"content" binding:"required"`
				MemoDetail string `json:"memo_detail"`
				Color      string `json:"color"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "content is required"})
				return
			}

			color := req.Color
			if color == "" {
				color = "yellow"
			}

			err := db.Table("g5_member_memo").
				Where("member_id = ? AND target_member_id = ?", currentUserID, targetID).
				Updates(map[string]interface{}{
					"memo":        req.Content,
					"memo_detail": req.MemoDetail,
					"color":       color,
				}).Error
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to update memo"})
				return
			}

			type memoRow struct {
				ID         int     `gorm:"column:id" json:"id"`
				MemberID   string  `gorm:"column:member_id" json:"member_id"`
				TargetID   string  `gorm:"column:target_member_id" json:"target_id"`
				Memo       string  `gorm:"column:memo" json:"content"`
				MemoDetail *string `gorm:"column:memo_detail" json:"memo_detail"`
				Color      string  `gorm:"column:color" json:"color"`
				UpdatedAt  *string `gorm:"column:updated_at" json:"updated_at"`
			}
			var saved memoRow
			db.Table("g5_member_memo").
				Where("member_id = ? AND target_member_id = ?", currentUserID, targetID).
				First(&saved)
			c.JSON(http.StatusOK, gin.H{"success": true, "data": saved})
		})

		// DELETE /api/v1/members/:id/memo — 메모 삭제
		memberMemoGroup.DELETE("", func(c *gin.Context) {
			currentUserID := middleware.GetUserID(c)
			targetID := c.Param("id")

			err := db.Table("g5_member_memo").
				Where("member_id = ? AND target_member_id = ?", currentUserID, targetID).
				Delete(nil).Error
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to delete memo"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		})

		// GET /api/v1/members/batch/memo?ids=user1,user2,... — 배치 메모 조회
		router.GET("/api/v1/members/batch/memo", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			currentUserID := middleware.GetUserID(c)
			idsParam := c.Query("ids")
			if idsParam == "" {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": map[string]interface{}{}})
				return
			}

			ids := strings.Split(idsParam, ",")
			if len(ids) > 100 {
				ids = ids[:100]
			}

			type memoRow struct {
				TargetID   string  `gorm:"column:target_member_id" json:"target_id"`
				Memo       string  `gorm:"column:memo" json:"content"`
				MemoDetail *string `gorm:"column:memo_detail" json:"memo_detail"`
				Color      string  `gorm:"column:color" json:"color"`
			}
			var memos []memoRow
			db.Table("g5_member_memo").
				Select("target_member_id, memo, memo_detail, color").
				Where("member_id = ? AND target_member_id IN ?", currentUserID, ids).
				Find(&memos)

			result := make(map[string]interface{}, len(memos))
			for _, m := range memos {
				if m.Memo != "" {
					result[m.TargetID] = m
				}
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
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
		// Block repository (used for filtering blocked users' posts/comments)
		blockRepo := v2repo.NewBlockRepository(db)

		// getBlockedIDs returns blocked user IDs with Redis cache (5 min TTL).
		// Empty results are also cached to avoid repeated DB queries for users with no blocks.
		getBlockedIDs := func(ctx context.Context, userID string) []string {
			if userID == "" {
				return nil
			}
			cacheKey := "block:" + userID
			if cacheService != nil {
				var ids []string
				if err := cacheService.Get(ctx, cacheKey, &ids); err == nil {
					if len(ids) == 0 {
						return nil
					}
					return ids
				}
			}
			ids, err := blockRepo.GetBlockedUserIDs(userID)
			if err != nil {
				return nil
			}
			// Cache even empty results to prevent repeated DB queries
			if cacheService != nil {
				_ = cacheService.Set(ctx, cacheKey, ids, 5*time.Minute)
			}
			if len(ids) == 0 {
				return nil
			}
			return ids
		}

		// v1 block routes
		v1Members := router.Group("/api/v1/members")
		v1Members.POST("/:id/block", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			userID := middleware.GetUserID(c)
			targetID := c.Param("id")
			if userID == targetID {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "자기 자신을 차단할 수 없습니다"})
				return
			}
			exists, err := blockRepo.Exists(userID, targetID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "차단 확인 실패"})
				return
			}
			if exists {
				c.JSON(http.StatusConflict, gin.H{"success": false, "error": "이미 차단한 회원입니다"})
				return
			}
			if _, err := blockRepo.Create(userID, targetID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "차단 실패"})
				return
			}
			if cacheService != nil {
				_ = cacheService.Delete(c.Request.Context(), "block:"+userID)
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "차단 완료"})
		})
		v1Members.DELETE("/:id/block", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			userID := middleware.GetUserID(c)
			targetID := c.Param("id")
			if err := blockRepo.Delete(userID, targetID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
				return
			}
			if cacheService != nil {
				_ = cacheService.Delete(c.Request.Context(), "block:"+userID)
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "차단 해제 완료"})
		})
		router.GET("/api/v1/my/blocked", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			userID := middleware.GetUserID(c)
			ids, err := blockRepo.GetBlockedUserIDs(userID)
			if err != nil || len(ids) == 0 {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
				return
			}
			// Batch query for all blocked user nicks (N+1 → 1 query)
			type memberNick struct {
				MbID string `gorm:"column:mb_id"`
				Nick string `gorm:"column:mb_nick"`
			}
			var members []memberNick
			db.Table("g5_member").Select("mb_id, mb_nick").Where("mb_id IN ?", ids).Find(&members)
			nickMap := make(map[string]string, len(members))
			for _, m := range members {
				nickMap[m.MbID] = m.Nick
			}

			type blockedItem struct {
				MbID      string `json:"mb_id"`
				MbName    string `json:"mb_name"`
				BlockedAt string `json:"blocked_at"`
			}
			items := make([]blockedItem, 0, len(ids))
			for _, id := range ids {
				items = append(items, blockedItem{MbID: id, MbName: nickMap[id]})
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
		})

		// v1 boards routes → use Gnuboard g5_* tables
		v1Boards := router.Group("/api/v1/boards")
		v1Boards.Use(middleware.OptionalJWTAuth(jwtManager))
		v1Boards.Use(middleware.ArchiveBoardCheck())
		v1Boards.Use(middleware.APICacheControl(10)) // 브라우저 캐시 10초

		// Board extended settings repo & write restriction service (used by multiple handlers)
		v2ExtendedSettingsRepo := v2repo.NewBoardExtendedSettingsRepository(db)
		writeRestrictionSvc := service.NewBoardWriteRestrictionService(db, v2ExtendedSettingsRepo)

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
			cursorWrNum, cursorNumErr := strconv.Atoi(c.Query("cursor_wr_num"))
			cursorWrReply := c.Query("cursor_wr_reply")
			useCursor := cursorNumErr == nil && cursorWrReply != ""

			// Search parameters
			sfl := c.Query("sfl")           // search field: title, content, title_content, author
			stx := c.Query("stx")           // search text
			category := c.Query("category") // category filter (ca_name)
			isSearching := sfl != "" && stx != ""

			// Get blocked user IDs (skip for admins)
			var blockedIDs []string
			if middleware.GetUserLevel(c) < 10 {
				blockedIDs = getBlockedIDs(ctx, middleware.GetUserID(c))
			}

			// --- 2-layer cache: in-memory → Redis → DB ---
			memKey := fmt.Sprintf("posts:%s:%d:%d", slug, page, limit)
			if category != "" {
				memKey = fmt.Sprintf("posts:%s:%d:%d:cat:%s", slug, page, limit, category)
			}
			if useCursor {
				memKey = fmt.Sprintf("posts:%s:cursor:%d:%s:%d", slug, cursorWrNum, cursorWrReply, limit)
			}

			if !isSearching && !useCursor && category == "" {
				// Layer 1: In-memory cache (30s TTL)
				if cached, ok := postMemCache.Load(memKey); ok {
					mc := cached.(*memCachedPosts)
					if time.Now().Before(mc.expiresAt) {
						c.Header("X-Cache", "HIT")
						if len(blockedIDs) == 0 {
							// Zero blocks: return pre-serialized JSON (zero parsing)
							c.Data(http.StatusOK, "application/json", mc.jsonBytes)
							return
						}
						// Has blocks: filter from parsed items, marshal once
						filtered := filterItems(mc.items, blockedIDs)
						c.JSON(http.StatusOK, gin.H{"success": true, "data": filtered, "meta": mc.meta})
						return
					}
					// Expired, remove from cache
					postMemCache.Delete(memKey)
				}

				// Layer 2: Redis cache
				if cacheService != nil {
					if cached, err := cacheService.GetPosts(ctx, slug, page, limit); err == nil {
						// Parse Redis JSON once → store in memory
						var parsed struct {
							Success bool             `json:"success"`
							Data    []map[string]any `json:"data"`
							Meta    map[string]any   `json:"meta"`
						}
						if json.Unmarshal(cached, &parsed) == nil && parsed.Data != nil {
							mc := &memCachedPosts{
								items:     parsed.Data,
								jsonBytes: cached,
								meta:      parsed.Meta,
								expiresAt: time.Now().Add(30 * time.Second),
							}
							postMemCache.Store(memKey, mc)

							c.Header("X-Cache", "HIT")
							if len(blockedIDs) == 0 {
								c.Data(http.StatusOK, "application/json", cached)
								return
							}
							filtered := filterItems(parsed.Data, blockedIDs)
							c.JSON(http.StatusOK, gin.H{"success": true, "data": filtered, "meta": parsed.Meta})
							return
						}
						// Parse failed, fall through to DB
					}
				}
			}

			// Layer 3: DB query
			board, err := gnuBoardRepo.FindByID(slug)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}, "meta": gin.H{"total": 0, "page": 1, "limit": 20}})
				return
			}

			var posts []*gnuboard.G5Write
			var total int64
			if isSearching && category != "" {
				posts, total, err = gnuWriteRepo.SearchPostsByCategory(slug, sfl, stx, category, page, limit)
			} else if isSearching {
				posts, total, err = gnuWriteRepo.SearchPosts(slug, sfl, stx, page, limit)
			} else if useCursor {
				posts, total, err = gnuWriteRepo.FindPostsAfter(slug, limit, cursorWrNum, cursorWrReply)
			} else if category != "" {
				posts, total, err = gnuWriteRepo.FindPostsByCategory(slug, category, page, limit)
			} else {
				posts, total, err = gnuWriteRepo.FindPosts(slug, page, limit)
			}
			if err != nil {
				if isSearching {
					c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": err.Error()})
				} else {
					c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}, "meta": gin.H{"total": 0, "page": page, "limit": limit}})
				}
				return
			}

			// Build notice ID map from board settings
			noticeIDs := gnurepo.ParseNoticeIDs(board.BoNotice)
			noticeIDMap := v1handler.BuildNoticeIDMap(noticeIDs)

			// Transform to v1 format
			items := v1handler.TransformToV1Posts(posts, noticeIDMap)

			// Admin sees full (unmasked) IP
			if middleware.GetUserLevel(c) >= 10 {
				v1handler.OverrideIPForAdmin(items, posts)
			}

			// Enrich thumbnails from g5_board_file for posts that have files but no thumbnail
			var needFileIDs []int
			for _, item := range items {
				if _, ok := item["thumbnail"]; !ok {
					if hasFile, _ := item["has_file"].(bool); hasFile {
						if id, ok := item["id"].(int); ok {
							needFileIDs = append(needFileIDs, id)
						}
					}
				}
			}
			if len(needFileIDs) > 0 {
				if fileImages, err := gnuFileRepo.GetFirstImagesByPostIDs(slug, needFileIDs); err == nil && len(fileImages) > 0 {
					cdnURL := strings.TrimRight(cfg.Storage.CDNURL, "/")
					for i, item := range items {
						if _, ok := item["thumbnail"]; !ok {
							if id, ok := item["id"].(int); ok {
								if fname, ok := fileImages[id]; ok {
									if cdnURL != "" {
										items[i]["thumbnail"] = cdnURL + "/data/file/" + slug + "/" + fname
									} else {
										items[i]["thumbnail"] = "data/file/" + slug + "/" + fname
									}
								}
							}
						}
					}
				}
			}

			meta := gin.H{"board_id": slug, "page": page, "limit": limit, "total": total}
			if useCursor && len(posts) > 0 {
				last := posts[len(posts)-1]
				meta["next_cursor_wr_num"] = last.WrNum
				meta["next_cursor_wr_reply"] = last.WrReply
			}
			response := gin.H{
				"success": true,
				"data":    items,
				"meta":    meta,
			}

			// Store in both caches (only for non-search, non-category requests, unfiltered data)
			if !isSearching && !useCursor && category == "" {
				if cacheService != nil {
					_ = cacheService.SetPosts(ctx, slug, page, limit, response)
				}
				// Pre-serialize for in-memory cache
				if jsonBytes, err := json.Marshal(response); err == nil {
					mc := &memCachedPosts{
						items:     items,
						jsonBytes: jsonBytes,
						meta:      meta,
						expiresAt: time.Now().Add(30 * time.Second),
					}
					postMemCache.Store(memKey, mc)
				}
			}

			// Filter blocked users from response (in-memory, after caching)
			if len(blockedIDs) > 0 {
				filtered := filterItems(items, blockedIDs)
				response["data"] = filtered
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

			// Redis 캐시에서 게시글 조회 (raw post data)
			var post *gnuboard.G5Write
			ctx := c.Request.Context()
			if cacheService != nil {
				if cached, cacheErr := cacheService.GetPost(ctx, slug, id); cacheErr == nil {
					var cachedPost gnuboard.G5Write
					if json.Unmarshal(cached, &cachedPost) == nil {
						post = &cachedPost
					}
				}
			}

			if post == nil {
				// Cache miss: DB에서 조회
				post, err = gnuWriteRepo.FindPostByIDIncludeDeleted(slug, id)
				if err != nil {
					c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Post not found"})
					return
				}
				// Redis에 캐시 저장
				if cacheService != nil {
					_ = cacheService.SetPost(ctx, slug, id, post)
				}
			}

			// Increment view count async (skip for deleted posts)
			// 응답 지연 방지: goroutine으로 비동기 처리
			if post.WrDeletedAt == nil {
				go func(boardID string, wrID int) {
					if vcErr := gnuWriteRepo.IncrementHit(boardID, wrID); vcErr != nil {
						log.Printf("IncrementHit error: %v", vcErr)
					}
				}(slug, id)
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

			// 태그 조회
			if tags, err := gnuTagRepo.GetPostTags(slug, id); err == nil && len(tags) > 0 {
				postDetail["tags"] = tags
			}

			// Admin sees full (unmasked) IP
			if middleware.GetUserLevel(c) >= 10 {
				v1handler.OverrideIPForAdminSingle(postDetail, post)
			}

			// 삭제된 글: 일반 유저는 본문 숨김, 관리자는 본문 + 리비전 이력 표시
			if post.WrDeletedAt != nil {
				if middleware.GetUserLevel(c) >= 10 {
					// 관리자: 리비전 이력 조회
					var revisions []map[string]any
					db.Raw("SELECT id, version, change_type, title, content, edited_by, edited_by_name, edited_at, metadata FROM g5_write_revisions WHERE board_id = ? AND wr_id = ? ORDER BY version DESC", slug, id).Scan(&revisions)
					postDetail["revisions"] = revisions
				} else {
					// 일반 유저: 본문 숨김
					postDetail["content"] = ""
					postDetail["title"] = "[삭제된 게시물입니다]"
				}
			}

			// 비밀글 접근 제어: 작성자 또는 관리자만 내용 열람 가능
			if post.WrDeletedAt == nil && strings.Contains(post.WrOption, "secret") {
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

			ctx := c.Request.Context()
			isAdmin := middleware.GetUserLevel(c) >= 10

			// Get comments from g5_write_{slug} where wr_parent = id and wr_is_comment = 1
			var comments []*gnuboard.G5Write

			if isAdmin {
				// 관리자: 삭제된 댓글 포함 — 캐시 사용하지 않음 (데이터 다름)
				comments, err = gnuWriteRepo.FindCommentsIncludeDeleted(slug, id)
			} else {
				// 일반 사용자: Redis 캐시에서 전체 댓글 조회 후 차단 필터링
				var cacheHit bool
				if cacheService != nil {
					if cached, cacheErr := cacheService.GetComments(ctx, slug, id); cacheErr == nil {
						var cachedComments []*gnuboard.G5Write
						if json.Unmarshal(cached, &cachedComments) == nil {
							comments = cachedComments
							cacheHit = true
						}
					}
				}

				if !cacheHit {
					// Cache miss: DB에서 조회 (차단 필터 없이 전체)
					comments, err = gnuWriteRepo.FindCommentsFiltered(slug, id, nil)
					if err == nil && cacheService != nil {
						_ = cacheService.SetComments(ctx, slug, id, comments)
					}
				}

				// 차단 사용자 필터링 (캐시 후 인메모리)
				blockedIDs := getBlockedIDs(ctx, middleware.GetUserID(c))
				if len(blockedIDs) > 0 {
					blockedSet := make(map[string]struct{}, len(blockedIDs))
					for _, bid := range blockedIDs {
						blockedSet[bid] = struct{}{}
					}
					filtered := make([]*gnuboard.G5Write, 0, len(comments))
					for _, comment := range comments {
						if _, blocked := blockedSet[comment.MbID]; !blocked {
							filtered = append(filtered, comment)
						}
					}
					comments = filtered
				}
			}
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
				return
			}

			// 댓글별 수정 횟수 배치 조회
			commentIDs := make([]int, len(comments))
			for i, cm := range comments {
				commentIDs[i] = cm.WrID
			}
			editCountMap := map[int]int{}
			if len(commentIDs) > 0 {
				type EditCount struct {
					WrID  int `gorm:"column:wr_id"`
					Count int `gorm:"column:cnt"`
				}
				var counts []EditCount
				db.Table("g5_write_revisions").
					Select("wr_id, COUNT(*) as cnt").
					Where("board_id = ? AND wr_id IN ?", slug, commentIDs).
					Group("wr_id").
					Find(&counts)
				for _, ec := range counts {
					editCountMap[ec.WrID] = ec.Count
				}
			}

			transformed := v1handler.TransformToV1Comments(comments)
			// Admin sees full (unmasked) IP
			if isAdmin {
				v1handler.OverrideIPForAdmin(transformed, comments)
			}
			for i, comment := range comments {
				if cnt, ok := editCountMap[comment.WrID]; ok && cnt > 0 {
					transformed[i]["edit_count"] = cnt
				}
			}

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    transformed,
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
				MbID             string     `json:"mb_id"`
				MbName           string     `json:"mb_name"`
				MbNick           string     `json:"mb_nick"`
				MbImageUrl       string     `json:"mb_image,omitempty"`
				MbImageUpdatedAt *time.Time `json:"mb_image_updated_at,omitempty"`
				BgIP             string     `json:"bg_ip,omitempty"`
				LikedAt          *time.Time `json:"liked_at"`
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
				Select("bg.mb_id, COALESCE(m.mb_name, '') as mb_name, COALESCE(m.mb_nick, bg.mb_id) as mb_nick, COALESCE(m.mb_image_url, '') as mb_image_url, m.mb_image_updated_at, bg.bg_ip, bg.bg_datetime as liked_at").
				Joins("LEFT JOIN g5_member m ON bg.mb_id = m.mb_id").
				Where("bg.bo_table = ? AND bg.wr_id = ? AND bg.bg_flag = ?", slug, postID, "good").
				Order("bg.bg_datetime DESC").
				Offset(offset).Limit(limit).
				Scan(&likers)

			// IP 마스킹: 마지막 옥텟을 ***로 변환
			for i := range likers {
				likers[i].BgIP = maskIP(likers[i].BgIP)
			}

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"likers": likers,
					"total":  total,
				},
			})
		})

		// GET /api/v1/boards/:slug/posts/:id/revisions - Get post revision history
		v1Boards.GET("/:slug/posts/:id/revisions", func(c *gin.Context) {
			slug := c.Param("slug")
			postID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid post ID"})
				return
			}

			userLevel := middleware.GetUserLevel(c)

			// 관리자: full revision 데이터 반환
			if userLevel >= 10 {
				type Revision struct {
					ID           int64     `json:"id"`
					BoardID      string    `json:"board_id"`
					WrID         int       `json:"wr_id"`
					Version      int       `json:"version"`
					ChangeType   string    `json:"change_type"`
					Title        *string   `json:"title"`
					Content      *string   `json:"content"`
					EditedBy     string    `json:"edited_by"`
					EditedByName *string   `json:"edited_by_name"`
					EditedAt     time.Time `json:"edited_at"`
				}

				var revisions []Revision
				if err := db.Table("g5_write_revisions").
					Where("board_id = ? AND wr_id = ?", slug, postID).
					Order("version DESC").
					Find(&revisions).Error; err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "리비전 조회 실패"})
					return
				}

				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"data":    revisions,
				})
				return
			}

			// 일반 사용자: count + last_edited_at만 반환
			var editCount int64
			db.Table("g5_write_revisions").
				Where("board_id = ? AND wr_id = ?", slug, postID).
				Count(&editCount)

			var lastEditedAt *time.Time
			if editCount > 0 {
				var t time.Time
				db.Table("g5_write_revisions").
					Select("MAX(edited_at)").
					Where("board_id = ? AND wr_id = ?", slug, postID).
					Scan(&t)
				lastEditedAt = &t
			}

			c.JSON(http.StatusOK, gin.H{
				"success":        true,
				"data":           []any{},
				"edit_count":     editCount,
				"last_edited_at": lastEditedAt,
			})
		})

		// POST /api/v1/boards/:slug/posts - Create post in g5_write_{slug}
		v1Boards.POST("/:slug/posts", middleware.JWTAuth(jwtManager), banCheck, middleware.IPProtection(ipProtectCfg), func(c *gin.Context) {
			slug := c.Param("slug")

			// 게시판 설정 조회
			board, err := gnuBoardRepo.FindByID(slug)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "게시판을 찾을 수 없습니다"})
				return
			}

			// 요청 바디 파싱
			var req struct {
				Title    string   `json:"title" binding:"required"`
				Content  string   `json:"content" binding:"required"`
				Category *string  `json:"category"`
				IsSecret *bool    `json:"is_secret"`
				Link1    *string  `json:"link1"`
				Link2    *string  `json:"link2"`
				Extra1   *string  `json:"extra_1"`
				Extra2   *string  `json:"extra_2"`
				Extra3   *string  `json:"extra_3"`
				Tags     []string `json:"tags"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "제목과 내용을 입력해주세요"})
				return
			}

			mbID := middleware.GetUserID(c)
			userLevel := middleware.GetUserLevel(c)

			// 제휴 링크 차단 검증
			if err := common.ValidateAffiliateLinks(req.Content, slug, userLevel, false); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": err.Error()})
				return
			}

			// 레벨 체크
			if userLevel < board.BoWriteLevel {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "글쓰기 권한이 없습니다. 레벨 " + strconv.Itoa(board.BoWriteLevel) + " 이상이 필요합니다."})
				return
			}

			// 포인트 차감 게시판인 경우 잔액 확인 (g5_member.mb_point 기반)
			if board.BoWritePoint < 0 {
				canAfford, err := gnuPointWriteRepo.CanAfford(mbID, board.BoWritePoint)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "포인트 확인 실패"})
					return
				}
				if !canAfford {
					c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "포인트가 부족합니다. " + strconv.Itoa(-board.BoWritePoint) + "포인트가 필요합니다."})
					return
				}
			}

			// ExtendedSettings 기반 글쓰기 제한 체크 (범용 - 모든 게시판)
			restriction, err := writeRestrictionSvc.Check(slug, mbID, userLevel)
			if err != nil {
				fmt.Printf("[WARN] write restriction check failed for board %s: %v\n", slug, err)
				// 제한 체크 실패 시 통과 (기존 동작 유지)
			} else if !restriction.CanWrite {
				c.JSON(http.StatusForbidden, gin.H{
					"success": false,
					"error":   restriction.Reason,
					"data":    restriction,
				})
				return
			}

			// 작성자 닉네임 조회
			authorName := middleware.GetNickname(c)
			if authorName == "" {
				db.Table("g5_member").Select("mb_nick").Where("mb_id = ?", mbID).Scan(&authorName)
			}

			// g5_write_{slug} 테이블에 게시글 INSERT
			now := time.Now()
			nowStr := now.Format("2006-01-02 15:04:05")

			wrOption := ""
			if req.IsSecret != nil && *req.IsSecret {
				wrOption = "secret"
			}

			post := gnuboard.G5Write{
				WrNum:       0,
				WrParent:    0,
				WrIsComment: 0,
				WrSubject:   req.Title,
				WrContent:   req.Content,
				MbID:        mbID,
				WrName:      authorName,
				WrDatetime:  now,
				WrLast:      nowStr,
				WrIP:        middleware.GetClientIP(c),
				WrOption:    wrOption,
			}

			if req.Category != nil {
				post.CaName = *req.Category
			}
			if req.Link1 != nil {
				post.WrLink1 = *req.Link1
			}
			if req.Link2 != nil {
				post.WrLink2 = *req.Link2
			}

			tableName := fmt.Sprintf("g5_write_%s", slug)

			// wr_num 값 계산 (가장 작은 음수값 - 1, gnuboard 정렬 규칙)
			var minNum int
			db.Table(tableName).Select("COALESCE(MIN(wr_num), 0)").Scan(&minNum)
			post.WrNum = minNum - 1

			if err := db.Table(tableName).Create(&post).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "게시글 작성 실패"})
				return
			}

			// wr_parent를 자기 자신의 wr_id로 UPDATE (gnuboard 규칙)
			db.Table(tableName).Where("wr_id = ?", post.WrID).Update("wr_parent", post.WrID)

			// 태그 저장 (g5_na_tag / g5_na_tag_log)
			if len(req.Tags) > 0 {
				if err := gnuTagRepo.SetPostTags(slug, post.WrID, req.Tags, mbID); err != nil {
					fmt.Printf("[WARN] tag save failed for %s/%d: %v\n", slug, post.WrID, err)
				}
			}

			// extra 필드 업데이트 (있는 경우)
			extras := map[string]interface{}{}
			if req.Extra1 != nil {
				extras["wr_1"] = *req.Extra1
			}
			if req.Extra2 != nil {
				extras["wr_2"] = *req.Extra2
			}
			if req.Extra3 != nil {
				extras["wr_3"] = *req.Extra3
			}
			if len(extras) > 0 {
				db.Table(tableName).Where("wr_id = ?", post.WrID).Updates(extras)
			}

			// 포인트 처리 (g5_point 기반 FIFO 소비)
			if board.BoWritePoint != 0 {
				var pc *v2repo.PointConfig
				if pointConfigRepo != nil {
					pc, _ = pointConfigRepo.GetPointConfig()
				}
				_ = gnuPointWriteRepo.AddPoint(mbID, board.BoWritePoint, "글쓰기", tableName, fmt.Sprintf("%d", post.WrID), "@write", pc) //nolint:errcheck
			}

			// 게시글 목록 캐시 무효화 (새 글이 목록에 즉시 반영되도록)
			if cacheService != nil {
				_ = cacheService.InvalidatePosts(c.Request.Context(), slug)
			}
			postMemCache.Range(func(key, value interface{}) bool {
				if strings.HasPrefix(key.(string), "posts:"+slug+":") {
					postMemCache.Delete(key)
				}
				return true
			})

			// 팔로워/구독자 알림 (비동기)
			go func() {
				authorName := post.WrName
				if authorName == "" {
					authorName = mbID
				}
				subject := post.WrSubject
				now := time.Now()

				// 1. 팔로워 알림: 이 작성자를 팔로우한 사람들
				var followerIDs []string
				db.Table("g5_member_follow").Select("mb_id").Where("target_id = ?", mbID).Pluck("mb_id", &followerIDs)
				for _, fid := range followerIDs {
					if pref, _ := notiPrefRepo.Get(fid); !pref.NotiFollow {
						continue
					}
					_ = notiRepo.Create(&gnurepo.Notification{
						PhToCase: "follow", PhFromCase: "write", BoTable: slug,
						WrID: post.WrID, MbID: fid, RelMbID: mbID,
						RelMbNick:  authorName,
						RelMsg:     fmt.Sprintf("%s님이 새 글을 작성했습니다: %s", authorName, subject),
						RelURL:     fmt.Sprintf("/%s/%d", slug, post.WrID),
						PhReaded:   "N",
						PhDatetime: now,
						WrParent:   post.WrID,
					})
				}

				// 2. 게시판 구독자 알림: 이 게시판을 구독한 사람들 (작성자 제외, 팔로워 중복 제외)
				var subscriberIDs []string
				db.Table("g5_board_subscribe").Select("mb_id").Where("bo_table = ? AND mb_id != ?", slug, mbID).Pluck("mb_id", &subscriberIDs)
				followerSet := make(map[string]bool, len(followerIDs))
				for _, fid := range followerIDs {
					followerSet[fid] = true
				}
				for _, sid := range subscriberIDs {
					if followerSet[sid] {
						continue // 이미 팔로워 알림 받음
					}
					if pref, _ := notiPrefRepo.Get(sid); !pref.NotiFollow {
						continue
					}
					_ = notiRepo.Create(&gnurepo.Notification{
						PhToCase: "subscribe", PhFromCase: "write", BoTable: slug,
						WrID: post.WrID, MbID: sid, RelMbID: mbID,
						RelMbNick:  authorName,
						RelMsg:     fmt.Sprintf("%s 게시판에 새 글: %s", slug, subject),
						RelURL:     fmt.Sprintf("/%s/%d", slug, post.WrID),
						PhReaded:   "N",
						PhDatetime: now,
						WrParent:   post.WrID,
					})
				}
			}()

			c.JSON(http.StatusCreated, gin.H{
				"success": true,
				"data":    v1handler.TransformToV1PostDetail(&post, false),
			})
		})

		// POST /api/v1/boards/:slug/posts/:id/comments - Create comment in g5_write_{slug}
		v1Boards.POST("/:slug/posts/:id/comments", middleware.JWTAuth(jwtManager), banCheck, middleware.IPProtection(ipProtectCfg), func(c *gin.Context) {
			slug := c.Param("slug")
			postID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid post ID"})
				return
			}

			// 게시판 설정 조회
			board, err := gnuBoardRepo.FindByID(slug)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "게시판을 찾을 수 없습니다"})
				return
			}

			// 요청 바디 파싱
			var req struct {
				Content  string `json:"content" binding:"required"`
				ParentID *int   `json:"parent_id,omitempty"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "내용을 입력해주세요"})
				return
			}

			mbID := middleware.GetUserID(c)
			userLevel := middleware.GetUserLevel(c)

			// 제휴 링크 차단 검증
			if err := common.ValidateAffiliateLinks(req.Content, slug, userLevel, false); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": err.Error()})
				return
			}

			// 레벨 체크
			if userLevel < board.BoCommentLevel {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "댓글 작성 권한이 없습니다. 레벨 " + strconv.Itoa(board.BoCommentLevel) + " 이상이 필요합니다."})
				return
			}

			// 포인트 차감 게시판인 경우 잔액 확인 (g5_member.mb_point 기반)
			if board.BoCommentPoint < 0 {
				canAfford, err := gnuPointWriteRepo.CanAfford(mbID, board.BoCommentPoint)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "포인트 확인 실패"})
					return
				}
				if !canAfford {
					c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "포인트가 부족합니다. " + strconv.Itoa(-board.BoCommentPoint) + "포인트가 필요합니다."})
					return
				}
			}

			// 작성자 닉네임 조회
			authorName := middleware.GetNickname(c)
			if authorName == "" {
				db.Table("g5_member").Select("mb_nick").Where("mb_id = ?", mbID).Scan(&authorName)
			}

			// g5_write_{slug} 테이블에 댓글 INSERT
			now := time.Now()
			nowStr := now.Format("2006-01-02 15:04:05")
			tableName := fmt.Sprintf("g5_write_%s", slug)

			// 대댓글 처리 (그누보드 호환)
			// wr_comment: 순서 번호 (루트 댓글은 MAX+1, 대댓글은 부모 루트의 순서 번호)
			// wr_comment_reply: 계층 경로 문자열 ("", "A", "AB", "AAA" 등)
			wrComment := 0
			wrCommentReply := ""
			depth := 0

			// 트랜잭션으로 댓글 생성 (wr_comment 레이스 컨디션 방지)
			var createdComment gnuboard.G5Write
			txErr := db.Transaction(func(tx *gorm.DB) error {
				if req.ParentID != nil && *req.ParentID > 0 {
					// 대댓글: 부모 댓글 조회 (FOR UPDATE로 잠금)
					var parentComment gnuboard.G5Write
					if err := tx.Table(tableName).
						Clauses(clause.Locking{Strength: "UPDATE"}).
						Where("wr_id = ? AND wr_is_comment = 1", *req.ParentID).
						First(&parentComment).Error; err != nil {
						return fmt.Errorf("PARENT_NOT_FOUND")
					}

					wrComment = parentComment.WrComment

					parentReply := parentComment.WrCommentReply
					replyLen := len(parentReply) + 1
					depth = replyLen

					var lastReply string
					tx.Table(tableName).
						Select("wr_comment_reply").
						Where("wr_parent = ? AND wr_is_comment = 1 AND wr_comment = ? AND LENGTH(wr_comment_reply) = ? AND wr_comment_reply LIKE ?",
							postID, wrComment, replyLen, parentReply+"%").
						Order("wr_comment_reply DESC").
						Limit(1).
						Scan(&lastReply)

					if lastReply == "" {
						wrCommentReply = parentReply + "A"
					} else {
						lastChar := lastReply[len(lastReply)-1]
						if lastChar < 'Z' {
							wrCommentReply = parentReply + string(lastChar+1)
						} else {
							return fmt.Errorf("REPLY_LIMIT")
						}
					}
				} else {
					// 루트 댓글: FOR UPDATE로 게시글 행 잠금 후 MAX(wr_comment) + 1
					tx.Table(tableName).
						Clauses(clause.Locking{Strength: "UPDATE"}).
						Select("wr_id").
						Where("wr_id = ? AND wr_is_comment = 0", postID).
						Scan(&struct{}{})

					var maxComment int
					tx.Table(tableName).
						Select("COALESCE(MAX(wr_comment), -1)").
						Where("wr_parent = ? AND wr_is_comment = 1", postID).
						Scan(&maxComment)
					wrComment = maxComment + 1
				}

				createdComment = gnuboard.G5Write{
					WrNum:          0,
					WrParent:       postID,
					WrIsComment:    1,
					WrComment:      wrComment,
					WrCommentReply: wrCommentReply,
					WrContent:      req.Content,
					MbID:           mbID,
					WrName:         authorName,
					WrDatetime:     now,
					WrLast:         nowStr,
					WrIP:           middleware.GetClientIP(c),
				}

				if err := tx.Table(tableName).Create(&createdComment).Error; err != nil {
					return err
				}

				// 부모 게시글의 wr_comment 카운트 갱신
				var commentCount int64
				tx.Table(tableName).Where("wr_parent = ? AND wr_is_comment = 1 AND wr_deleted_at IS NULL", postID).Count(&commentCount)
				tx.Table(tableName).Where("wr_id = ?", postID).Update("wr_comment", commentCount)

				return nil
			})

			if txErr != nil {
				switch txErr.Error() {
				case "PARENT_NOT_FOUND":
					c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "부모 댓글을 찾을 수 없습니다"})
				case "REPLY_LIMIT":
					c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "더 이상 대댓글을 작성할 수 없습니다."})
				default:
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "댓글 작성 실패"})
				}
				return
			}
			comment := createdComment

			// 포인트 처리 (g5_point 기반 FIFO 소비)
			if board.BoCommentPoint != 0 {
				var pc *v2repo.PointConfig
				if pointConfigRepo != nil {
					pc, _ = pointConfigRepo.GetPointConfig()
				}
				_ = gnuPointWriteRepo.AddPoint(mbID, board.BoCommentPoint, "댓글작성", fmt.Sprintf("g5_write_%s", slug), fmt.Sprintf("%d", comment.WrID), "@comment", pc) //nolint:errcheck
			}

			// 캐시 무효화: 댓글 목록 + 게시글 목록 (댓글 수 변경 반영)
			if cacheService != nil {
				_ = cacheService.InvalidateComments(c.Request.Context(), slug, postID)
				_ = cacheService.InvalidatePosts(c.Request.Context(), slug)
			}
			postMemCache.Range(func(key, value interface{}) bool {
				if strings.HasPrefix(key.(string), "posts:"+slug+":") {
					postMemCache.Delete(key)
				}
				return true
			})

			// Admin sees full IP, others see masked
			commentIP := v1handler.MaskIP(comment.WrIP)
			if middleware.GetUserLevel(c) >= 10 {
				commentIP = comment.WrIP
			}

			// 알림 생성 (비동기)
			go func() {
				// 게시글 작성자 조회
				var postAuthor struct {
					MbID      string `gorm:"column:mb_id"`
					WrSubject string `gorm:"column:wr_subject"`
				}
				if err := db.Table(tableName).Select("mb_id, wr_subject").Where("wr_id = ? AND wr_is_comment = 0", postID).Scan(&postAuthor).Error; err != nil || postAuthor.MbID == "" {
					return
				}

				// 대댓글인 경우: 부모 댓글 작성자에게 알림
				if req.ParentID != nil && *req.ParentID > 0 {
					var parentAuthorMbID string
					if err := db.Table(tableName).Select("mb_id").Where("wr_id = ?", *req.ParentID).Scan(&parentAuthorMbID).Error; err == nil && parentAuthorMbID != "" && parentAuthorMbID != mbID {
						// 수신자가 발신자를 차단한 경우 알림 생략 (Redis 캐시 활용)
						if slices.Contains(getBlockedIDs(context.Background(), parentAuthorMbID), mbID) {
							// skip
						} else if pref, _ := notiPrefRepo.Get(parentAuthorMbID); pref.NotiReply {
							_ = notiRepo.Create(&gnurepo.Notification{
								PhToCase:      "comment_reply",
								PhFromCase:    "comment",
								BoTable:       slug,
								WrID:          comment.WrID,
								MbID:          parentAuthorMbID,
								RelMbID:       mbID,
								RelMbNick:     authorName,
								RelMsg:        fmt.Sprintf("%s님이 회원님의 댓글에 답글을 남겼습니다.", authorName),
								RelURL:        fmt.Sprintf("/%s/%d#comment_%d", slug, postID, comment.WrID),
								PhReaded:      "N",
								PhDatetime:    now,
								ParentSubject: postAuthor.WrSubject,
								WrParent:      postID,
							})
						}
					}
				}

				// 게시글 작성자에게 알림 (자기 댓글은 제외, 차단한 사용자 제외)
				if postAuthor.MbID != mbID {
					if slices.Contains(getBlockedIDs(context.Background(), postAuthor.MbID), mbID) {
						// 게시글 작성자가 댓글 작성자를 차단한 경우 알림 생략
					} else if pref, _ := notiPrefRepo.Get(postAuthor.MbID); pref.NotiComment {
						_ = notiRepo.Create(&gnurepo.Notification{
							PhToCase:      "comment",
							PhFromCase:    "comment",
							BoTable:       slug,
							WrID:          comment.WrID,
							MbID:          postAuthor.MbID,
							RelMbID:       mbID,
							RelMbNick:     authorName,
							RelMsg:        fmt.Sprintf("%s님이 회원님의 글에 댓글을 남겼습니다.", authorName),
							RelURL:        fmt.Sprintf("/%s/%d#comment_%d", slug, postID, comment.WrID),
							PhReaded:      "N",
							PhDatetime:    now,
							ParentSubject: postAuthor.WrSubject,
							WrParent:      postID,
						})
					}
				}
			}()

			c.JSON(http.StatusCreated, gin.H{
				"success": true,
				"data": gin.H{
					"id":         comment.WrID,
					"post_id":    postID,
					"content":    comment.WrContent,
					"author":     authorName,
					"author_id":  mbID,
					"author_ip":  commentIP,
					"likes":      0,
					"dislikes":   0,
					"depth":      depth,
					"created_at": now.Format(time.RFC3339),
					"is_secret":  false,
				},
			})
		})

		// PUT /api/v1/boards/:slug/posts/:id - Update post
		v1Boards.PUT("/:slug/posts/:id", middleware.JWTAuth(jwtManager), banCheck, func(c *gin.Context) {
			slug := c.Param("slug")
			postID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid post ID"})
				return
			}

			// 게시글 조회 (삭제된 글 포함 — 삭제 여부 체크용)
			post, err := gnuWriteRepo.FindPostByIDIncludeDeleted(slug, postID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "게시글을 찾을 수 없습니다"})
				return
			}

			// 소프트 삭제된 글 수정 차단
			if post.WrDeletedAt != nil {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "삭제된 게시물은 수정할 수 없습니다"})
				return
			}

			// 삭제 예약된 글 수정 차단
			if pending, _ := scheduledDeleteRepo.FindByPost(slug, postID); pending != nil {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "삭제 대기 중인 게시물은 수정할 수 없습니다"})
				return
			}

			// 작성자 또는 관리자 확인
			userID := middleware.GetUserID(c)
			userLevel := middleware.GetUserLevel(c)
			if post.MbID != userID && userLevel < 10 {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "수정 권한이 없습니다"})
				return
			}

			// 요청 바디 파싱
			var req struct {
				Title    *string  `json:"title"`
				Content  *string  `json:"content"`
				Category *string  `json:"category"`
				Link1    *string  `json:"link1"`
				Link2    *string  `json:"link2"`
				Extra1   *string  `json:"extra_1"`
				Extra2   *string  `json:"extra_2"`
				Extra3   *string  `json:"extra_3"`
				Tags     []string `json:"tags"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "잘못된 요청입니다"})
				return
			}

			// 수정 전 내용을 리비전에 저장
			var nextVersion int
			db.Raw("SELECT COALESCE(MAX(version), 0) + 1 FROM g5_write_revisions WHERE board_id = ? AND wr_id = ?", slug, postID).Scan(&nextVersion)
			db.Exec(`INSERT INTO g5_write_revisions
				(board_id, wr_id, version, change_type, title, content, edited_by, edited_by_name, edited_at)
				VALUES (?, ?, ?, 'update', ?, ?, ?, ?, NOW())`,
				slug, postID, nextVersion, post.WrSubject, post.WrContent, userID, post.WrName)

			// g5_da_content_history에도 이중 기록
			{
				prevData, _ := json.Marshal(map[string]interface{}{
					"wr_subject": post.WrSubject,
					"wr_content": post.WrContent,
					"wr_name":    post.WrName,
					"mb_id":      post.MbID,
				})
				db.Exec(`INSERT INTO g5_da_content_history
					(bo_table, wr_id, wr_is_comment, mb_id, wr_name, operation, operated_by, operated_at, previous_data)
					VALUES (?, ?, 0, ?, ?, '수정', ?, NOW(), ?)`,
					slug, postID, post.MbID, post.WrName, userID, string(prevData))
			}

			// 업데이트할 필드 구성
			updates := map[string]interface{}{}
			if req.Title != nil {
				updates["wr_subject"] = *req.Title
			}
			if req.Content != nil {
				updates["wr_content"] = *req.Content
			}
			if req.Category != nil {
				updates["ca_name"] = *req.Category
			}
			if req.Link1 != nil {
				updates["wr_link1"] = *req.Link1
			}
			if req.Link2 != nil {
				updates["wr_link2"] = *req.Link2
			}
			if req.Extra1 != nil {
				updates["wr_1"] = *req.Extra1
			}
			if req.Extra2 != nil {
				updates["wr_2"] = *req.Extra2
			}
			if req.Extra3 != nil {
				updates["wr_3"] = *req.Extra3
			}

			if len(updates) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "수정할 내용이 없습니다"})
				return
			}

			tableName := fmt.Sprintf("g5_write_%s", slug)
			if err := db.Table(tableName).Where("wr_id = ?", postID).Updates(updates).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "게시글 수정 실패"})
				return
			}

			// 태그 업데이트 (tags 필드가 전송된 경우)
			if req.Tags != nil {
				if err := gnuTagRepo.SetPostTags(slug, postID, req.Tags, userID); err != nil {
					fmt.Printf("[WARN] tag update failed for %s/%d: %v\n", slug, postID, err)
				}
			}

			// 캐시 무효화: 게시글 상세 + 목록
			if cacheService != nil {
				_ = cacheService.InvalidatePost(c.Request.Context(), slug, postID)
				_ = cacheService.InvalidatePosts(c.Request.Context(), slug)
			}
			postMemCache.Range(func(key, value interface{}) bool {
				if strings.HasPrefix(key.(string), "posts:"+slug+":") {
					postMemCache.Delete(key)
				}
				return true
			})

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "수정 완료"})
		})

		// PUT /api/v1/boards/:slug/posts/:id/comments/:comment_id - Update comment
		v1Boards.PUT("/:slug/posts/:id/comments/:comment_id", middleware.JWTAuth(jwtManager), banCheck, func(c *gin.Context) {
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
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "수정 권한이 없습니다"})
				return
			}

			// 요청 바디 파싱
			var req struct {
				Content string `json:"content" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "내용을 입력해주세요"})
				return
			}

			// 수정 전 내용을 리비전에 저장
			var nextVersion int
			db.Raw("SELECT COALESCE(MAX(version), 0) + 1 FROM g5_write_revisions WHERE board_id = ? AND wr_id = ?",
				slug, commentID).Scan(&nextVersion)
			db.Exec(`INSERT INTO g5_write_revisions
				(board_id, wr_id, version, change_type, title, content, edited_by, edited_by_name, edited_at)
				VALUES (?, ?, ?, 'update', NULL, ?, ?, ?, NOW())`,
				slug, commentID, nextVersion, comment.WrContent, userID, comment.WrName)

			// g5_da_content_history에도 이중 기록
			{
				prevData, _ := json.Marshal(map[string]interface{}{
					"wr_content": comment.WrContent,
					"wr_name":    comment.WrName,
					"mb_id":      comment.MbID,
				})
				db.Exec(`INSERT INTO g5_da_content_history
					(bo_table, wr_id, wr_is_comment, mb_id, wr_name, operation, operated_by, operated_at, previous_data)
					VALUES (?, ?, 1, ?, ?, '수정', ?, NOW(), ?)`,
					slug, commentID, comment.MbID, comment.WrName, userID, string(prevData))
			}

			tableName := fmt.Sprintf("g5_write_%s", slug)
			now := time.Now().Format("2006-01-02 15:04:05")
			if err := db.Table(tableName).Where("wr_id = ?", commentID).Updates(map[string]interface{}{
				"wr_content": req.Content,
				"wr_last":    now,
			}).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "댓글 수정 실패"})
				return
			}

			// 캐시 무효화: 댓글 목록
			if cacheService != nil {
				postID, _ := strconv.Atoi(c.Param("id"))
				_ = cacheService.InvalidateComments(c.Request.Context(), slug, postID)
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "수정 완료"})
		})

		// GET /api/v1/boards/:slug/posts/:id/comments/:comment_id/revisions - Get comment revision history
		v1Boards.GET("/:slug/posts/:id/comments/:comment_id/revisions", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			slug := c.Param("slug")
			commentID, err := strconv.Atoi(c.Param("comment_id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid comment ID"})
				return
			}

			// 관리자만 리비전 열람 가능
			userLevel := middleware.GetUserLevel(c)
			if userLevel < 10 {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "관리자만 조회 가능합니다"})
				return
			}

			type Revision struct {
				ID           int64     `json:"id"`
				BoardID      string    `json:"board_id"`
				WrID         int       `json:"wr_id"`
				Version      int       `json:"version"`
				ChangeType   string    `json:"change_type"`
				Title        *string   `json:"title"`
				Content      *string   `json:"content"`
				EditedBy     string    `json:"edited_by"`
				EditedByName *string   `json:"edited_by_name"`
				EditedAt     time.Time `json:"edited_at"`
			}

			var revisions []Revision
			if err := db.Table("g5_write_revisions").
				Where("board_id = ? AND wr_id = ?", slug, commentID).
				Order("version DESC").
				Find(&revisions).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "리비전 조회 실패"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    revisions,
			})
		})

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

			// 관리자(level >= 10)는 즉시 삭제
			if userLevel >= 10 {
				if err := gnuWriteRepo.SoftDeletePost(slug, postID, userID); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "게시글 삭제 실패"})
					return
				}
				// 캐시 무효화: 게시글 상세 + 목록
				if cacheService != nil {
					_ = cacheService.InvalidatePost(c.Request.Context(), slug, postID)
					_ = cacheService.InvalidatePosts(c.Request.Context(), slug)
				}
				postMemCache.Range(func(key, value interface{}) bool {
					if strings.HasPrefix(key.(string), "posts:"+slug+":") {
						postMemCache.Delete(key)
					}
					return true
				})
				c.JSON(http.StatusOK, gin.H{"success": true, "message": "삭제 완료"})
				return
			}

			// 일반 사용자: 무조건 지연 소프트 삭제
			commentCount := post.WrComment
			delayMinutes := gnuboard.CalculateDelay(commentCount)

			// 이미 삭제 예약된 경우 체크
			existing, _ := scheduledDeleteRepo.FindByPost(slug, postID)
			if existing != nil {
				c.JSON(http.StatusConflict, gin.H{
					"success":      false,
					"error":        "이미 삭제가 예약되어 있습니다",
					"scheduled_at": existing.ScheduledAt,
				})
				return
			}

			// 지연 삭제 예약
			now := time.Now()
			sd := &gnuboard.ScheduledDelete{
				BoTable:      slug,
				WrID:         postID,
				WrIsComment:  0,
				ReplyCount:   commentCount,
				DelayMinutes: delayMinutes,
				ScheduledAt:  now.Add(time.Duration(delayMinutes) * time.Minute),
				RequestedBy:  userID,
				RequestedAt:  now,
				Status:       "pending",
			}
			if err := scheduledDeleteRepo.Create(sd); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "삭제 예약 실패"})
				return
			}

			// 캐시 무효화
			middleware.InvalidateCacheByPath(redisClient, fmt.Sprintf("/api/v1/boards/%s/posts/%d/delete-status", slug, postID))

			c.JSON(http.StatusOK, gin.H{
				"success":       true,
				"message":       fmt.Sprintf("댓글이 %d개 있어 %d분 후 삭제됩니다", commentCount, delayMinutes),
				"scheduled":     true,
				"scheduled_at":  sd.ScheduledAt,
				"delay_minutes": delayMinutes,
			})
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

			// 관리자(level >= 10)는 즉시 삭제
			postID, _ := strconv.Atoi(c.Param("id"))
			if userLevel >= 10 {
				if err := gnuWriteRepo.SoftDeleteComment(slug, commentID, userID); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "댓글 삭제 실패"})
					return
				}
				// 캐시 무효화: 댓글 + 게시글 목록 (댓글 수 변경)
				if cacheService != nil {
					_ = cacheService.InvalidateComments(c.Request.Context(), slug, postID)
					_ = cacheService.InvalidatePosts(c.Request.Context(), slug)
				}
				c.JSON(http.StatusOK, gin.H{"success": true, "message": "삭제 완료"})
				return
			}

			// 일반 사용자: 답글 수에 따라 지연 삭제 적용
			replyCount, err := gnuWriteRepo.CountCommentReplies(slug, postID, commentID)
			if err != nil {
				// 카운트 실패 시 즉시 삭제로 fallback
				replyCount = 0
			}

			delayMinutes := gnuboard.CalculateDelay(common.SafeInt64ToInt(replyCount))

			if delayMinutes == 0 {
				// 답글 없으면 즉시 삭제
				if err := gnuWriteRepo.SoftDeleteComment(slug, commentID, userID); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "댓글 삭제 실패"})
					return
				}
				// 캐시 무효화: 댓글 + 게시글 목록 (댓글 수 변경)
				if cacheService != nil {
					_ = cacheService.InvalidateComments(c.Request.Context(), slug, postID)
					_ = cacheService.InvalidatePosts(c.Request.Context(), slug)
				}
				c.JSON(http.StatusOK, gin.H{"success": true, "message": "삭제 완료"})
				return
			}

			// 이미 삭제 예약된 경우 체크
			existing, _ := scheduledDeleteRepo.FindByPost(slug, commentID)
			if existing != nil {
				c.JSON(http.StatusConflict, gin.H{
					"success":      false,
					"error":        "이미 삭제가 예약되어 있습니다",
					"scheduled_at": existing.ScheduledAt,
				})
				return
			}

			// 지연 삭제 예약
			now := time.Now()
			sd := &gnuboard.ScheduledDelete{
				BoTable:      slug,
				WrID:         commentID,
				WrIsComment:  1,
				ReplyCount:   common.SafeInt64ToInt(replyCount),
				DelayMinutes: delayMinutes,
				ScheduledAt:  now.Add(time.Duration(delayMinutes) * time.Minute),
				RequestedBy:  userID,
				RequestedAt:  now,
				Status:       "pending",
			}
			if err := scheduledDeleteRepo.Create(sd); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "삭제 예약 실패"})
				return
			}

			// 캐시 무효화
			middleware.InvalidateCacheByPath(redisClient, fmt.Sprintf("/api/v1/boards/%s/posts/%d/delete-status", slug, commentID))

			c.JSON(http.StatusOK, gin.H{
				"success":       true,
				"message":       fmt.Sprintf("답글이 %d개 있어 %d분 후 삭제됩니다", replyCount, delayMinutes),
				"scheduled":     true,
				"scheduled_at":  sd.ScheduledAt,
				"delay_minutes": delayMinutes,
			})
		})

		// POST /api/v1/boards/:slug/posts/:id/comments/:comment_id/restore - Restore soft deleted comment (admin only)
		v1Boards.POST("/:slug/posts/:id/comments/:comment_id/restore", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			slug := c.Param("slug")
			commentID, err := strconv.Atoi(c.Param("comment_id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid comment ID"})
				return
			}

			// 관리자 확인
			userLevel := middleware.GetUserLevel(c)
			if userLevel < 10 {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "관리자만 복구할 수 있습니다"})
				return
			}

			// 댓글 복구
			if err := gnuWriteRepo.RestoreComment(slug, commentID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "댓글 복구 실패"})
				return
			}

			// 캐시 무효화
			postID, _ := strconv.Atoi(c.Param("id"))
			if cacheService != nil {
				_ = cacheService.InvalidateComments(c.Request.Context(), slug, postID)
				_ = cacheService.InvalidatePosts(c.Request.Context(), slug)
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "댓글 복구 완료"})
		})

		// POST /api/v1/boards/:slug/posts/:id/cancel-delete - Cancel a scheduled delete
		v1Boards.POST("/:slug/posts/:id/cancel-delete", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			slug := c.Param("slug")
			wrID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid ID"})
				return
			}

			sd, err := scheduledDeleteRepo.FindByPost(slug, wrID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "삭제 예약을 찾을 수 없습니다"})
				return
			}

			// 작성자 또는 관리자만 취소 가능
			userID := middleware.GetUserID(c)
			userLevel := middleware.GetUserLevel(c)
			if sd.RequestedBy != userID && userLevel < 10 {
				c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "취소 권한이 없습니다"})
				return
			}

			if err := scheduledDeleteRepo.Cancel(sd.ID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "삭제 취소 실패"})
				return
			}

			// 캐시 무효화
			middleware.InvalidateCacheByPath(redisClient, fmt.Sprintf("/api/v1/boards/%s/posts/%d/delete-status", slug, wrID))

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "삭제가 취소되었습니다"})
		})

		// GET /api/v1/boards/:slug/posts/:id/delete-status - Check scheduled delete status
		v1Boards.GET("/:slug/posts/:id/delete-status", middleware.CacheWithTTL(redisClient, 30*time.Second), func(c *gin.Context) {
			slug := c.Param("slug")
			wrID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid ID"})
				return
			}

			sd, err := scheduledDeleteRepo.FindByPost(slug, wrID)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "scheduled": false})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"success":       true,
				"scheduled":     true,
				"scheduled_at":  sd.ScheduledAt,
				"requested_at":  sd.RequestedAt,
				"requested_by":  sd.RequestedBy,
				"delay_minutes": sd.DelayMinutes,
				"reply_count":   sd.ReplyCount,
				"is_comment":    sd.WrIsComment == 1,
			})
		})

		// Board display settings (for admin layout switcher)
		v2DisplaySettingsRepo := v2repo.NewBoardDisplaySettingsRepository(db)
		// Auto-migrate to add new columns (e.g. comment_layout)
		if err := db.AutoMigrate(&v2domain.V2BoardDisplaySettings{}); err != nil {
			fmt.Printf("[WARN] v2_board_display_settings migration failed: %v\n", err)
		}
		displaySettingsHandler := v2handler.NewDisplaySettingsHandler(v2BoardRepo, v2DisplaySettingsRepo)
		v1Boards.GET("/:slug/display-settings", middleware.CacheWithTTL(redisClient, 5*time.Minute), displaySettingsHandler.GetDisplaySettings)
		v1Boards.PUT("/:slug/display-settings", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			displaySettingsHandler.UpdateDisplaySettings(c)
			// 변경 시 캐시 무효화
			if c.Writer.Status() == http.StatusOK {
				middleware.InvalidateCache(redisClient, "api:cache:")
			}
		})

		// Board extended settings (JSON-based flexible settings)
		// Auto-migrate the extended settings table
		if err := db.AutoMigrate(&v2domain.V2BoardExtendedSettings{}); err != nil {
			fmt.Printf("[WARN] v2_board_extended_settings migration failed: %v\n", err)
		}

		// GET /api/v1/boards/:slug/extended-settings
		v1Boards.GET("/:slug/extended-settings", func(c *gin.Context) {
			slug := c.Param("slug")
			settings, err := v2ExtendedSettingsRepo.FindByBoardSlug(slug)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "설정 조회 실패"}})
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": gin.H{
				"board_id": settings.BoardID,
				"settings": settings.Settings,
			}})
		})

		// PUT /api/v1/boards/:slug/extended-settings (admin only)
		nariyaDataPath := os.Getenv("NARIYA_DATA_PATH")
		if nariyaDataPath == "" {
			nariyaDataPath = "/home/damoang/www/data/nariya/board"
		}
		v1Boards.PUT("/:slug/extended-settings", middleware.JWTAuth(jwtManager), middleware.RequireAdmin(), func(c *gin.Context) {
			slug := c.Param("slug")
			var req struct {
				Settings string `json:"settings" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "요청 형식이 올바르지 않습니다"}})
				return
			}

			settings := &v2domain.V2BoardExtendedSettings{
				BoardID:  slug,
				Settings: req.Settings,
			}
			if err := v2ExtendedSettingsRepo.Upsert(settings); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "설정 저장 실패"}})
				return
			}

			// Regenerate nariya PHP files for PHP/gnuboard compatibility
			if err := v2domain.WriteNariyaPHPFiles(nariyaDataPath, slug, req.Settings); err != nil {
				fmt.Printf("[WARN] nariya PHP sync failed for %s: %v\n", slug, err)
			}

			c.JSON(http.StatusOK, gin.H{"data": gin.H{
				"board_id": settings.BoardID,
				"settings": settings.Settings,
			}})
		})

		// GET /api/v1/boards/:slug/write-permission - Check write permission (auth required)
		v1Boards.GET("/:slug/write-permission", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			slug := c.Param("slug")
			mbID := middleware.GetUserID(c)
			userLevel := middleware.GetUserLevel(c)

			restriction, err := writeRestrictionSvc.Check(slug, mbID, userLevel)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "권한 확인 실패"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "data": restriction})
		})

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

			// 4. 원본 게시글 + 댓글 soft delete (이동 안내 메시지)
			now := time.Now()
			movedBy := middleware.GetUserID(c)
			srcTable := fmt.Sprintf("g5_write_%s", srcBoard)
			tx.Table(srcTable).Where("wr_parent = ? AND wr_is_comment = 1", postID).Updates(map[string]interface{}{
				"wr_deleted_at": now,
				"wr_deleted_by": movedBy,
			})
			tx.Table(srcTable).Where("wr_id = ?", postID).Updates(map[string]interface{}{
				"wr_deleted_at": now,
				"wr_deleted_by": movedBy,
				"wr_content":    fmt.Sprintf("이 게시물은 <a href=\"/%s/%d\">%s 게시판</a>으로 이동되었습니다.", req.TargetBoardID, newPost.WrID, req.TargetBoardID),
			})

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

		// POST /api/v1/boards/:slug/posts/:id/report - Report a post
		v1Boards.POST("/:slug/posts/:id/report", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			slug := c.Param("slug")
			postID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid post ID"})
				return
			}

			userID := middleware.GetUserID(c)
			if userID == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "로그인이 필요합니다"})
				return
			}

			var req struct {
				Reasons []int  `json:"reasons" binding:"required"`
				Detail  string `json:"detail"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "reasons is required"})
				return
			}

			// 게시글 존재 확인 & 작성자 조회
			post, err := gnuWriteRepo.FindPostByID(slug, postID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "게시글을 찾을 수 없습니다"})
				return
			}

			// 자신의 글은 신고 불가
			if post.MbID == userID {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "자신의 글은 신고할 수 없습니다"})
				return
			}

			// 중복 신고 확인 (g5_na_singo 테이블)
			var count int64
			db.Table("g5_na_singo").Where("sg_table = ? AND sg_id = ? AND mb_id = ?", slug, postID, userID).Count(&count)
			if count > 0 {
				c.JSON(http.StatusConflict, gin.H{"success": false, "error": "이미 신고한 게시글입니다"})
				return
			}

			// 신고자 IP
			sgIP := c.ClientIP()

			// 사유별 1행씩 INSERT (nariya 호환: sg_type = reason code)
			now := time.Now()
			for _, reason := range req.Reasons {
				db.Table("g5_na_singo").Create(map[string]interface{}{
					"sg_flag":        0,
					"mb_id":          userID,
					"sg_table":       slug,
					"sg_id":          postID,
					"sg_parent":      postID,
					"sg_type":        reason,
					"sg_desc":        req.Detail,
					"wr_time":        post.WrDatetime,
					"sg_time":        now,
					"sg_ip":          sgIP,
					"target_mb_id":   post.MbID,
					"target_content": post.WrContent,
					"target_title":   post.WrSubject,
				})
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "신고가 접수되었습니다"})
		})

		// POST /api/v1/boards/:slug/posts/:id/comments/:comment_id/report - Report a comment
		v1Boards.POST("/:slug/posts/:id/comments/:comment_id/report", middleware.JWTAuth(jwtManager), func(c *gin.Context) {
			slug := c.Param("slug")
			postID, err := strconv.Atoi(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid post ID"})
				return
			}
			commentID, err := strconv.Atoi(c.Param("comment_id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid comment ID"})
				return
			}

			userID := middleware.GetUserID(c)
			if userID == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "로그인이 필요합니다"})
				return
			}

			var req struct {
				Reasons []int  `json:"reasons" binding:"required"`
				Detail  string `json:"detail"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "reasons is required"})
				return
			}

			// 댓글 존재 확인 & 작성자 조회
			var comment gnuboard.G5Write
			err = db.Table(fmt.Sprintf("g5_write_%s", slug)).
				Where("wr_id = ? AND wr_parent = ? AND wr_is_comment = 1", commentID, postID).
				First(&comment).Error
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "댓글을 찾을 수 없습니다"})
				return
			}

			// 자신의 댓글은 신고 불가
			if comment.MbID == userID {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "자신의 댓글은 신고할 수 없습니다"})
				return
			}

			// 중복 신고 확인 (g5_na_singo 테이블)
			var count int64
			db.Table("g5_na_singo").Where("sg_table = ? AND sg_id = ? AND mb_id = ?", slug, commentID, userID).Count(&count)
			if count > 0 {
				c.JSON(http.StatusConflict, gin.H{"success": false, "error": "이미 신고한 댓글입니다"})
				return
			}

			// 신고자 IP
			sgIP := c.ClientIP()
			now := time.Now()

			// 사유별 1행씩 INSERT (nariya 호환)
			for _, reason := range req.Reasons {
				db.Table("g5_na_singo").Create(map[string]interface{}{
					"sg_flag":        0,
					"mb_id":          userID,
					"sg_table":       slug,
					"sg_id":          commentID,
					"sg_parent":      postID,
					"sg_type":        reason,
					"sg_desc":        req.Detail,
					"wr_time":        comment.WrDatetime,
					"sg_time":        now,
					"sg_ip":          sgIP,
					"target_mb_id":   comment.MbID,
					"target_content": comment.WrContent,
				})
			}

			c.JSON(http.StatusOK, gin.H{"success": true, "message": "신고가 접수되었습니다"})
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

		// ========== Admin Board Management API (g5_board) ==========
		adminBoardGroup := router.Group("/api/v1/admin/boards")
		adminBoardGroup.Use(middleware.JWTAuth(jwtManager), middleware.RequireAdmin())

		// GET /api/v1/admin/boards — 전체 게시판 목록 (관리자용)
		adminBoardGroup.GET("", func(c *gin.Context) {
			boards, err := gnuBoardRepo.FindAll()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "게시판 목록 조회 실패"}})
				return
			}

			// board_type 매핑 (v2_boards에서 조회)
			boardTypes := make(map[string]string)
			var v2BoardRows []struct {
				Slug      string `gorm:"column:slug"`
				BoardType string `gorm:"column:board_type"`
			}
			if err := db.Table("v2_boards").Select("slug, COALESCE(board_type, 'standard') as board_type").Find(&v2BoardRows).Error; err == nil {
				for _, row := range v2BoardRows {
					boardTypes[row.Slug] = row.BoardType
				}
			}

			result := make([]gin.H, 0, len(boards))
			for _, b := range boards {
				resp := b.ToAdminResponse()
				item := gin.H{
					"board_id":       resp.BoardID,
					"group_id":       resp.GroupID,
					"subject":        resp.Subject,
					"admin":          resp.Admin,
					"device":         resp.Device,
					"skin":           resp.Skin,
					"mobile_skin":    resp.MobileSkin,
					"list_level":     resp.ListLevel,
					"read_level":     resp.ReadLevel,
					"write_level":    resp.WriteLevel,
					"reply_level":    resp.ReplyLevel,
					"comment_level":  resp.CommentLevel,
					"upload_level":   resp.UploadLevel,
					"download_level": resp.DownloadLevel,
					"write_point":    resp.WritePoint,
					"comment_point":  resp.CommentPoint,
					"read_point":     resp.ReadPoint,
					"download_point": resp.DownloadPoint,
					"use_category":   resp.UseCategory,
					"category_list":  resp.CategoryList,
					"use_good":       resp.UseGood,
					"use_nogood":     resp.UseNogood,
					"use_secret":     resp.UseSecret,
					"use_sns":        resp.UseSns,
					"page_rows":      resp.PageRows,
					"upload_count":   resp.UploadCount,
					"upload_size":    resp.UploadSize,
					"order":          resp.Order,
					"count_write":    resp.CountWrite,
					"count_comment":  resp.CountComment,
					"notice":         resp.Notice,
				}
				if bt, ok := boardTypes[resp.BoardID]; ok {
					item["board_type"] = bt
				} else {
					item["board_type"] = "standard"
				}
				result = append(result, item)
			}
			c.JSON(http.StatusOK, gin.H{"data": result})
		})

		// GET /api/v1/admin/boards/:boardId — 게시판 상세 (관리자용)
		adminBoardGroup.GET(":boardId", func(c *gin.Context) {
			boardID := c.Param("boardId")
			board, err := gnuBoardRepo.FindByID(boardID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "게시판을 찾을 수 없습니다"}})
				return
			}

			resp := board.ToAdminResponse()

			// board_type from v2_boards
			var boardType string
			if err := db.Table("v2_boards").Select("COALESCE(board_type, 'standard')").Where("slug = ?", boardID).Scan(&boardType).Error; err != nil || boardType == "" {
				boardType = "standard"
			}

			c.JSON(http.StatusOK, gin.H{
				"data": gin.H{
					"board_id":       resp.BoardID,
					"group_id":       resp.GroupID,
					"subject":        resp.Subject,
					"admin":          resp.Admin,
					"device":         resp.Device,
					"skin":           resp.Skin,
					"mobile_skin":    resp.MobileSkin,
					"list_level":     resp.ListLevel,
					"read_level":     resp.ReadLevel,
					"write_level":    resp.WriteLevel,
					"reply_level":    resp.ReplyLevel,
					"comment_level":  resp.CommentLevel,
					"upload_level":   resp.UploadLevel,
					"download_level": resp.DownloadLevel,
					"write_point":    resp.WritePoint,
					"comment_point":  resp.CommentPoint,
					"read_point":     resp.ReadPoint,
					"download_point": resp.DownloadPoint,
					"use_category":   resp.UseCategory,
					"category_list":  resp.CategoryList,
					"use_good":       resp.UseGood,
					"use_nogood":     resp.UseNogood,
					"use_secret":     resp.UseSecret,
					"use_sns":        resp.UseSns,
					"page_rows":      resp.PageRows,
					"upload_count":   resp.UploadCount,
					"upload_size":    resp.UploadSize,
					"order":          resp.Order,
					"count_write":    resp.CountWrite,
					"count_comment":  resp.CountComment,
					"notice":         resp.Notice,
					"board_type":     boardType,
				},
			})
		})

		// POST /api/v1/admin/boards — 게시판 생성
		adminBoardGroup.POST("", func(c *gin.Context) {
			var req struct {
				BoardID       string `json:"board_id" binding:"required"`
				GroupID       string `json:"group_id" binding:"required"`
				Subject       string `json:"subject" binding:"required"`
				BoardType     string `json:"board_type"`
				Skin          string `json:"skin"`
				MobileSkin    string `json:"mobile_skin"`
				PageRows      *int   `json:"page_rows"`
				ListLevel     *int   `json:"list_level"`
				ReadLevel     *int   `json:"read_level"`
				WriteLevel    *int   `json:"write_level"`
				ReplyLevel    *int   `json:"reply_level"`
				CommentLevel  *int   `json:"comment_level"`
				UploadLevel   *int   `json:"upload_level"`
				DownloadLevel *int   `json:"download_level"`
				WritePoint    *int   `json:"write_point"`
				CommentPoint  *int   `json:"comment_point"`
				DownloadPoint *int   `json:"download_point"`
				UseCategory   *int   `json:"use_category"`
				CategoryList  string `json:"category_list"`
				UseGood       *int   `json:"use_good"`
				UseNogood     *int   `json:"use_nogood"`
				UploadCount   *int   `json:"upload_count"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "요청 형식이 올바르지 않습니다"}})
				return
			}

			// 중복 체크
			if gnuBoardRepo.Exists(req.BoardID) {
				c.JSON(http.StatusConflict, gin.H{"error": gin.H{"message": "이미 존재하는 게시판 ID입니다"}})
				return
			}

			board := &gnuboard.G5Board{
				BoTable:        req.BoardID,
				GrID:           req.GroupID,
				BoSubject:      req.Subject,
				BoSkin:         req.Skin,
				BoMobileSkin:   req.MobileSkin,
				BoCategoryList: req.CategoryList,
				BoPageRows:     20,
			}
			if req.PageRows != nil {
				board.BoPageRows = *req.PageRows
			}
			if req.ListLevel != nil {
				board.BoListLevel = *req.ListLevel
			}
			if req.ReadLevel != nil {
				board.BoReadLevel = *req.ReadLevel
			}
			if req.WriteLevel != nil {
				board.BoWriteLevel = *req.WriteLevel
			}
			if req.ReplyLevel != nil {
				board.BoReplyLevel = *req.ReplyLevel
			}
			if req.CommentLevel != nil {
				board.BoCommentLevel = *req.CommentLevel
			}
			if req.UploadLevel != nil {
				board.BoUploadLevel = *req.UploadLevel
			}
			if req.DownloadLevel != nil {
				board.BoDownloadLevel = *req.DownloadLevel
			}
			if req.WritePoint != nil {
				board.BoWritePoint = *req.WritePoint
			}
			if req.CommentPoint != nil {
				board.BoCommentPoint = *req.CommentPoint
			}
			if req.DownloadPoint != nil {
				board.BoDownloadPoint = *req.DownloadPoint
			}
			if req.UseCategory != nil {
				board.BoUseCategory = *req.UseCategory
			}
			if req.UseGood != nil {
				board.BoUseGood = *req.UseGood
			}
			if req.UseNogood != nil {
				board.BoUseNogood = *req.UseNogood
			}
			// req.UploadCount: bo_num_list_count 컬럼은 DB에 없으므로 무시

			if err := gnuBoardRepo.Create(board); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "게시판 생성 실패: " + err.Error()}})
				return
			}

			// g5_write_{board_id} 테이블 생성
			createTableSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS g5_write_%s LIKE g5_write_free`, req.BoardID)
			if err := db.Exec(createTableSQL).Error; err != nil {
				// 테이블 생성 실패해도 board는 생성됨 — 로그만 남김
				fmt.Printf("[WARN] Failed to create write table for %s: %v\n", req.BoardID, err)
			}

			// v2_boards에 board_type 저장
			if req.BoardType != "" && req.BoardType != "standard" {
				db.Exec("INSERT INTO v2_boards (slug, name, board_type, is_active) VALUES (?, ?, ?, 1) ON DUPLICATE KEY UPDATE board_type = VALUES(board_type)",
					req.BoardID, req.Subject, req.BoardType)
			}

			c.JSON(http.StatusCreated, gin.H{"data": board.ToAdminResponse()})
		})

		// PUT /api/v1/admin/boards/:boardId — 게시판 수정
		adminBoardGroup.PUT(":boardId", func(c *gin.Context) {
			boardID := c.Param("boardId")
			board, err := gnuBoardRepo.FindByID(boardID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "게시판을 찾을 수 없습니다"}})
				return
			}

			var req struct {
				GroupID       *string `json:"group_id"`
				Subject       *string `json:"subject"`
				Admin         *string `json:"admin"`
				BoardType     *string `json:"board_type"`
				Skin          *string `json:"skin"`
				MobileSkin    *string `json:"mobile_skin"`
				PageRows      *int    `json:"page_rows"`
				ListLevel     *int    `json:"list_level"`
				ReadLevel     *int    `json:"read_level"`
				WriteLevel    *int    `json:"write_level"`
				ReplyLevel    *int    `json:"reply_level"`
				CommentLevel  *int    `json:"comment_level"`
				UploadLevel   *int    `json:"upload_level"`
				DownloadLevel *int    `json:"download_level"`
				WritePoint    *int    `json:"write_point"`
				CommentPoint  *int    `json:"comment_point"`
				ReadPoint     *int    `json:"read_point"`
				DownloadPoint *int    `json:"download_point"`
				UseCategory   *int    `json:"use_category"`
				CategoryList  *string `json:"category_list"`
				UseGood       *int    `json:"use_good"`
				UseNogood     *int    `json:"use_nogood"`
				UseSecret     *int    `json:"use_secret"`
				UseSns        *int    `json:"use_sns"`
				UploadCount   *int    `json:"upload_count"`
				Order         *int    `json:"order"`
				Notice        *string `json:"notice"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "요청 형식이 올바르지 않습니다"}})
				return
			}

			if req.GroupID != nil {
				board.GrID = *req.GroupID
			}
			if req.Subject != nil {
				board.BoSubject = *req.Subject
			}
			if req.Admin != nil {
				board.BoAdmin = *req.Admin
			}
			if req.Skin != nil {
				board.BoSkin = *req.Skin
			}
			if req.MobileSkin != nil {
				board.BoMobileSkin = *req.MobileSkin
			}
			if req.PageRows != nil {
				board.BoPageRows = *req.PageRows
			}
			if req.ListLevel != nil {
				board.BoListLevel = *req.ListLevel
			}
			if req.ReadLevel != nil {
				board.BoReadLevel = *req.ReadLevel
			}
			if req.WriteLevel != nil {
				board.BoWriteLevel = *req.WriteLevel
			}
			if req.ReplyLevel != nil {
				board.BoReplyLevel = *req.ReplyLevel
			}
			if req.CommentLevel != nil {
				board.BoCommentLevel = *req.CommentLevel
			}
			if req.UploadLevel != nil {
				board.BoUploadLevel = *req.UploadLevel
			}
			if req.DownloadLevel != nil {
				board.BoDownloadLevel = *req.DownloadLevel
			}
			if req.WritePoint != nil {
				board.BoWritePoint = *req.WritePoint
			}
			if req.CommentPoint != nil {
				board.BoCommentPoint = *req.CommentPoint
			}
			if req.ReadPoint != nil {
				board.BoReadPoint = *req.ReadPoint
			}
			if req.DownloadPoint != nil {
				board.BoDownloadPoint = *req.DownloadPoint
			}
			if req.UseCategory != nil {
				board.BoUseCategory = *req.UseCategory
			}
			if req.CategoryList != nil {
				board.BoCategoryList = *req.CategoryList
			}
			if req.UseGood != nil {
				board.BoUseGood = *req.UseGood
			}
			if req.UseNogood != nil {
				board.BoUseNogood = *req.UseNogood
			}
			if req.UseSecret != nil {
				board.BoUseSecret = *req.UseSecret
			}
			if req.UseSns != nil {
				board.BoUseSns = *req.UseSns
			}
			// req.UploadCount: bo_num_list_count 컬럼은 DB에 없으므로 무시
			if req.Order != nil {
				board.BoOrder = *req.Order
			}
			if req.Notice != nil {
				board.BoNotice = *req.Notice
			}

			if err := gnuBoardRepo.Update(board); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "게시판 수정 실패"}})
				return
			}

			// v2_boards board_type 업데이트
			if req.BoardType != nil {
				db.Exec("INSERT INTO v2_boards (slug, name, board_type, is_active) VALUES (?, ?, ?, 1) ON DUPLICATE KEY UPDATE board_type = VALUES(board_type)",
					boardID, board.BoSubject, *req.BoardType)
			}

			// 캐시 무효화
			if cacheService != nil {
				_ = cacheService.InvalidateBoard(c.Request.Context(), boardID)
			}

			c.JSON(http.StatusOK, gin.H{"data": board.ToAdminResponse()})
		})

		// DELETE /api/v1/admin/boards/:boardId — 게시판 삭제
		adminBoardGroup.DELETE(":boardId", func(c *gin.Context) {
			boardID := c.Param("boardId")
			if !gnuBoardRepo.Exists(boardID) {
				c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "게시판을 찾을 수 없습니다"}})
				return
			}

			if err := gnuBoardRepo.Delete(boardID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "게시판 삭제 실패"}})
				return
			}

			// 캐시 무효화
			if cacheService != nil {
				_ = cacheService.InvalidateBoard(c.Request.Context(), boardID)
			}

			c.JSON(http.StatusOK, gin.H{"data": gin.H{"message": "삭제 완료"}})
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

		// Admin Settings (report-lock threshold etc.)
		adminSettingsHandler := v2handler.NewAdminSettingsHandler(db)
		v2routes.SetupAdminSettings(router, adminSettingsHandler, jwtManager)

		// v2 Scrap, Memo, Block, Message
		v2ScrapRepo := v2repo.NewScrapRepository(db)
		v2MemoRepo := v2repo.NewMemoRepository(db)
		v2BlockRepo := v2repo.NewBlockRepository(db)
		v2MessageRepo := v2repo.NewMessageRepository(db)
		v2routes.SetupScrap(router, v2handler.NewScrapHandler(v2ScrapRepo), jwtManager)
		v2routes.SetupMemo(router, v2handler.NewMemoHandler(v2MemoRepo), jwtManager)
		v2routes.SetupBlock(router, v2handler.NewBlockHandler(v2BlockRepo, cacheService), jwtManager)
		v2routes.SetupMessage(router, v2handler.NewMessageHandler(v2MessageRepo), jwtManager, db)
		v2routes.SetupFavorite(router, v2handler.NewFavoriteHandler(db), jwtManager)

		// v1 message routes (uses g5_memo table directly)
		gnuMemoRepo := gnurepo.NewMemoRepository(db)
		v1MsgHandler := handler.NewV1MessageHandler(gnuMemoRepo, gnuMemberRepo, notiRepo)
		v1Messages := router.Group("/api/v1/messages", middleware.JWTAuth(jwtManager))
		v1Messages.GET("", v1MsgHandler.GetMessages)
		v1Messages.GET("/unread-count", v1MsgHandler.GetUnreadCount)
		v1Messages.GET("/:id", v1MsgHandler.GetMessage)
		v1Messages.POST("", banCheck, v1MsgHandler.SendMessage)
		v1Messages.DELETE("/:id", v1MsgHandler.DeleteMessage)

		// Banner, Promotion, License (v1 + v2 dual routes)
		bannerRepo := v2repo.NewBannerRepository(db)
		v2routes.SetupBanner(router, v2handler.NewBannerHandler(bannerRepo))

		promotionRepo := v2repo.NewPromotionRepository(db)
		v2routes.SetupPromotion(router, v2handler.NewPromotionHandler(promotionRepo, redisClient))

		v2routes.SetupLicense(router, v2handler.NewLicenseHandler())

		// Content pages (g5_content)
		contentRepo := v2repo.NewContentRepository(db)
		contentHandler := v2handler.NewContentHandler(contentRepo)
		v2routes.SetupContent(router, contentHandler, jwtManager)

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

			// v1 검색 라우트 (프론트엔드 호환)
			searchV1 := router.Group("/api/v1/search")
			searchV1.GET("", searchHandler.Search)
			searchV1.GET("/autocomplete", searchHandler.Autocomplete)

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

			// Member profile image
			memberSvc := service.NewMemberService(s3Client, gnuMemberRepo)
			memberHandler := handler.NewMemberHandler(memberSvc)
			memberImage := router.Group("/api/v2/members/me", middleware.JWTAuth(jwtManager))
			memberImage.POST("/image", memberHandler.UploadImage)
			memberImage.DELETE("/image", memberHandler.DeleteImage)
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
				cntColumn: gorm.Expr(cntColumn + " + 1"),
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

		// Giving plugin API
		givingHandler := handler.NewGivingHandler(db)
		givingGroup := router.Group("/api/plugins/giving")
		{
			givingGroup.GET("/list", givingHandler.List)
		}

		// Internal cron endpoints (curl-based cron jobs)
		cronHandler := cron.NewHandler(db)
		cronHandler.SetPointExpiryDeps(pointConfigRepo, gnuPointWriteRepo, gnurepo.NewNotiRepository(db))
		cronGroup := router.Group("/api/internal/cron")
		cronGroup.POST("/member-lock-release", cronHandler.MemberLockRelease)
		cronGroup.POST("/update-member-levels", cronHandler.UpdateMemberLevels)
		cronGroup.POST("/process-approved-reports", cronHandler.ProcessApprovedReports)
		cronGroup.POST("/update-report-pattern", cronHandler.UpdateReportPattern)
		cronGroup.POST("/discipline-release", cronHandler.DisciplineRelease)
		cronGroup.POST("/point-expiry", cronHandler.PointExpiry)
		cronGroup.POST("/point-expiry-notify", cronHandler.PointExpiryNotify)
		cronGroup.POST("/auto-promote", cronHandler.AutoPromote)

		// Start delete worker for delayed deletion processing
		deleteWorker := worker.NewDeleteWorker(gnuWriteRepo, scheduledDeleteRepo)
		deleteWorker.Start()
		defer deleteWorker.Stop()
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

// maskIP masks the second octet of an IPv4 address with ♡ (e.g. 222.114.55.158 → 222.♡.55.158)
func maskIP(ip string) string {
	if ip == "" {
		return ""
	}
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		parts[1] = "♡"
		return strings.Join(parts, ".")
	}
	if len(ip) > 3 {
		return ip[:3] + ".♡"
	}
	return "♡"
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
	mysqlCfg.InterpolateParams = true // prepared statement 누적 방지 (RDS max_prepared_stmt_count 한계)

	// 프로덕션: Warn (SQL 로깅 비활성화로 I/O 대폭 감소)
	// 개발: Info (디버깅용 SQL 로깅)
	logLevel := gormlogger.Warn
	if appEnv := os.Getenv("APP_ENV"); appEnv == "" || appEnv == "local" || appEnv == "development" {
		logLevel = gormlogger.Info
	}
	db, err := gorm.Open(mysql.Open(mysqlCfg.FormatDSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(logLevel),
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
	// ConnMaxLifetime: 풀에서 연결 최대 수명. 짧을수록 stale connection 위험 감소.
	// k3s 재시작 등으로 TCP 끊김 시 빠른 복구를 위해 5분 권장.
	connMaxLifetime := time.Duration(cfg.Database.ConnMaxLifetime) * time.Second
	if connMaxLifetime > 5*time.Minute {
		connMaxLifetime = 5 * time.Minute
	}
	sqlDB.SetConnMaxLifetime(connMaxLifetime)
	sqlDB.SetConnMaxIdleTime(2 * time.Minute)

	return db, nil
}
