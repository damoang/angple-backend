package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/damoang/angple-backend/docs" // swagger docs
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
	"github.com/damoang/angple-backend/internal/routes"
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

	// 플러그인 자동 등록을 위한 import (init()에서 Factory 등록됨)
	_ "github.com/damoang/angple-backend/internal/plugins/advertising"
	_ "github.com/damoang/angple-backend/internal/plugins/commerce"
	_ "github.com/damoang/angple-backend/internal/plugins/embed"
	_ "github.com/damoang/angple-backend/internal/plugins/imagelink"
	_ "github.com/damoang/angple-backend/internal/plugins/marketplace"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
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
// @description     다모앙(damoang.net) 커뮤니티 백엔드 API 서버
// @description     기존 PHP(그누보드) 기반 시스템을 Go로 마이그레이션한 프로젝트
//
// @contact.name    SDK
// @contact.email   sdk@damoang.net
//
// @license.name    Proprietary
//
// @host            localhost:8082
// @BasePath        /api/v1
//
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description JWT Authorization header using the Bearer scheme. Example: "Bearer {token}"
//
// @tag.name auth
// @tag.description 인증 관련 API (로그인, 토큰 관리)
//
// @tag.name boards
// @tag.description 게시판 관리 API
//
// @tag.name posts
// @tag.description 게시글 CRUD API
//
// @tag.name comments
// @tag.description 댓글 CRUD API
//
// @tag.name menus
// @tag.description 메뉴 조회 API
//
// @tag.name recommended
// @tag.description 추천 게시물 API (AI 분석 포함)
//
// @tag.name site
// @tag.description 사이트 설정 API

// getConfigPath returns config file path based on APP_ENV environment variable
func getConfigPath() string {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "local" // 기본값: 로컬 개발 환경
	}
	return fmt.Sprintf("configs/config.%s.yaml", env)
}

