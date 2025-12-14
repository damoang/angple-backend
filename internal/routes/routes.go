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

	// Current user endpoint (uses damoang_jwt cookie)
	auth.Get("/me", authHandler.GetCurrentUser)

	// Profile endpoint (auth required)
	auth.Get("/profile", middleware.JWTAuth(jwtManager), authHandler.GetProfile)

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
	recommended.Get("/:period", recommendedHandler.GetRecommended)

	// Menus (공개 API - 인증 불필요)
	menus := api.Group("/menus")
	menus.Get("", menuHandler.GetMenus)
	menus.Get("/sidebar", menuHandler.GetSidebarMenus)
	menus.Get("/header", menuHandler.GetHeaderMenus)
}
