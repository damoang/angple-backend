package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/damoang/angple-backend/internal/config"
	"github.com/damoang/angple-backend/internal/handler"
	"github.com/damoang/angple-backend/internal/repository"
	"github.com/damoang/angple-backend/internal/routes"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/damoang/angple-backend/pkg/jwt"
	pkglogger "github.com/damoang/angple-backend/pkg/logger"
	pkgredis "github.com/damoang/angple-backend/pkg/redis"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	fiberSwagger "github.com/swaggo/fiber-swagger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	_ "github.com/damoang/angple-backend/docs" // swagger docs
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
// @BasePath        /api/v2
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
		log.Fatalf("Failed to connect to database: %v", err)
	}
	pkglogger.Info("Connected to MySQL")

	// Redis 연결 (Phase 3에서 캐싱에 사용 예정)
	_, err = pkgredis.NewClient(
		cfg.Redis.Host,
		cfg.Redis.Port,
		cfg.Redis.Password,
		cfg.Redis.DB,
		cfg.Redis.PoolSize,
	)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	pkglogger.Info("Connected to Redis")

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

	// Repositories
	memberRepo := repository.NewMemberRepository(db)
	postRepo := repository.NewPostRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	menuRepo := repository.NewMenuRepository(db)
	siteRepo := repository.NewSiteRepository(db)
	boardRepo := repository.NewBoardRepository(db)

	// Services
	authService := service.NewAuthService(memberRepo, jwtManager)
	postService := service.NewPostService(postRepo)
	commentService := service.NewCommentService(commentRepo)
	menuService := service.NewMenuService(menuRepo)
	siteService := service.NewSiteService(siteRepo)
	boardService := service.NewBoardService(boardRepo)

	// Handlers
	authHandler := handler.NewAuthHandler(authService, cfg)
	postHandler := handler.NewPostHandler(postService)
	commentHandler := handler.NewCommentHandler(commentService)
	menuHandler := handler.NewMenuHandler(menuService)
	siteHandler := handler.NewSiteHandler(siteService)
	boardHandler := handler.NewBoardHandler(boardService)

	// Recommended Handler (파일 직접 읽기)
	recommendedPath := cfg.DataPaths.RecommendedPath
	if recommendedPath == "" {
		recommendedPath = "/home/damoang/www/data/cache/recommended"
	}
	recommendedHandler := handler.NewRecommendedHandler(recommendedPath)

	// Fiber 앱 생성
	app := fiber.New(fiber.Config{
		Prefork:       false, // 개발 환경에서는 false
		CaseSensitive: true,
		StrictRouting: false,
		ServerHeader:  "Angple API",
		AppName:       "Angple Backend v1.0.0",
	})

	// 미들웨어
	app.Use(recover.New())
	app.Use(logger.New())

	// CORS 설정 (config에서 읽어오거나 운영 기본값 사용)
	allowOrigins := cfg.CORS.AllowOrigins
	if allowOrigins == "" {
		// 운영 환경 기본값: 운영 도메인만 허용
		allowOrigins = "https://web.damoang.net, https://damoang.net"
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins:     allowOrigins,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowCredentials: true,
	}))

	// Health Check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
			"time":   time.Now().Unix(),
		})
	})

	// Swagger UI
	app.Get("/swagger/*", fiberSwagger.WrapHandler)

	// API v2 라우트
	routes.Setup(app, postHandler, commentHandler, authHandler, menuHandler, siteHandler, boardHandler, jwtManager, damoangJWT, recommendedHandler, cfg)

	// 서버 시작
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	pkglogger.Info("Server listening on %s", addr)
	if err := app.Listen(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
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
