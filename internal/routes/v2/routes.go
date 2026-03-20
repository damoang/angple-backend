package v2

import (
	"github.com/damoang/angple-backend/internal/handler"
	v2handler "github.com/damoang/angple-backend/internal/handler/v2"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SetupAuth configures v2 authentication routes
func SetupAuth(router *gin.Engine, h *v2handler.V2AuthHandler, jwtManager *jwt.Manager, loginRateLimit ...gin.HandlerFunc) {
	authGroup := router.Group("/api/v2/auth")
	loginHandlers := append(loginRateLimit, h.Login)
	authGroup.POST("/login", loginHandlers...)
	authGroup.POST("/refresh", h.RefreshToken)
	authGroup.POST("/logout", h.Logout)
	authGroup.GET("/me", middleware.JWTAuth(jwtManager), h.GetMe)
	authGroup.GET("/profile", middleware.JWTAuth(jwtManager), h.GetMe) // alias for /me
	// TODO: v2 마이그레이션 - exchange는 레거시 SSO용, 향후 세션 기반으로 전환
	authGroup.POST("/exchange", h.ExchangeToken)

	// v1 미러
	v1Auth := router.Group("/api/v1/auth")
	v1Auth.POST("/exchange", h.ExchangeToken)
}

// Setup configures v2 API routes (new DB schema)
func Setup(router *gin.Engine, h *v2handler.V2Handler, jwtManager *jwt.Manager, boardPermChecker middleware.BoardPermissionChecker, gnuDB *gorm.DB) {
	api := router.Group("/api/v2")
	auth := middleware.JWTAuth(jwtManager)
	banCheck := middleware.BanCheck(gnuDB)

	// Users
	users := api.Group("/users")
	users.GET("", h.ListUsers)
	users.GET("/:id", h.GetUser)
	users.GET("/username/:username", h.GetUserByUsername)

	// Boards (OptionalJWTAuth로 인증된 사용자에게 permissions 제공)
	boards := api.Group("/boards")
	boards.Use(middleware.ValidateBoardSlug())
	boards.Use(middleware.OptionalJWTAuth(jwtManager))
	boards.Use(middleware.ArchiveBoardCheck())
	boards.GET("", h.ListBoards)
	boards.GET("/:slug", h.GetBoard)

	// Posts (nested under boards)
	boardPosts := boards.Group("/:slug/posts")
	boardPosts.GET("", h.ListPosts)
	boardPosts.POST("", auth, banCheck, middleware.RequireWrite(boardPermChecker), h.CreatePost)
	boardPosts.GET("/:id", h.GetPost)
	boardPosts.PUT("/:id", auth, banCheck, h.UpdatePost)
	boardPosts.DELETE("/:id", auth, banCheck, h.DeletePost)
	boardPosts.PATCH("/:id/soft-delete", auth, banCheck, h.SoftDeletePost)
	boardPosts.POST("/:id/restore", auth, middleware.RequireAdmin(), h.RestorePost)
	boardPosts.DELETE("/:id/permanent", auth, middleware.RequireAdmin(), h.PermanentDeletePost)
	boardPosts.GET("/:id/revisions", auth, h.GetPostRevisions)
	boardPosts.POST("/:id/revisions/:version/restore", auth, middleware.RequireAdmin(), h.RestoreRevision)

	// Comments (nested under posts)
	comments := boardPosts.Group("/:id/comments")
	comments.GET("", h.ListComments)
	comments.POST("", auth, banCheck, middleware.RequireComment(boardPermChecker), h.CreateComment)
	comments.PUT("/:comment_id", auth, banCheck, h.UpdateComment)
	comments.DELETE("/:comment_id", auth, banCheck, h.DeleteComment)
}

// SetupAdminPosts configures v2 admin post routes (deleted posts)
func SetupAdminPosts(router *gin.Engine, h *v2handler.V2Handler, jwtManager *jwt.Manager) {
	admin := router.Group("/api/v1/admin")
	admin.Use(middleware.JWTAuth(jwtManager), middleware.RequireAdmin())
	admin.GET("/posts/deleted", h.GetDeletedPosts)
	admin.POST("/posts/:id/restore", h.RestorePost)
	admin.DELETE("/posts/:id/permanent", h.PermanentDeletePost)
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

// SetupAdminXP configures admin XP management routes
func SetupAdminXP(router *gin.Engine, h *v2handler.ExpHandler, jwtManager *jwt.Manager) {
	admin := router.Group("/api/v2/admin/xp")
	admin.Use(middleware.JWTAuth(jwtManager), middleware.RequireAdmin())

	// XP config (login XP amount, etc.)
	admin.GET("/config", h.AdminGetXPConfig)
	admin.PUT("/config", h.AdminUpdateXPConfig)

	// Member XP management
	adminMembers := admin.Group("/members")
	adminMembers.GET("", h.AdminListMemberXP)
	adminMembers.GET("/:mbId/history", h.AdminGetMemberXPHistory)
	adminMembers.POST("/:mbId/grant", h.AdminGrantXP)
}

// SetupAdminPoint configures admin point configuration routes
func SetupAdminPoint(router *gin.Engine, h *v2handler.ExpHandler, jwtManager *jwt.Manager) {
	admin := router.Group("/api/v2/admin/point")
	admin.Use(middleware.JWTAuth(jwtManager), middleware.RequireAdmin())

	admin.GET("/config", h.AdminGetPointConfig)
	admin.PUT("/config", h.AdminUpdatePointConfig)
}

// SetupAdminSettings configures admin settings routes
func SetupAdminSettings(router *gin.Engine, h *v2handler.AdminSettingsHandler, jwtManager *jwt.Manager) {
	admin := router.Group("/api/v1/admin/settings")
	admin.Use(middleware.JWTAuth(jwtManager), middleware.RequireAdmin())

	admin.GET("/report-lock", h.GetReportLockThreshold)
	admin.PUT("/report-lock", h.UpdateReportLockThreshold)
}

// SetupScrap configures v2 scrap routes
func SetupScrap(router *gin.Engine, h *v2handler.ScrapHandler, jwtManager *jwt.Manager, gnuDB *gorm.DB) {
	auth := middleware.JWTAuth(jwtManager)
	banCheck := middleware.BanCheck(gnuDB)

	posts := router.Group("/api/v2/posts")
	posts.POST("/:id/scrap", auth, banCheck, h.AddScrap)
	posts.DELETE("/:id/scrap", auth, h.RemoveScrap)

	me := router.Group("/api/v2/me", auth)
	me.GET("/scraps", h.ListScraps)
}

// SetupMemo configures v2 memo routes
func SetupMemo(router *gin.Engine, h *v2handler.MemoHandler, jwtManager *jwt.Manager, gnuDB *gorm.DB) {
	auth := middleware.JWTAuth(jwtManager)
	banCheck := middleware.BanCheck(gnuDB)

	memo := router.Group("/api/v2/members/:id/memo", auth)
	memo.GET("", h.GetMemo)
	memo.POST("", banCheck, h.CreateMemo)
	memo.PUT("", h.UpdateMemo)
	memo.DELETE("", h.DeleteMemo)
	memo.GET("/all", middleware.RequireAdmin(), h.GetAllMemos)
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
func SetupMessage(router *gin.Engine, h *v2handler.MessageHandler, jwtManager *jwt.Manager, gnuDB ...*gorm.DB) {
	auth := middleware.JWTAuth(jwtManager)

	messages := router.Group("/api/v2/messages", auth)
	if len(gnuDB) > 0 && gnuDB[0] != nil {
		messages.POST("", middleware.BanCheck(gnuDB[0]), h.SendMessage)
	} else {
		messages.POST("", h.SendMessage)
	}
	messages.GET("/inbox", h.GetInbox)
	messages.GET("/sent", h.GetSent)
	messages.GET("/:id", h.GetMessage)
	messages.DELETE("/:id", h.DeleteMessage)
}

// SetupInstall configures v2 installation routes
func SetupInstall(router *gin.Engine, h *v2handler.InstallHandler) {
	install := router.Group("/api/v2/install")

	install.GET("/status", h.CheckInstallStatus)

	// 설치 미완료일 때만 접근 가능
	installOnly := install.Group("", h.RequireNotInstalled())
	installOnly.POST("/test-db", h.TestDB)
	installOnly.POST("/create-admin", h.CreateAdmin)
}

// SetupMyPage configures user's own data routes (points, exp, posts, comments, stats)
func SetupMyPage(router *gin.Engine, pointHandler *v2handler.PointHandler, expHandler *v2handler.ExpHandler, myPageHandler *handler.MyPageHandler, jwtManager *jwt.Manager) {
	auth := middleware.JWTAuth(jwtManager)

	my := router.Group("/api/v1/my", auth)

	// Point routes
	my.GET("/point", pointHandler.GetPointSummary)
	my.GET("/point/history", pointHandler.GetPointHistory)

	// Exp routes
	my.GET("/exp", expHandler.GetExpSummary)
	my.GET("/exp/history", expHandler.GetExpHistory)

	// MyPage routes (posts, comments, liked-posts, stats)
	my.GET("/posts", myPageHandler.GetMyPosts)
	my.GET("/comments", myPageHandler.GetMyComments)
	my.GET("/liked-posts", myPageHandler.GetMyLikedPosts)
	my.GET("/stats", myPageHandler.GetBoardStats)
}

// SetupMemberActivity configures public member activity route
func SetupMemberActivity(router *gin.Engine, myPageHandler *handler.MyPageHandler) {
	router.GET("/api/v1/members/:id/activity", myPageHandler.GetMemberActivity)
}

// SetupDisciplineLog configures discipline log routes (read-only, uses gnuboard g5_write_disciplinelog)
func SetupDisciplineLog(router *gin.Engine, h *v2handler.DisciplineLogHandler, _ *jwt.Manager) {
	// Public routes (read-only)
	disciplineLog := router.Group("/api/v1/discipline-logs")
	disciplineLog.GET("", h.GetList)
	disciplineLog.GET("/violation-types", h.GetViolationTypes)
	disciplineLog.GET("/:id", h.GetDetail)
	// Note: Admin CRUD is handled by PHP admin (gnuboard)
}

// SetupBanner configures banner routes (v1 + v2)
// TODO: v2 마이그레이션 - DB 재설계 후 v2 전용 배너 테이블로 전환
func SetupBanner(router *gin.Engine, h *v2handler.BannerHandler) {
	// v1 routes (primary)
	v1Banners := router.Group("/api/v1/banners")
	v1Banners.GET("", h.GetBanners)
	v1Banners.GET("/:id/click", h.TrackClick)

	// v2 routes (frontend 호환 - 기존 플러그인이 v2로 호출)
	v2Banners := router.Group("/api/v2/banners")
	v2Banners.GET("", h.GetBanners)
	v2Banners.GET("/:id/click", h.TrackClick)
}

// SetupPromotion configures promotion routes (v1 + v2)
// TODO: v2 마이그레이션 - DB 재설계 후 v2 전용 프로모션 테이블로 전환
func SetupPromotion(router *gin.Engine, h *v2handler.PromotionHandler) {
	// v1 routes (primary)
	v1Promo := router.Group("/api/v1/promotion")
	v1Promo.GET("/posts/insert", h.GetInsertPosts)

	// v2 routes (frontend 호환 - 기존 플러그인이 v2로 호출)
	v2Promo := router.Group("/api/v2/promotion")
	v2Promo.GET("/posts/insert", h.GetInsertPosts)
}

// SetupLicense configures license verification routes (v1 + v2)
// TODO: v2 마이그레이션 - 라이선스 테이블 설계 후 실제 검증 로직 구현
func SetupLicense(router *gin.Engine, h *v2handler.LicenseHandler) {
	// v1 routes (primary)
	router.POST("/api/v1/licenses/verify", h.Verify)

	// v2 routes (frontend 호환)
	router.POST("/api/v2/licenses/verify", h.Verify)
}

// SetupSiteLogo configures site logo routes (admin CRUD + public active logo)
func SetupSiteLogo(router *gin.Engine, h *v2handler.SiteLogoHandler, jwtManager *jwt.Manager) {
	admin := router.Group("/api/v1/admin/logos")
	admin.Use(middleware.JWTAuth(jwtManager), middleware.RequireAdmin())
	admin.GET("", h.ListLogos)
	admin.POST("", h.CreateLogo)
	admin.PUT("/:id", h.UpdateLogo)
	admin.DELETE("/:id", h.DeleteLogo)

	// public: 오늘의 로고 (SSR용)
	router.GET("/api/v1/logos/active", h.GetActiveLogo)
}

// SetupFavorite configures board favorites routes
func SetupFavorite(router *gin.Engine, h *v2handler.FavoriteHandler, jwtManager *jwt.Manager) {
	auth := middleware.JWTAuth(jwtManager)

	// /api/v1/my/* 패턴 — 기존 my/point, my/posts 등과 일관성 유지
	my := router.Group("/api/v1/my", auth)
	my.GET("/favorites", h.GetFavorites)
	my.PUT("/favorites", h.UpdateFavorites)
}

// SetupContent configures content page routes (admin + public)
func SetupContent(router *gin.Engine, h *v2handler.ContentHandler, jwtManager *jwt.Manager) {
	// Admin routes (requires authentication + admin)
	admin := router.Group("/api/v2/admin/contents")
	admin.Use(middleware.JWTAuth(jwtManager), middleware.RequireAdmin())
	admin.GET("", h.ListContents)
	admin.GET("/:id", h.GetContent)
	admin.PUT("/:id", h.UpdateContent)

	// Public route (no auth required)
	router.GET("/api/v2/contents/:id", h.GetPublicContent)
}
