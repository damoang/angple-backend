package routes

import (
	"github.com/damoang/angple-backend/internal/config"
	"github.com/damoang/angple-backend/internal/handler"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// Setup configures all API routes
func Setup(
	router *gin.Engine,
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
	api := router.Group("/api/v2", middleware.DamoangCookieAuth(damoangJWT, cfg))

	// Authentication endpoints (no auth required)
	auth := api.Group("/auth")
	auth.POST("/login", authHandler.Login)
	auth.POST("/refresh", authHandler.RefreshToken)
	auth.POST("/logout", authHandler.Logout)

	// Current user endpoint (uses damoang_jwt cookie)
	auth.GET("/me", authHandler.GetCurrentUser)

	// Profile endpoint (auth required)
	auth.GET("/profile", middleware.JWTAuth(jwtManager), authHandler.GetProfile)

	// Board Management (게시판 관리)
	boardsManagement := api.Group("/boards")
	boardsManagement.GET("", boardHandler.ListBoards)                                               // 게시판 목록 (공개)
	boardsManagement.GET("/:board_id", boardHandler.GetBoard)                                       // 게시판 정보 (공개)
	boardsManagement.POST("", middleware.JWTAuth(jwtManager), boardHandler.CreateBoard)             // 게시판 생성 (관리자)
	boardsManagement.PUT("/:board_id", middleware.JWTAuth(jwtManager), boardHandler.UpdateBoard)    // 게시판 수정 (관리자)
	boardsManagement.DELETE("/:board_id", middleware.JWTAuth(jwtManager), boardHandler.DeleteBoard) // 게시판 삭제 (관리자)

	// Group별 게시판
	groups := api.Group("/groups")
	groups.GET("/:group_id/boards", boardHandler.ListBoardsByGroup)

	// Board Posts (중첩 그룹 사용으로 Gin 라우팅 충돌 해결)
	boards := api.Group("/boards")

	// 게시판별 게시글 그룹
	boardPosts := boards.Group("/:board_id/posts")
	{
		// 게시글 목록 및 검색
		boardPosts.GET("", postHandler.ListPosts)
		boardPosts.GET("/search", postHandler.SearchPosts)
		boardPosts.POST("", middleware.JWTAuth(jwtManager), postHandler.CreatePost)

		// 게시글 상세/수정/삭제
		boardPosts.GET("/:id", postHandler.GetPost)
		boardPosts.PUT("/:id", middleware.JWTAuth(jwtManager), postHandler.UpdatePost)
		boardPosts.DELETE("/:id", middleware.JWTAuth(jwtManager), postHandler.DeletePost)

		// 댓글 관련 (파라미터 이름 통일: post_id -> id)
		comments := boardPosts.Group("/:id/comments")
		{
			comments.GET("", commentHandler.ListComments)
			comments.GET("/:comment_id", commentHandler.GetComment)
			comments.POST("", middleware.JWTAuth(jwtManager), commentHandler.CreateComment)
			comments.PUT("/:comment_id", middleware.JWTAuth(jwtManager), commentHandler.UpdateComment)
			comments.DELETE("/:comment_id", middleware.JWTAuth(jwtManager), commentHandler.DeleteComment)
		}
	}

	// Recommended Posts (공개 API - 인증 불필요)
	recommended := api.Group("/recommended")
	recommended.GET("/ai/:period", recommendedHandler.GetRecommendedAI) // AI 분석 기반 추천
	recommended.GET("/:period", recommendedHandler.GetRecommended)      // 일반 추천

	// Menus (공개 API - 인증 불필요)
	menus := api.Group("/menus")
	menus.GET("", menuHandler.GetMenus)
	menus.GET("/sidebar", menuHandler.GetSidebarMenus)
	menus.GET("/header", menuHandler.GetHeaderMenus)

	// Sites (Multi-tenant SaaS)
	sites := api.Group("/sites")

	// Public endpoints (인증 불필요)
	sites.GET("/subdomain/:subdomain", siteHandler.GetBySubdomain)                   // angple-saas Admin hooks에서 호출
	sites.GET("/check-subdomain/:subdomain", siteHandler.CheckSubdomainAvailability) // 회원가입 플로우에서 중복 체크
	sites.GET("/:id", siteHandler.GetByID)
	sites.GET("", siteHandler.ListActive) // Admin 대시보드용

	// Settings endpoints
	sites.GET("/:id/settings", siteHandler.GetSettings)
	sites.PUT("/:id/settings", siteHandler.UpdateSettings) // TODO: 인증 추가 필요

	// Provisioning endpoint (결제 후 사이트 생성)
	sites.POST("", siteHandler.Create) // TODO: 인증 추가 필요 (Admin only)
}
