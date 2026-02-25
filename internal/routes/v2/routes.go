package v2

import (
	v2handler "github.com/damoang/angple-backend/internal/handler/v2"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// SetupAuth configures v2 authentication routes
func SetupAuth(router *gin.Engine, h *v2handler.V2AuthHandler, jwtManager *jwt.Manager) {
	authGroup := router.Group("/api/v2/auth")
	authGroup.POST("/login", h.Login)
	authGroup.POST("/refresh", h.RefreshToken)
	authGroup.POST("/logout", h.Logout)
	authGroup.GET("/me", middleware.JWTAuth(jwtManager), h.GetMe)
	authGroup.GET("/profile", middleware.JWTAuth(jwtManager), h.GetMe) // alias for /me
}

// Setup configures v2 API routes (new DB schema)
func Setup(router *gin.Engine, h *v2handler.V2Handler, jwtManager *jwt.Manager, boardPermChecker middleware.BoardPermissionChecker, ipProtectCfg *middleware.IPProtectionConfig) {
	api := router.Group("/api/v2")
	auth := middleware.JWTAuth(jwtManager)
	ipProtect := middleware.IPProtection(ipProtectCfg)

	// Users
	users := api.Group("/users")
	users.GET("", h.ListUsers)
	users.GET("/:id", h.GetUser)
	users.GET("/username/:username", h.GetUserByUsername)

	// Boards (OptionalJWTAuth로 인증된 사용자에게 permissions 제공)
	boards := api.Group("/boards")
	boards.Use(middleware.OptionalJWTAuth(jwtManager))
	boards.GET("", h.ListBoards)
	boards.GET("/:slug", h.GetBoard)

	// Posts (nested under boards)
	boardPosts := boards.Group("/:slug/posts")
	boardPosts.GET("", h.ListPosts)
	boardPosts.POST("", auth, middleware.RequireWrite(boardPermChecker), ipProtect, h.CreatePost)
	boardPosts.GET("/:id", h.GetPost)
	boardPosts.PUT("/:id", auth, h.UpdatePost)
	boardPosts.DELETE("/:id", auth, h.DeletePost)

	// Comments (nested under posts)
	comments := boardPosts.Group("/:id/comments")
	comments.GET("", h.ListComments)
	comments.POST("", auth, middleware.RequireComment(boardPermChecker), ipProtect, h.CreateComment)
	comments.DELETE("/:comment_id", auth, h.DeleteComment)
}

// SetupAdmin configures v2 admin API routes
func SetupAdmin(router *gin.Engine, h *v2handler.AdminHandler, jwtManager *jwt.Manager) {
	admin := router.Group("/api/v2/admin")
	admin.Use(middleware.JWTAuth(jwtManager), middleware.RequireAdmin())

	// Admin Boards
	adminBoards := admin.Group("/boards")
	adminBoards.GET("", h.ListBoards)
	adminBoards.POST("", h.CreateBoard)
	adminBoards.PUT("/:id", h.UpdateBoard)
	adminBoards.DELETE("/:id", h.DeleteBoard)

	// Admin Members
	adminMembers := admin.Group("/members")
	adminMembers.GET("", h.ListMembers)
	adminMembers.GET("/:id", h.GetMember)
	adminMembers.PUT("/:id", h.UpdateMember)
	adminMembers.POST("/:id/ban", h.BanMember)

	// Admin Dashboard
	admin.GET("/dashboard/stats", h.GetDashboardStats)
}

// SetupScrap configures v2 scrap routes
func SetupScrap(router *gin.Engine, h *v2handler.ScrapHandler, jwtManager *jwt.Manager) {
	auth := middleware.JWTAuth(jwtManager)

	posts := router.Group("/api/v2/posts")
	posts.POST("/:id/scrap", auth, h.AddScrap)
	posts.DELETE("/:id/scrap", auth, h.RemoveScrap)

	me := router.Group("/api/v2/me", auth)
	me.GET("/scraps", h.ListScraps)
}

// SetupMemo configures v2 memo routes
func SetupMemo(router *gin.Engine, h *v2handler.MemoHandler, jwtManager *jwt.Manager) {
	auth := middleware.JWTAuth(jwtManager)

	memo := router.Group("/api/v2/members/:id/memo", auth)
	memo.GET("", h.GetMemo)
	memo.POST("", h.CreateMemo)
	memo.PUT("", h.UpdateMemo)
	memo.DELETE("", h.DeleteMemo)
}

// SetupBlock configures v2 block routes
func SetupBlock(router *gin.Engine, h *v2handler.BlockHandler, jwtManager *jwt.Manager) {
	auth := middleware.JWTAuth(jwtManager)

	// Block/Unblock member
	members := router.Group("/api/v2/members")
	members.POST("/:id/block", auth, h.BlockMember)
	members.DELETE("/:id/block", auth, h.UnblockMember)

	// List blocked members
	me := router.Group("/api/v2/members/me", auth)
	me.GET("/blocks", h.ListBlocks)
}

// SetupMessage configures v2 message routes
func SetupMessage(router *gin.Engine, h *v2handler.MessageHandler, jwtManager *jwt.Manager) {
	auth := middleware.JWTAuth(jwtManager)

	messages := router.Group("/api/v2/messages", auth)
	messages.POST("", h.SendMessage)
	messages.GET("/inbox", h.GetInbox)
	messages.GET("/sent", h.GetSent)
	messages.GET("/:id", h.GetMessage)
	messages.DELETE("/:id", h.DeleteMessage)
}

// SetupInstall configures v2 installation routes (no authentication required)
func SetupInstall(router *gin.Engine, h *v2handler.InstallHandler) {
	install := router.Group("/api/v2/install")

	install.GET("/status", h.CheckInstallStatus)
	install.POST("/test-db", h.TestDB)
	install.POST("/create-admin", h.CreateAdmin)
}
