package main

import (
	"fmt"
	"log"
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
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	// .env 파일 로드 (없어도 에러 무시)
	_ = godotenv.Load()

	// 로거 초기화
	pkglogger.Init()
	pkglogger.Info("Starting Angple API Server...")

	// 설정 로드
	cfg, err := config.Load("configs/config.dev.yaml")
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

	// Services
	authService := service.NewAuthService(memberRepo, jwtManager)
	postService := service.NewPostService(postRepo)
	commentService := service.NewCommentService(commentRepo)
	menuService := service.NewMenuService(menuRepo)

	// Handlers
	authHandler := handler.NewAuthHandler(authService)
	postHandler := handler.NewPostHandler(postService)
	commentHandler := handler.NewCommentHandler(commentService)
	menuHandler := handler.NewMenuHandler(menuService)

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

	// API v2 라우트
	routes.Setup(app, postHandler, commentHandler, authHandler, menuHandler, jwtManager, damoangJWT, recommendedHandler, cfg)

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
