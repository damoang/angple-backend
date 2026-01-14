package routes

import (
	"github.com/damoang/angple-backend/internal/config"
	"github.com/damoang/angple-backend/internal/handler"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gofiber/fiber/v2"
)

// Setup configures all API routes
func Setup(
	app *fiber.App,
	postHandler *handler.PostHandler,
	commentHandler *handler.CommentHandler,
	authHandler *handler.AuthHandler,
	menuHandler *handler.MenuHandler,
	siteHandler *handler.SiteHandler,
	boardHandler *handler.BoardHandler,
	jwtManager *jwt.Manager,
	damoangJWT *jwt.DamoangManager,
	recommendedHandler *handler.RecommendedHandler,
	cfg *config.Config,
) {
	// Global middleware for damoang_jwt cookie authentication
	api := app.Group("/api/v2", middleware.DamoangCookieAuth(damoangJWT, cfg))

	// Authentication endpoints (no auth required)
	auth := api.Group("/auth")
	auth.Post("/login", authHandler.Login)
	auth.Post("/refresh", authHandler.RefreshToken)
	auth.Post("/logout", authHandler.Logout)

	// Current user endpoint (uses damoang_jwt cookie)
	auth.Get("/me", authHandler.GetCurrentUser)

	// Profile endpoint (auth required)
	auth.Get("/profile", middleware.JWTAuth(jwtManager), authHandler.GetProfile)

	// Board Management (게시판 관리)
	boardsManagement := api.Group("/boards")
	boardsManagement.Get("", boardHandler.ListBoards)                                               // 게시판 목록 (공개)
	boardsManagement.Get("/:board_id", boardHandler.GetBoard)                                       // 게시판 정보 (공개)
	boardsManagement.Post("", middleware.JWTAuth(jwtManager), boardHandler.CreateBoard)             // 게시판 생성 (관리자)
	boardsManagement.Put("/:board_id", middleware.JWTAuth(jwtManager), boardHandler.UpdateBoard)    // 게시판 수정 (관리자)
	boardsManagement.Delete("/:board_id", middleware.JWTAuth(jwtManager), boardHandler.DeleteBoard) // 게시판 삭제 (관리자)

	// Group별 게시판
	groups := api.Group("/groups")
	groups.Get("/:group_id/boards", boardHandler.ListBoardsByGroup)

	// Board Posts
	boards := api.Group("/boards")
	boards.Get("/:board_id/posts", postHandler.ListPosts)
	boards.Get("/:board_id/posts/search", postHandler.SearchPosts)
	boards.Get("/:board_id/posts/:id", postHandler.GetPost)

	// Authentication required endpoints
	boards.Post("/:board_id/posts", middleware.JWTAuth(jwtManager), postHandler.CreatePost)
	boards.Put("/:board_id/posts/:id", middleware.JWTAuth(jwtManager), postHandler.UpdatePost)
	boards.Delete("/:board_id/posts/:id", middleware.JWTAuth(jwtManager), postHandler.DeletePost)

	// Comments
	boards.Get("/:board_id/posts/:post_id/comments", commentHandler.ListComments)
	boards.Get("/:board_id/posts/:post_id/comments/:id", commentHandler.GetComment)

	// Authentication required comment endpoints
	boards.Post("/:board_id/posts/:post_id/comments", middleware.JWTAuth(jwtManager), commentHandler.CreateComment)
	boards.Put("/:board_id/posts/:post_id/comments/:id", middleware.JWTAuth(jwtManager), commentHandler.UpdateComment)
	boards.Delete("/:board_id/posts/:post_id/comments/:id", middleware.JWTAuth(jwtManager), commentHandler.DeleteComment)

	// Recommended Posts (공개 API - 인증 불필요)
	recommended := api.Group("/recommended")
	recommended.Get("/ai/:period", recommendedHandler.GetRecommendedAI) // AI 분석 기반 추천
	recommended.Get("/:period", recommendedHandler.GetRecommended)      // 일반 추천

	// Menus (공개 API - 인증 불필요)
	menus := api.Group("/menus")
	menus.Get("", menuHandler.GetMenus)
	menus.Get("/sidebar", menuHandler.GetSidebarMenus)
	menus.Get("/header", menuHandler.GetHeaderMenus)

	// Sites (Multi-tenant SaaS)
	sites := api.Group("/sites")

	// Public endpoints (인증 불필요)
	sites.Get("/subdomain/:subdomain", siteHandler.GetBySubdomain)                   // angple-saas Admin hooks에서 호출
	sites.Get("/check-subdomain/:subdomain", siteHandler.CheckSubdomainAvailability) // 회원가입 플로우에서 중복 체크
	sites.Get("/:id", siteHandler.GetByID)
	sites.Get("", siteHandler.ListActive) // Admin 대시보드용

	// Settings endpoints
	sites.Get("/:id/settings", siteHandler.GetSettings)
	sites.Put("/:id/settings", siteHandler.UpdateSettings) // TODO: 인증 추가 필요

	// Provisioning endpoint (결제 후 사이트 생성)
	sites.Post("", siteHandler.Create) // TODO: 인증 추가 필요 (Admin only)
}
