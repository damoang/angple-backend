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
	v2handler "github.com/damoang/angple-backend/internal/handler/v2"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/migration"
	"github.com/damoang/angple-backend/internal/plugin"
	pluginstoreHandler "github.com/damoang/angple-backend/internal/pluginstore/handler"
	pluginstoreRepo "github.com/damoang/angple-backend/internal/pluginstore/repository"
	pluginstoreSvc "github.com/damoang/angple-backend/internal/pluginstore/service"
	"github.com/damoang/angple-backend/internal/repository"
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

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
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
	_ = godotenv.Load() //nolint:errcheck

	// 로거 초기화
	pkglogger.Init()
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}
	pkglogger.InitStructured(env)
	pkglogger.Info("Starting Angple API Server...")

	// 설정 로드
	configPath := getConfigPath()
	pkglogger.Info("Loading config from: %s", configPath)
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

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
	_ = cacheService

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
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
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

	if redisClient != nil {
		router.Use(middleware.RateLimit(redisClient, middleware.DefaultRateLimitConfig()))
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

		// v2 Core API
		v2PostRepo := v2repo.NewPostRepository(db)
		v2CommentRepo := v2repo.NewCommentRepository(db)
		v2BoardRepo := v2repo.NewBoardRepository(db)
		v2Handler := v2handler.NewV2Handler(v2UserRepo, v2PostRepo, v2CommentRepo, v2BoardRepo)
		v2routes.Setup(router, v2Handler, jwtManager)

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
			c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
		})
		router.GET("/api/v1/boards/:slug/notices", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
		})
		router.GET("/api/ads/celebration/today", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
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
		// v1 boards routes → adapt v2 data to v1 format
		v1Boards := router.Group("/api/v1/boards")
		v1Boards.GET("/:slug", v2Handler.GetBoard)
		v1Boards.GET("/:slug/posts", func(c *gin.Context) {
			slug := c.Param("slug")
			board, err := v2BoardRepo.FindBySlug(slug)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}, "meta": gin.H{"total": 0, "page": 1, "limit": 20}})
				return
			}
			page, err2 := strconv.Atoi(c.DefaultQuery("page", "1"))
			if err2 != nil {
				page = 1
			}
			limit, err3 := strconv.Atoi(c.DefaultQuery("limit", "20"))
			if err3 != nil {
				limit = 20
			}
			if page < 1 {
				page = 1
			}
			if limit < 1 || limit > 100 {
				limit = 20
			}
			posts, total, err := v2PostRepo.FindByBoard(board.ID, page, limit)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}, "meta": gin.H{"total": 0, "page": page, "limit": limit}})
				return
			}
			// batch-load user nicknames
			userIDs := make([]string, 0, len(posts))
			for _, p := range posts {
				userIDs = append(userIDs, fmt.Sprintf("%d", p.UserID))
			}
			nickMap, err4 := v2UserRepo.FindNicksByIDs(userIDs)
			if err4 != nil {
				nickMap = map[string]string{}
			}
			// transform to v1 format
			items := make([]gin.H, 0, len(posts))
			for _, p := range posts {
				uid := fmt.Sprintf("%d", p.UserID)
				author := nickMap[uid]
				if author == "" {
					author = "익명"
				}
				items = append(items, gin.H{
					"id":             p.ID,
					"title":          p.Title,
					"content":        p.Content,
					"author":         author,
					"author_id":      uid,
					"views":          p.ViewCount,
					"likes":          0,
					"comments_count": p.CommentCount,
					"is_notice":      p.IsNotice,
					"created_at":     p.CreatedAt,
					"updated_at":     p.UpdatedAt,
				})
			}
			c.JSON(http.StatusOK, gin.H{
				"data": items,
				"meta": gin.H{"board_id": slug, "page": page, "limit": limit, "total": total},
			})
		})
		v1Boards.GET("/:slug/posts/:id", func(c *gin.Context) {
			id, err := strconv.ParseUint(c.Param("id"), 10, 64)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
				return
			}
			post, err := v2PostRepo.FindByID(id)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
				return
			}
			if vcErr := v2PostRepo.IncrementViewCount(id); vcErr != nil {
				log.Printf("IncrementViewCount error: %v", vcErr)
			}
			uid := fmt.Sprintf("%d", post.UserID)
			nickMap, nickErr := v2UserRepo.FindNicksByIDs([]string{uid})
			if nickErr != nil {
				nickMap = map[string]string{}
			}
			author := nickMap[uid]
			if author == "" {
				author = "익명"
			}
			c.JSON(http.StatusOK, gin.H{
				"data": gin.H{
					"id":             post.ID,
					"title":          post.Title,
					"content":        post.Content,
					"author":         author,
					"author_id":      uid,
					"views":          post.ViewCount,
					"likes":          0,
					"comments_count": post.CommentCount,
					"is_notice":      post.IsNotice,
					"created_at":     post.CreatedAt,
					"updated_at":     post.UpdatedAt,
				},
			})
		})
		v1Boards.GET("/:slug/posts/:id/comments", v2Handler.ListComments)

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

			media := router.Group("/api/v2/media", middleware.JWTAuth(jwtManager))
			media.POST("/images", mediaHandler.UploadImage)
			media.POST("/attachments", mediaHandler.UploadAttachment)
			media.POST("/videos", mediaHandler.UploadVideo)
			media.DELETE("/files", mediaHandler.DeleteFile)
		}

		// WebSocket
		wsHandler := handler.NewWSHandler(wsHub, cfg.CORS.AllowOrigins)
		router.GET("/ws/notifications", middleware.JWTAuth(jwtManager), wsHandler.Connect)

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
