package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/damoang/angple-backend/docs" // swagger docs
	"github.com/damoang/angple-backend/internal/config"
	"github.com/damoang/angple-backend/internal/handler"
	"github.com/damoang/angple-backend/internal/repository"
	"github.com/damoang/angple-backend/internal/routes"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/damoang/angple-backend/pkg/jwt"
	pkglogger "github.com/damoang/angple-backend/pkg/logger"
	pkgredis "github.com/damoang/angple-backend/pkg/redis"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
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
		pkglogger.Info("⚠️  Warning: Failed to connect to database: %v (continuing without DB)", err)
		pkglogger.Info("⚠️  Health check will work, but API endpoints will fail")
		db = nil
	} else {
		pkglogger.Info("✅ Connected to MySQL")
	}

	// Redis 연결 (Phase 3에서 캐싱에 사용 예정)
	_, err = pkgredis.NewClient(
		cfg.Redis.Host,
		cfg.Redis.Port,
		cfg.Redis.Password,
		cfg.Redis.DB,
		cfg.Redis.PoolSize,
	)
	if err != nil {
		pkglogger.Info("⚠️  Warning: Failed to connect to Redis: %v (continuing without Redis)", err)
	} else {
		pkglogger.Info("✅ Connected to Redis")
	}

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

	// DI Container (skip if no DB connection)
	var authHandler *handler.AuthHandler
	var postHandler *handler.PostHandler
	var commentHandler *handler.CommentHandler
	var menuHandler *handler.MenuHandler
	var siteHandler *handler.SiteHandler
	var boardHandler *handler.BoardHandler

	if db != nil {
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
		authHandler = handler.NewAuthHandler(authService, cfg)
		postHandler = handler.NewPostHandler(postService)
		commentHandler = handler.NewCommentHandler(commentService)
		menuHandler = handler.NewMenuHandler(menuService)
		siteHandler = handler.NewSiteHandler(siteService)
		boardHandler = handler.NewBoardHandler(boardService)
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
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowCredentials: true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	}
	// CORS가 단일 origin 문자열인 경우 처리
	if len(corsConfig.AllowOrigins) == 1 && corsConfig.AllowOrigins[0] != "" {
		// 쉼표로 구분된 여러 origin 처리
		corsConfig.AllowOrigins = splitAndTrim(allowOrigins, ",")
	}
	router.Use(cors.New(corsConfig))

	// Health Check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"time":   time.Now().Unix(),
		})
	})

	// Swagger UI
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// API v2 라우트 (only if DB is connected)
	if db != nil {
		routes.Setup(router, postHandler, commentHandler, authHandler, menuHandler, siteHandler, boardHandler, jwtManager, damoangJWT, recommendedHandler, cfg)
	} else {
		pkglogger.Info("⚠️  Skipping API route setup (no DB connection)")
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
