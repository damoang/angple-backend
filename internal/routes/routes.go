package routes

import (
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
	jwtManager *jwt.Manager,
) {
	api := app.Group("/api/v2")

	// Authentication endpoints (no auth required)
	auth := api.Group("/auth")
	auth.Post("/login", authHandler.Login)
	auth.Post("/refresh", authHandler.RefreshToken)

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
}