func main() {
	// .env 파일 로드 (없어도 에러 무시)
	_ = godotenv.Load() //nolint:errcheck // .env 파일이 없어도 정상 동작

	// 로거 초기화
	pkglogger.Init()
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}
	pkglogger.InitStructured(env)
	pkglogger.Info("Starting Angple API Server...")

	// 설정 로드 (APP_ENV 환경변수로 config 파일 선택)
	configPath := getConfigPath()
	pkglogger.Info("Loading config from: %s", configPath)
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// MySQL 연결
	db, err := initDB(cfg)
	if err != nil {
		pkglogger.Info("⚠️  Warning: Failed to connect to database: %v (continuing without DB)", err)
		pkglogger.Info("⚠️  Health check will work, but API endpoints will fail")
		db = nil
	} else {
		pkglogger.Info("✅ Connected to MySQL")
		if err := migration.Run(db); err != nil {
			pkglogger.Info("⚠️  Migration warning: %v", err)
		}
		// v2 스키마 생성 (v2_ 접두사 테이블, 기존 g5_* 와 공존)
		if err := migration.RunV2Schema(db); err != nil {
			pkglogger.Info("⚠️  V2 schema migration warning: %v", err)
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
		pkglogger.Info("⚠️  Warning: Failed to connect to Redis: %v (continuing without Redis)", err)
		redisClient = nil
	} else {
		pkglogger.Info("✅ Connected to Redis")
	}

	// Cache Service 초기화
	var cacheService pkgcache.Service
	if redisClient != nil {
		cacheService = pkgcache.NewService(redisClient)
		pkglogger.Info("✅ Cache service initialized")
	}

	// Elasticsearch 연결
	var esClient *pkges.Client
	if cfg.Elasticsearch.Enabled && len(cfg.Elasticsearch.Addresses) > 0 {
		var esErr error
		esClient, esErr = pkges.NewClient(cfg.Elasticsearch.Addresses, cfg.Elasticsearch.Username, cfg.Elasticsearch.Password)
		if esErr != nil {
			pkglogger.Info("⚠️  Warning: Elasticsearch connection failed: %v (continuing without ES)", esErr)
			esClient = nil
		} else {
			pkglogger.Info("✅ Connected to Elasticsearch")
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
			pkglogger.Info("⚠️  Warning: S3 storage init failed: %v (continuing without S3)", s3Err)
			s3Client = nil
		} else {
			pkglogger.Info("✅ Connected to S3 storage")
		}
	}

	// WebSocket Hub (Redis Pub/Sub for multi-instance)
	wsHub := ws.NewHub(redisClient)
	go wsHub.Run()

	// DI Container: Repository -> Service -> Handler

	// JWT Manager
	jwtManager := jwt.NewManager(
		cfg.JWT.Secret,
		cfg.JWT.ExpiresIn,
		cfg.JWT.RefreshIn,
	)

	// Damoang JWT Manager (for damoang_jwt cookie verification)
	damoangSecret := cfg.JWT.DamoangSecret
	if damoangSecret == "" {
		log.Fatal("DAMOANG_JWT_SECRET environment variable is required")
	}
	damoangJWT := jwt.NewDamoangManager(damoangSecret)

	// Plugin HookManager (생성만 먼저, 플러그인 활성화는 나중에)
	pluginLogger := plugin.NewDefaultLogger("plugin")
	hookManager := plugin.NewHookManager(pluginLogger)

	// DI Container (skip if no DB connection)
	var authHandler *handler.AuthHandler
	var postHandler *handler.PostHandler
	var commentHandler *handler.CommentHandler
	var menuHandler *handler.MenuHandler
	var siteHandler *handler.SiteHandler
	var boardHandler *handler.BoardHandler
	var memberHandler *handler.MemberHandler
	var autosaveHandler *handler.AutosaveHandler
	var filterHandler *handler.FilterHandler
	var tokenHandler *handler.TokenHandler
	var memoHandler *handler.MemoHandler
	var reactionHandler *handler.ReactionHandler
	var reportHandler *handler.ReportHandler
	var dajoongiHandler *handler.DajoongiHandler
	var promotionHandler *handler.PromotionHandler
	var bannerHandler *handler.BannerHandler
	var goodHandler *handler.GoodHandler
	var notificationHandler *handler.NotificationHandler
	var memberProfileHandler *handler.MemberProfileHandler
	var fileHandler *handler.FileHandler
	var scrapHandler *handler.ScrapHandler
	var blockHandler *handler.BlockHandler
	var messageHandler *handler.MessageHandler
	var wsHandler *handler.WSHandler
	var disciplineHandler *handler.DisciplineHandler
	var galleryHandler *handler.GalleryHandler
	var adminHandler *handler.AdminHandler
	var tenantHandler *handler.TenantHandler
	var provisioningHandler *handler.ProvisioningHandler
	var recommendationHandler *handler.RecommendationHandler
	var auditLogger *middleware.AuditLogger
	var oauthHandler *handler.OAuthHandler
	var oauthService *service.OAuthService
	var searchHandler *handler.SearchHandler
	var mediaHandler *handler.MediaHandler
	var paymentHandler *handler.PaymentHandler
	var boardPermissionChecker middleware.BoardPermissionChecker

	if db != nil {
		// Repositories
		memberRepo := repository.NewMemberRepository(db)
		postRepo := repository.NewPostRepository(db)
		commentRepo := repository.NewCommentRepository(db)
		menuRepo := repository.NewMenuRepository(db)
		siteRepo := repository.NewSiteRepository(db)
		boardRepo := repository.NewBoardRepository(db)
		autosaveRepo := repository.NewAutosaveRepository(db)
		dajoongiRepo := repository.NewDajoongiRepository(db)
		memoRepo := repository.NewMemoRepository(db)
		reactionRepo := repository.NewReactionRepository(db)
		reportRepo := repository.NewReportRepository(db)
		promotionRepo := repository.NewPromotionRepository(db)
		bannerRepo := repository.NewBannerRepository(db)
		goodRepo := repository.NewGoodRepository(db)
		notificationRepo := repository.NewNotificationRepository(db)
		pointRepo := repository.NewPointRepository(db)
		fileRepo := repository.NewFileRepository(db)
		scrapRepo := repository.NewScrapRepository(db)
		blockRepo := repository.NewBlockRepository(db)
		messageRepo := repository.NewMessageRepository(db)
		disciplineRepo := repository.NewDisciplineRepository(db)
		galleryRepo := repository.NewGalleryRepository(db)

		// Services
		authService := service.NewAuthService(memberRepo, jwtManager, hookManager, cacheService)
		postService := service.NewPostService(postRepo, hookManager)
		commentService := service.NewCommentService(commentRepo, goodRepo, hookManager)
		menuService := service.NewMenuService(menuRepo)
		siteService := service.NewSiteService(siteRepo)
		boardService := service.NewBoardService(boardRepo, cacheService)
		boardPermissionChecker = boardService // implements middleware.BoardPermissionChecker
		memberValidationService := service.NewMemberValidationService(memberRepo)
		autosaveService := service.NewAutosaveService(autosaveRepo)
		memoService := service.NewMemoService(memoRepo, memberRepo)
		reactionService := service.NewReactionService(reactionRepo)
		reportService := service.NewReportService(reportRepo)
		promotionService := service.NewPromotionService(promotionRepo)
		bannerService := service.NewBannerService(bannerRepo)
		goodService := service.NewGoodService(goodRepo)
		notificationService := service.NewNotificationService(notificationRepo, wsHub)
		memberProfileService := service.NewMemberProfileService(memberRepo, pointRepo, db, cacheService)

		scrapService := service.NewScrapService(scrapRepo)
		blockService := service.NewBlockService(blockRepo, memberRepo)
		messageService := service.NewMessageService(messageRepo, memberRepo, blockRepo)
		disciplineService := service.NewDisciplineService(disciplineRepo)
		galleryService := service.NewGalleryService(galleryRepo, redisClient)
		adminMemberService := service.NewAdminMemberService(memberRepo, db)

		// File upload path
		uploadPath := cfg.DataPaths.UploadPath
		if uploadPath == "" {
			uploadPath = "/home/damoang/www/data/file"
		}
		fileService := service.NewFileService(fileRepo, uploadPath)

		// Handlers
		authHandler = handler.NewAuthHandler(authService, cfg)
		postHandler = handler.NewPostHandler(postService, boardRepo)
		commentHandler = handler.NewCommentHandler(commentService)
		menuHandler = handler.NewMenuHandler(menuService)
		siteHandler = handler.NewSiteHandler(siteService)
		boardHandler = handler.NewBoardHandler(boardService)
		memberHandler = handler.NewMemberHandler(memberValidationService, memberRepo)
		autosaveHandler = handler.NewAutosaveHandler(autosaveService)
		filterHandler = handler.NewFilterHandler(nil) // TODO: Load filter words from DB
		tokenHandler = handler.NewTokenHandler()
		memoHandler = handler.NewMemoHandler(memoService)
		reactionHandler = handler.NewReactionHandler(reactionService)
		reportHandler = handler.NewReportHandler(reportService)
		dajoongiHandler = handler.NewDajoongiHandler(dajoongiRepo)
		promotionHandler = handler.NewPromotionHandler(promotionService)
		bannerHandler = handler.NewBannerHandler(bannerService)
		goodHandler = handler.NewGoodHandler(goodService)
		notificationHandler = handler.NewNotificationHandler(notificationService)
		memberProfileHandler = handler.NewMemberProfileHandler(memberProfileService)
		fileHandler = handler.NewFileHandler(fileService)
		scrapHandler = handler.NewScrapHandler(scrapService)
		blockHandler = handler.NewBlockHandler(blockService)
		messageHandler = handler.NewMessageHandler(messageService)
		wsHandler = handler.NewWSHandler(wsHub, cfg.CORS.AllowOrigins)
		disciplineHandler = handler.NewDisciplineHandler(disciplineService)
		galleryHandler = handler.NewGalleryHandler(galleryService)
		adminHandler = handler.NewAdminHandler(adminMemberService)

		// Tenant Management
		tenantDBResolver := middleware.NewTenantDBResolver(db)
		tenantSvc := service.NewTenantService(siteRepo, db, tenantDBResolver)
		tenantHandler = handler.NewTenantHandler(tenantSvc)

		// SaaS Provisioning
		subRepo := repository.NewSubscriptionRepository(db)
		if err := subRepo.AutoMigrate(); err != nil {
			log.Printf("warning: subscription AutoMigrate failed: %v", err)
		}
		provisioningSvc := service.NewProvisioningService(siteRepo, subRepo, tenantDBResolver, db, "angple.com")
		provisioningHandler = handler.NewProvisioningHandler(provisioningSvc)

		// AI Recommendation
		recRepo := repository.NewRecommendationRepository(db)
		if err := recRepo.AutoMigrate(); err != nil {
			log.Printf("warning: recommendation AutoMigrate failed: %v", err)
		}
		recSvc := service.NewRecommendationService(recRepo, db, cacheService)
		recommendationHandler = handler.NewRecommendationHandler(recSvc)

		// Audit Logger
		auditLogger = middleware.NewAuditLogger(db)

		// OAuth Service + Handler
		oauthService = service.NewOAuthService(db, jwtManager)
		// Register providers from environment variables
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
		oauthHandler = handler.NewOAuthHandler(oauthService)

		// Elasticsearch Search (optional)
		if esClient != nil {
			searchSvc := service.NewSearchService(esClient, db)
			searchHandler = handler.NewSearchHandler(searchSvc)
		}

		// Media Pipeline (S3 storage)
		if s3Client != nil {
			mediaSvc := service.NewMediaService(s3Client)
			mediaHandler = handler.NewMediaHandler(mediaSvc)
		}

		// Payment Service (Toss + Stripe)
		paymentRepo := repository.NewPaymentRepository(db)
		subRepo2 := repository.NewSubscriptionRepository(db)
		paymentSvc := service.NewPaymentService(paymentRepo, subRepo2, db, service.PaymentConfig{
			TossSecretKey:   os.Getenv("TOSS_SECRET_KEY"),
			StripeSecretKey: os.Getenv("STRIPE_SECRET_KEY"),
		})
		paymentHandler = handler.NewPaymentHandler(paymentSvc)
	}

	// Recommended Handler (파일 직접 읽기)
	recommendedPath := cfg.DataPaths.RecommendedPath
	if recommendedPath == "" {
		recommendedPath = "/home/damoang/www/data/cache/recommended"
	}
	recommendedHandler := handler.NewRecommendedHandler(recommendedPath)

	// Gin 라우터 생성
	router := gin.Default() // Recovery와 Logger 미들웨어 포함

	// CORS 설정 (config에서 읽어오거나 운영 기본값 사용)
	allowOrigins := cfg.CORS.AllowOrigins
	if allowOrigins == "" {
		// 운영 환경 기본값: 운영 도메인만 허용
		allowOrigins = "https://web.damoang.net, https://damoang.net"
	}

	corsConfig := cors.Config{
		AllowOrigins:     []string{allowOrigins},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-API-Key", "X-CSRF-Token", "X-Request-ID"},
		AllowCredentials: true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		ExposeHeaders:    []string{"X-Request-ID", "X-RateLimit-Remaining", "X-Cache"},
		MaxAge:           86400,
	}
	// CORS가 단일 origin 문자열인 경우 처리
	if len(corsConfig.AllowOrigins) == 1 && corsConfig.AllowOrigins[0] != "" {
		// 쉼표로 구분된 여러 origin 처리
		corsConfig.AllowOrigins = splitAndTrim(allowOrigins, ",")
	}
	router.Use(cors.New(corsConfig))

	// i18n Bundle (메시지 번들 초기화)
	i18nBundle := i18n.NewBundle(i18n.LocaleKo)
	for locale, msgs := range i18n.DefaultMessages() {
		i18nBundle.LoadMessages(locale, msgs)
	}
	// JSON 파일이 있으면 오버라이드
	if _, err := os.Stat("i18n"); err == nil {
		if err := i18nBundle.LoadDir("i18n"); err != nil {
			log.Printf("warning: i18n LoadDir failed: %v", err)
		}
	}
	_ = i18nBundle // available for handlers via context

	// i18n middleware (Accept-Language 감지)
	router.Use(middleware.I18n())

	// Security middleware
	router.Use(middleware.SecurityHeaders())
	router.Use(middleware.InputSanitizer())

	// Observability middleware
	router.Use(middleware.Metrics())
	router.Use(middleware.RequestLogger())

	// Global Rate Limiter (IP별 120 req/min)
	if redisClient != nil {
		router.Use(middleware.RateLimit(redisClient, middleware.DefaultRateLimitConfig()))
	}

	// Prometheus metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Health Check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"time":   time.Now().Unix(),
		})
	})

	// Swagger UI
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// v1(레거시) API 사용량 추적기
	v1UsageTracker := middleware.NewAPIUsageTracker()

	// v1 사용량 모니터링 엔드포인트 (관리자용)
	router.GET("/api/v2/admin/v1-usage", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"tracking_since": v1UsageTracker.StartedAt(),
			"total_calls":    v1UsageTracker.TotalCalls(),
			"endpoints":      v1UsageTracker.GetStats(),
		})
	})
	router.POST("/api/v2/admin/v1-usage/reset", func(c *gin.Context) {
		v1UsageTracker.Reset()
		c.JSON(http.StatusOK, gin.H{"message": "사용량 카운터 초기화 완료"})
	})

	// 라우트 등록 (only if DB is connected)
	if db != nil {
		// v1 레거시 API (그누보드 DB 기반) → /api/v1
		routes.Setup(router, postHandler, commentHandler, authHandler, menuHandler, siteHandler, boardHandler, memberHandler, autosaveHandler, filterHandler, tokenHandler, memoHandler, reactionHandler, reportHandler, dajoongiHandler, promotionHandler, bannerHandler, jwtManager, damoangJWT, goodHandler, recommendedHandler, notificationHandler, memberProfileHandler, fileHandler, scrapHandler, blockHandler, messageHandler, wsHandler, disciplineHandler, galleryHandler, adminHandler, v1UsageTracker, cfg, boardPermissionChecker)

		// v2 API (v2_ 테이블 기반) → /api/v2
		v2UserRepo := v2repo.NewUserRepository(db)
		v2PostRepo := v2repo.NewPostRepository(db)
		v2CommentRepo := v2repo.NewCommentRepository(db)
		v2BoardRepo := v2repo.NewBoardRepository(db)
		v2Handler := v2handler.NewV2Handler(v2UserRepo, v2PostRepo, v2CommentRepo, v2BoardRepo)
		v2routes.Setup(router, v2Handler, jwtManager)

		// v2 Auth API
		v2AuthSvc := v2svc.NewV2AuthService(v2UserRepo, jwtManager, damoangJWT)
		v2AuthHandler := v2handler.NewV2AuthHandler(v2AuthSvc)
		v2routes.SetupAuth(router, v2AuthHandler, jwtManager)

		// v2 Admin API
		v2AdminSvc := v2svc.NewAdminService(v2UserRepo, v2BoardRepo, v2PostRepo, v2CommentRepo)
		v2AdminHandler := v2handler.NewAdminHandler(v2AdminSvc)
		v2routes.SetupAdmin(router, v2AdminHandler, jwtManager)

		// v2 Scrap, Memo, Block, Message
		v2ScrapRepo := v2repo.NewScrapRepository(db)
		v2MemoRepo := v2repo.NewMemoRepository(db)
		v2BlockRepo := v2repo.NewBlockRepository(db)
		v2MessageRepo := v2repo.NewMessageRepository(db)
		v2ScrapHandler := v2handler.NewScrapHandler(v2ScrapRepo)
		v2MemoHandler := v2handler.NewMemoHandler(v2MemoRepo)
		v2BlockHandler := v2handler.NewBlockHandler(v2BlockRepo)
		v2MessageHandler := v2handler.NewMessageHandler(v2MessageRepo)
		v2routes.SetupScrap(router, v2ScrapHandler, jwtManager)
		v2routes.SetupMemo(router, v2MemoHandler, jwtManager)
		v2routes.SetupBlock(router, v2BlockHandler, jwtManager)
		v2routes.SetupMessage(router, v2MessageHandler, jwtManager)

		// Installation API (인증 없이 접근 가능)
		v2InstallHandler := v2handler.NewInstallHandler(db)
		v2routes.SetupInstall(router, v2InstallHandler)

		// ============================================
		// v2 → v1 통합: v2 전용 기능을 v1에서도 제공
		// API 버전 단일화를 위한 전환 작업 (2026-02)
		// ============================================

		// v1에 auth/exchange 라우트 추가 (damoang_jwt → angple JWT 교환)
		v1Auth := router.Group("/api/v1/auth")
		v1Auth.POST("/exchange", v2AuthHandler.ExchangeGnuboardJWT)

		// v1에 install 라우트 추가 (설치 마법사)
		v1Install := router.Group("/api/v1/install")
		v1Install.GET("/status", v2InstallHandler.CheckInstallStatus)
		v1Install.POST("/test-db", v2InstallHandler.TestDB)
		v1Install.POST("/create-admin", v2InstallHandler.CreateAdmin)

		// Tenant Management (멀티테넌트 관리)
		adminTenants := router.Group("/api/v2/admin/tenants")
		adminTenants.GET("", tenantHandler.ListTenants)
		adminTenants.GET("/plans", middleware.CacheWithTTL(redisClient, 10*time.Minute), tenantHandler.GetPlanLimits)
		adminTenants.GET("/:id", tenantHandler.GetTenant)
		adminTenants.POST("/:id/suspend", tenantHandler.SuspendTenant)
		adminTenants.POST("/:id/unsuspend", tenantHandler.UnsuspendTenant)
		adminTenants.PUT("/:id/plan", tenantHandler.ChangePlan)
		adminTenants.GET("/:id/usage", tenantHandler.GetUsage)

		// SaaS Provisioning API
		saas := router.Group("/api/v2/saas")
		saas.GET("/pricing", middleware.CacheWithTTL(redisClient, 10*time.Minute), provisioningHandler.GetPricing)
		saas.POST("/communities", provisioningHandler.ProvisionCommunity)
		saas.DELETE("/communities/:id", provisioningHandler.DeleteCommunity)
		saas.GET("/communities/:id/subscription", provisioningHandler.GetSubscription)
		saas.PUT("/communities/:id/subscription/plan", provisioningHandler.ChangePlan)
		saas.POST("/communities/:id/subscription/cancel", provisioningHandler.CancelSubscription)
		saas.GET("/communities/:id/invoices", provisioningHandler.GetInvoices)

		// AI Recommendation API
		rec := router.Group("/api/v2/recommendations")
		rec.GET("/feed", recommendationHandler.GetPersonalizedFeed)
		rec.POST("/track", recommendationHandler.TrackActivity)
		rec.GET("/trending", middleware.CacheWithTTL(redisClient, 5*time.Minute), recommendationHandler.GetTrendingTopics)
		rec.GET("/interests", recommendationHandler.GetUserInterests)

		adminRec := router.Group("/api/v2/admin/recommendations")
		adminRec.POST("/extract", recommendationHandler.ExtractTopics)
		adminRec.POST("/refresh-trending", recommendationHandler.RefreshTrending)

		// OAuth2 Social Login API
		if oauthHandler != nil {
			oauth := router.Group("/api/v2/auth/oauth")
			oauth.GET("/:provider", oauthHandler.Redirect)
			oauth.GET("/:provider/callback", oauthHandler.Callback)

			// API Key management (authenticated)
			apiKeys := router.Group("/api/v2/auth/api-keys", middleware.JWTAuth(jwtManager))
			apiKeys.POST("", oauthHandler.GenerateAPIKey)
		}

		// CSRF token endpoint (for v1 cookie-based auth)
		router.GET("/api/v1/tokens/csrf", middleware.GenerateCSRFToken())

		// Audit Log API (관리자)
		if auditLogger != nil {
			router.GET("/api/v2/admin/audit-logs", func(c *gin.Context) {
				userID := c.Query("user_id")
				action := c.Query("action")
				page := 1
				perPage := 50
				if v, err := fmt.Sscanf(c.DefaultQuery("page", "1"), "%d", &page); v == 0 || err != nil {
					page = 1
				}
				logs, total, err := auditLogger.ListAuditLogs(c.Request.Context(), userID, action, page, perPage)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"success": true, "data": logs, "meta": gin.H{"total": total, "page": page}})
			})
		}

		// Payment API (Toss + Stripe)
		if paymentHandler != nil {
			payments := router.Group("/api/v2/payments")
			payments.POST("/toss", middleware.JWTAuth(jwtManager), paymentHandler.CreateTossPayment)
			payments.POST("/toss/confirm", paymentHandler.ConfirmTossPayment)
			payments.POST("/toss/webhook", paymentHandler.TossWebhook)
			payments.POST("/stripe/checkout", middleware.JWTAuth(jwtManager), paymentHandler.CreateStripeCheckout)
			payments.POST("/stripe/webhook", paymentHandler.StripeWebhook)
			payments.POST("/refund", middleware.JWTAuth(jwtManager), paymentHandler.RefundPayment)
			payments.GET("", middleware.JWTAuth(jwtManager), paymentHandler.ListPayments)
			payments.GET("/:order_id", middleware.JWTAuth(jwtManager), paymentHandler.GetPayment)
		}

		// Media Pipeline API (S3 storage)
		if mediaHandler != nil {
			media := router.Group("/api/v2/media", middleware.JWTAuth(jwtManager))
			media.POST("/images", mediaHandler.UploadImage)
			media.POST("/attachments", mediaHandler.UploadAttachment)
			media.POST("/videos", mediaHandler.UploadVideo)
			media.DELETE("/files", mediaHandler.DeleteFile)
		}

		// Elasticsearch Search API
		if searchHandler != nil {
			search := router.Group("/api/v2/search")
			search.GET("", searchHandler.Search)
			search.GET("/autocomplete", searchHandler.Autocomplete)

			adminSearch := router.Group("/api/v2/admin/search")
			adminSearch.POST("/index", searchHandler.BulkIndex)
			adminSearch.POST("/index-post", searchHandler.IndexPost)
			adminSearch.DELETE("/index/:board_id/:post_id", searchHandler.DeletePostIndex)
		}
	} else {
		pkglogger.Info("⚠️  Skipping API route setup (no DB connection)")
	}

	// Plugin System 초기화 (DB 연결 필요)
	if db != nil {
		// Plugin Store 초기화 (DB 기반 플러그인 상태 관리) - Manager보다 먼저 생성
		installRepo := pluginstoreRepo.NewInstallationRepository(db)
		settingRepo := pluginstoreRepo.NewSettingRepository(db)
		eventRepo := pluginstoreRepo.NewEventRepository(db)

		catalogSvc := pluginstoreSvc.NewCatalogService(installRepo)
		// Factory에 등록된 모든 플러그인 매니페스트 자동 등록
		factories := plugin.GetRegisteredFactories()
		log.Printf("[DEBUG] Registered factories count: %d", len(factories))
		for name, reg := range factories {
			log.Printf("[DEBUG] Registering manifest: %s", name)
			catalogSvc.RegisterManifest(reg.Manifest)
		}

		permRepo := pluginstoreRepo.NewPermissionRepository(db)

		storeSvc := pluginstoreSvc.NewStoreService(installRepo, eventRepo, settingRepo, catalogSvc, pluginLogger)
		settingSvc := pluginstoreSvc.NewSettingService(settingRepo, eventRepo, catalogSvc)
		permSvc := pluginstoreSvc.NewPermissionService(permRepo, catalogSvc)

		// Plugin Manager 생성 (settingSvc, permSvc 전달)
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

		// 설정 변경 시 플러그인 자동 리로드 연결
		settingSvc.SetReloader(pluginManager)

		// 내장 플러그인 자동 등록 (import한 패키지의 init()에서 Factory 등록됨)
		pluginManager.SetJWTManager(jwtManager)
		if err := pluginManager.RegisterAllFactories(); err != nil {
			pkglogger.Info("Failed to register plugin factories: %v", err)
		}

		// DB에서 enabled 플러그인 자동 활성화
		if err := storeSvc.BootEnabledPlugins(pluginManager); err != nil {
			pkglogger.Info("Failed to boot enabled plugins: %v", err)
		}

		// Plugin Store Admin API 핸들러
		storeHandler := pluginstoreHandler.NewStoreHandler(storeSvc, catalogSvc, pluginManager)
		settingHandler := pluginstoreHandler.NewSettingHandler(settingSvc, pluginManager)
		permHandler := pluginstoreHandler.NewPermissionHandler(permSvc)

		// Admin Plugin Store 라우트 등록 (v2 API - Bearer 토큰 인증)
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

		// Marketplace (플러그인 마켓플레이스)
		marketplaceRepo := pluginstoreRepo.NewMarketplaceRepository(db)
		if err := marketplaceRepo.AutoMigrate(); err != nil {
			pkglogger.Info("⚠️  Marketplace migration warning: %v", err)
		}
		marketplaceSvc := pluginstoreSvc.NewMarketplaceService(marketplaceRepo)
		marketplaceHandler := pluginstoreHandler.NewMarketplaceHandler(marketplaceSvc)

		// 마켓플레이스 Public API (v2)
		mp := router.Group("/api/v2/marketplace")
		mp.GET("", marketplaceHandler.Browse)
		mp.GET("/:name", marketplaceHandler.GetPlugin)
		mp.GET("/:name/reviews", marketplaceHandler.GetReviews)
		mp.POST("/:name/reviews", marketplaceHandler.AddReview)
		mp.POST("/:name/download", marketplaceHandler.TrackDownload)

		// 마켓플레이스 Developer API (v2, 인증 필요)
		mpDev := router.Group("/api/v2/marketplace/developers")
		mpDev.POST("/register", marketplaceHandler.RegisterDeveloper)
		mpDev.GET("/me", marketplaceHandler.GetMyProfile)
		mpDev.POST("/submissions", marketplaceHandler.SubmitPlugin)
		mpDev.GET("/submissions", marketplaceHandler.ListMySubmissions)

		// 마켓플레이스 Admin API
		mpAdmin := router.Group("/api/v2/admin/marketplace")
		mpAdmin.GET("/submissions/pending", marketplaceHandler.ListPendingSubmissions)
		mpAdmin.POST("/submissions/:id/review", marketplaceHandler.ReviewSubmission)

		// 플러그인 스케줄러 시작
		pluginManager.StartScheduler()

		pkglogger.Info("Plugin Store & Marketplace initialized")
	}

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
	dsn := cfg.Database.GetDSN()

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		// PrepareStmt: true, // Prepared statement 캐싱
		Logger: gormlogger.Default.LogMode(gormlogger.Info), // SQL 디버깅
	})
	if err != nil {
		return nil, err
	}

	// SQL 모드 비활성화 (STRICT_TRANS_TABLES 제거)
	db.Exec("SET SESSION sql_mode = ''")

	// UTF-8 인코딩 설정 (한글 깨짐 방지)
	db.Exec("SET NAMES utf8mb4")
	db.Exec("SET CHARACTER SET utf8mb4")
	db.Exec("SET character_set_connection=utf8mb4")

	// Connection Pool 설정
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetime) * time.Second)

	return db, nil
}
