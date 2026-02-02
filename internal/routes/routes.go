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
	memberHandler *handler.MemberHandler,
	autosaveHandler *handler.AutosaveHandler,
	filterHandler *handler.FilterHandler,
	tokenHandler *handler.TokenHandler,
	memoHandler *handler.MemoHandler,
	reactionHandler *handler.ReactionHandler,
	reportHandler *handler.ReportHandler,
	dajoongiHandler *handler.DajoongiHandler,
	promotionHandler *handler.PromotionHandler,
	bannerHandler *handler.BannerHandler,
	jwtManager *jwt.Manager,
	damoangJWT *jwt.DamoangManager,
	goodHandler *handler.GoodHandler,
	recommendedHandler *handler.RecommendedHandler,
	notificationHandler *handler.NotificationHandler,
	memberProfileHandler *handler.MemberProfileHandler,
	fileHandler *handler.FileHandler,
	scrapHandler *handler.ScrapHandler,
	blockHandler *handler.BlockHandler,
	messageHandler *handler.MessageHandler,
	wsHandler *handler.WSHandler,
	disciplineHandler *handler.DisciplineHandler,
	galleryHandler *handler.GalleryHandler,
	adminHandler *handler.AdminHandler,
	cfg *config.Config,
) {
	// Global middleware for damoang_jwt cookie authentication
	api := router.Group("/api/v2", middleware.DamoangCookieAuth(damoangJWT, cfg))

	// Authentication endpoints (no auth required)
	auth := api.Group("/auth")
	auth.POST("/login", authHandler.Login)
	auth.POST("/refresh", authHandler.RefreshToken)
	auth.POST("/logout", authHandler.Logout)
	auth.POST("/register", authHandler.Register) // 회원가입

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
		boardPosts.GET("/:id/preview", postHandler.GetPostPreview) // 게시글 미리보기
		boardPosts.PUT("/:id", middleware.JWTAuth(jwtManager), postHandler.UpdatePost)
		boardPosts.DELETE("/:id", middleware.JWTAuth(jwtManager), postHandler.DeletePost)

		// 게시글 추천/비추천 (프론트엔드 호환 토글 API)
		boardPosts.POST("/:id/like", goodHandler.LikePost)
		boardPosts.POST("/:id/dislike", goodHandler.DislikePost)
		boardPosts.GET("/:id/like-status", goodHandler.GetLikeStatus)

		// 게시글 추천/비추천 (명시적 API)
		boardPosts.POST("/:id/recommend", goodHandler.RecommendPost)
		boardPosts.DELETE("/:id/recommend", goodHandler.CancelRecommendPost)
		boardPosts.POST("/:id/downvote", goodHandler.DownvotePost)
		boardPosts.DELETE("/:id/downvote", goodHandler.CancelDownvotePost)

		// 스크랩
		boardPosts.POST("/:id/scrap", scrapHandler.AddScrap)
		boardPosts.DELETE("/:id/scrap", scrapHandler.RemoveScrap)

		// 댓글 관련 (파라미터 이름 통일: post_id -> id)
		comments := boardPosts.Group("/:id/comments")
		{
			comments.GET("", commentHandler.ListComments)
			comments.GET("/:comment_id", commentHandler.GetComment)
			comments.POST("", middleware.JWTAuth(jwtManager), commentHandler.CreateComment)
			comments.PUT("/:comment_id", middleware.JWTAuth(jwtManager), commentHandler.UpdateComment)
			comments.DELETE("/:comment_id", middleware.JWTAuth(jwtManager), commentHandler.DeleteComment)
			comments.POST("/:comment_id/like", middleware.JWTAuth(jwtManager), commentHandler.LikeComment)
			comments.POST("/:comment_id/dislike", middleware.JWTAuth(jwtManager), commentHandler.DislikeComment)
			comments.POST("/:comment_id/recommend", goodHandler.RecommendComment)
			comments.DELETE("/:comment_id/recommend", goodHandler.CancelRecommendComment)
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

	// Admin Menus (관리자 전용 - 개발 중 인증 비활성화)
	// TODO: 운영 환경에서는 middleware.JWTAuth(jwtManager) 추가 필요
	adminMenus := api.Group("/admin/menus")
	// adminMenus.Use(middleware.JWTAuth(jwtManager))
	adminMenus.GET("", menuHandler.GetAllMenusForAdmin)
	adminMenus.POST("", menuHandler.CreateMenu)
	adminMenus.PUT("/:id", menuHandler.UpdateMenu)
	adminMenus.DELETE("/:id", menuHandler.DeleteMenu)
	adminMenus.POST("/reorder", menuHandler.ReorderMenus)

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

	// Members (회원 검증 API - 공개)
	members := api.Group("/members")
	members.DELETE("/me", authHandler.Withdraw)                    // 회원 탈퇴 (본인만)
	members.POST("/check-id", memberHandler.CheckUserID)         // 회원 ID 중복 확인
	members.POST("/check-nickname", memberHandler.CheckNickname) // 닉네임 중복 확인
	members.POST("/check-email", memberHandler.CheckEmail)       // 이메일 중복 확인
	members.POST("/check-phone", memberHandler.CheckPhone)       // 휴대폰번호 중복 확인
	members.GET("/:id/nickname", memberHandler.GetNickname)      // 회원 닉네임 조회
	members.GET("/:id/profile", memberProfileHandler.GetProfile) // 회원 프로필 조회
	members.GET("/:id/posts", memberProfileHandler.GetPosts)     // 회원 작성글 조회
	members.GET("/:id/comments", memberProfileHandler.GetComments) // 회원 작성댓글 조회
	members.GET("/:id/points/history", memberProfileHandler.GetPointHistory) // 포인트 내역 (본인만)
	members.POST("/:id/block", blockHandler.BlockMember)                    // 회원 차단
	members.DELETE("/:id/block", blockHandler.UnblockMember)                // 차단 해제
	members.GET("/me/blocks", blockHandler.ListBlocks)                      // 차단 목록
	members.GET("/me/scraps", scrapHandler.ListScraps)                      // 내 스크랩 목록

	// Messages (쪽지 API - 로그인 필요)
	messages := api.Group("/messages")
	messages.POST("", messageHandler.SendMessage)        // 쪽지 보내기
	messages.GET("/inbox", messageHandler.GetInbox)      // 받은 쪽지함
	messages.GET("/sent", messageHandler.GetSent)        // 보낸 쪽지함
	messages.GET("/:id", messageHandler.GetMessage)      // 쪽지 상세
	messages.DELETE("/:id", messageHandler.DeleteMessage) // 쪽지 삭제

	// Autosave (자동 저장 API - 로그인 필요)
	autosave := api.Group("/autosave")
	autosave.POST("", autosaveHandler.Save)         // 자동 저장
	autosave.GET("", autosaveHandler.List)          // 목록 조회
	autosave.GET("/:id", autosaveHandler.Load)      // 불러오기
	autosave.DELETE("/:id", autosaveHandler.Delete) // 삭제

	// Filter (금지어 필터 API - 공개)
	filter := api.Group("/filter")
	filter.POST("/check", filterHandler.Check) // 금지어 검사

	// Tokens (CSRF 토큰 API - 공개)
	tokens := api.Group("/tokens")
	tokens.POST("/write", tokenHandler.GenerateWriteToken)     // 게시글 작성 토큰
	tokens.POST("/comment", tokenHandler.GenerateCommentToken) // 댓글 작성 토큰

	// Member Memo (회원 메모 API - 로그인 필요)
	memberMemo := members.Group("/:id/memo")
	memberMemo.GET("", memoHandler.GetMemo)       // 메모 조회
	memberMemo.POST("", memoHandler.CreateMemo)   // 메모 생성
	memberMemo.PUT("", memoHandler.UpdateMemo)    // 메모 수정
	memberMemo.DELETE("", memoHandler.DeleteMemo) // 메모 삭제

	// Reactions (게시글 반응 API)
	reactions := boardPosts.Group("/:id/reactions")
	reactions.GET("", reactionHandler.GetReactions) // 반응 목록
	reactions.POST("", reactionHandler.React)       // 반응 추가/제거

	// Reports (신고 API)
	reports := api.Group("/reports")
	reports.POST("", reportHandler.SubmitReport)           // 신고 접수 (일반 사용자)
	reports.GET("/mine", reportHandler.MyReports)          // 내 신고 내역
	reports.GET("", reportHandler.ListReports)             // 신고 목록 (관리자)
	reports.GET("/data", reportHandler.GetReportData)      // 신고 데이터 조회 (관리자)
	reports.GET("/recent", reportHandler.GetRecentReports) // 최근 신고 목록 (관리자)
	reports.GET("/stats", reportHandler.GetStats)          // 신고 통계 (관리자)
	reports.POST("/process", reportHandler.ProcessReport)  // 신고 처리 (관리자)

	// Disciplines (이용제한 API)
	disciplines := api.Group("/disciplines")
	disciplines.GET("/board", disciplineHandler.ListBoard)       // 이용제한 게시판
	disciplines.GET("/:id", disciplineHandler.GetDiscipline)     // 이용제한 상세 열람
	disciplines.POST("/:id/appeal", disciplineHandler.SubmitAppeal) // 소명 글 작성
	members.GET("/me/disciplines", disciplineHandler.MyDisciplines) // 내 이용제한 내역

	// Gallery (갤러리 API - 공개, Redis 캐시)
	gallery := api.Group("/gallery")
	gallery.GET("", galleryHandler.GetGalleryAll)          // 전체 갤러리
	gallery.GET("/:board_id", galleryHandler.GetGallery)   // 게시판별 갤러리

	// Unified Search (통합 검색 API - 공개, Redis 캐시)
	api.GET("/search", galleryHandler.UnifiedSearch)

	// Dajoongi (다중이 탐지 API - 관리자 전용)
	api.GET("/dajoongi", dajoongiHandler.GetDuplicateAccounts)

	// Notifications (알림 API - 로그인 필요)
	notifications := api.Group("/notifications")
	notifications.GET("/unread-count", notificationHandler.GetUnreadCount)
	notifications.GET("", notificationHandler.GetList)
	notifications.POST("/:id/read", notificationHandler.MarkAsRead)
	notifications.POST("/read-all", notificationHandler.MarkAllAsRead)
	notifications.DELETE("/:id", notificationHandler.Delete)

	// WebSocket (실시간 알림 스트림 - 로그인 필요)
	wsGroup := router.Group("/ws", middleware.DamoangCookieAuth(damoangJWT, cfg))
	wsGroup.GET("/notifications", wsHandler.Connect)

	// Upload (파일 업로드 API - 로그인 필요)
	upload := api.Group("/upload")
	upload.POST("/editor", fileHandler.UploadEditorImage)       // 에디터 이미지 업로드
	upload.POST("/attachment", fileHandler.UploadAttachment)     // 첨부파일 업로드

	// Files (파일 다운로드 API - 공개)
	files := api.Group("/files")
	files.GET("/:board_id/:wr_id/:file_no/download", fileHandler.DownloadFile) // 파일 다운로드

	// Admin Members (관리자 회원 관리)
	adminMembers := api.Group("/admin/members")
	// TODO: 운영 환경에서는 middleware.JWTAuth(jwtManager) + 관리자 권한 체크 추가 필요
	adminMembers.GET("", adminHandler.ListMembers)
	adminMembers.GET("/:id", adminHandler.GetMember)
	adminMembers.PUT("/:id", adminHandler.UpdateMember)
	adminMembers.POST("/:id/point", adminHandler.AdjustPoint)
	adminMembers.POST("/:id/restrict", adminHandler.RestrictMember)

	// ============================================
	// Plugin Routes (/api/plugins/*)
	// ============================================
	plugins := router.Group("/api/plugins", middleware.DamoangCookieAuth(damoangJWT, cfg))

	// Promotion Plugin (직홍게 플러그인)
	promotion := plugins.Group("/promotion")
	promotion.GET("/posts", promotionHandler.ListPromotionPosts)
	promotion.GET("/posts/insert", promotionHandler.GetPromotionPostsForInsert)
	promotion.GET("/posts/:id", promotionHandler.GetPromotionPost)
	promotion.POST("/posts", middleware.JWTAuth(jwtManager), promotionHandler.CreatePromotionPost)
	promotion.PUT("/posts/:id", middleware.JWTAuth(jwtManager), promotionHandler.UpdatePromotionPost)
	promotion.DELETE("/posts/:id", middleware.JWTAuth(jwtManager), promotionHandler.DeletePromotionPost)

	// Promotion Admin API
	promotionAdmin := promotion.Group("/admin")
	promotionAdmin.GET("/advertisers", promotionHandler.ListAdvertisers)
	promotionAdmin.POST("/advertisers", promotionHandler.CreateAdvertiser)
	promotionAdmin.PUT("/advertisers/:id", promotionHandler.UpdateAdvertiser)
	promotionAdmin.DELETE("/advertisers/:id", promotionHandler.DeleteAdvertiser)

	// Banner Plugin (배너 플러그인)
	banner := plugins.Group("/banner")
	banner.GET("/list", bannerHandler.ListBanners)
	banner.GET("/:id/click", bannerHandler.TrackClick)
	banner.POST("/:id/view", bannerHandler.TrackView)

	// Banner Admin API
	bannerAdmin := banner.Group("/admin")
	bannerAdmin.GET("/list", bannerHandler.ListAllBanners)
	bannerAdmin.POST("", bannerHandler.CreateBanner)
	bannerAdmin.PUT("/:id", bannerHandler.UpdateBanner)
	bannerAdmin.DELETE("/:id", bannerHandler.DeleteBanner)
	bannerAdmin.GET("/:id/stats", bannerHandler.GetBannerStats)
}
