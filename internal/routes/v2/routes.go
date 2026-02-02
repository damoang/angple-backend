package v2

import (
	v2handler "github.com/damoang/angple-backend/internal/handler/v2"
	"github.com/gin-gonic/gin"
)

// Setup configures v2 API routes (new DB schema)
func Setup(router *gin.Engine, h *v2handler.V2Handler) {
	api := router.Group("/api/v2-next")

	// Users
	users := api.Group("/users")
	users.GET("", h.ListUsers)
	users.GET("/:id", h.GetUser)
	users.GET("/username/:username", h.GetUserByUsername)

	// Boards
	boards := api.Group("/boards")
	boards.GET("", h.ListBoards)
	boards.GET("/:slug", h.GetBoard)

	// Posts (nested under boards)
	boardPosts := boards.Group("/:slug/posts")
	boardPosts.GET("", h.ListPosts)
	boardPosts.POST("", h.CreatePost)
	boardPosts.GET("/:id", h.GetPost)
	boardPosts.PUT("/:id", h.UpdatePost)
	boardPosts.DELETE("/:id", h.DeletePost)

	// Comments (nested under posts)
	comments := boardPosts.Group("/:id/comments")
	comments.GET("", h.ListComments)
	comments.POST("", h.CreateComment)
	comments.DELETE("/:comment_id", h.DeleteComment)
}
