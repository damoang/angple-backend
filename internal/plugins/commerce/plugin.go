package commerce

import (
	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/plugins/commerce/carrier"
	"github.com/damoang/angple-backend/internal/plugins/commerce/gateway"
	"github.com/damoang/angple-backend/internal/plugins/commerce/handler"
	"github.com/damoang/angple-backend/internal/plugins/commerce/middleware"
	"github.com/damoang/angple-backend/internal/plugins/commerce/repository"
	"github.com/damoang/angple-backend/internal/plugins/commerce/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// CommercePlugin 커머스 플러그인
type CommercePlugin struct {
	db                *gorm.DB
	redis             *redis.Client
	logger            plugin.Logger
	rateLimiter       *middleware.RateLimiter
	productHandler    *handler.ProductHandler
	cartHandler       *handler.CartHandler
	orderHandler      *handler.OrderHandler
	paymentHandler    *handler.PaymentHandler
	downloadHandler   *handler.DownloadHandler
	settlementHandler *handler.SettlementHandler
	couponHandler     *handler.CouponHandler
	reviewHandler     *handler.ReviewHandler
	shippingHandler   *handler.ShippingHandler
}

// Manifest 커머스 플러그인 매니페스트
var Manifest = &plugin.PluginManifest{
	Name:        "commerce",
	Version:     "1.0.0",
	Title:       "Commerce",
	Description: "SaaS/Self-hosting 가능한 이커머스 플러그인 (디지털/실물 상품 판매)",
	Author:      "SDK Corporation",
	License:     "Proprietary",
	Requires: plugin.Requires{
		Angple: ">=1.0.0",
	},
	// Admin 메뉴 정의 (Phase 9)
	Menus: []plugin.MenuConfig{
		{
			Title:         "Commerce",
			URL:           "/admin/commerce",
			Icon:          "shopping-cart",
			ShowInSidebar: true,
			OrderNum:      100,
			ViewLevel:     9, // Admin 전용
		},
		{
			Title:         "상품 관리",
			URL:           "/admin/commerce/products",
			Icon:          "package",
			ParentPath:    "/admin/commerce",
			ShowInSidebar: true,
			OrderNum:      1,
			ViewLevel:     9,
		},
		{
			Title:         "주문 관리",
			URL:           "/admin/commerce/orders",
			Icon:          "clipboard-list",
			ParentPath:    "/admin/commerce",
			ShowInSidebar: true,
			OrderNum:      2,
			ViewLevel:     9,
		},
		{
			Title:         "정산 관리",
			URL:           "/admin/commerce/settlements",
			Icon:          "calculator",
			ParentPath:    "/admin/commerce",
			ShowInSidebar: true,
			OrderNum:      3,
			ViewLevel:     9,
		},
		{
			Title:         "쿠폰 관리",
			URL:           "/admin/commerce/coupons",
			Icon:          "ticket",
			ParentPath:    "/admin/commerce",
			ShowInSidebar: true,
			OrderNum:      4,
			ViewLevel:     9,
		},
		{
			Title:         "리뷰 관리",
			URL:           "/admin/commerce/reviews",
			Icon:          "star",
			ParentPath:    "/admin/commerce",
			ShowInSidebar: true,
			OrderNum:      5,
			ViewLevel:     9,
		},
	},
	Routes: []plugin.RouteConfig{
		// 상품 관리 API (Phase 2)
		{Path: "/products", Method: "GET", Handler: "ListProducts", Auth: "required"},
		{Path: "/products", Method: "POST", Handler: "CreateProduct", Auth: "required"},
		{Path: "/products/:id", Method: "GET", Handler: "GetProduct", Auth: "required"},
		{Path: "/products/:id", Method: "PUT", Handler: "UpdateProduct", Auth: "required"},
		{Path: "/products/:id", Method: "DELETE", Handler: "DeleteProduct", Auth: "required"},

		// 공개 상품 목록 (Phase 2)
		{Path: "/shop/products", Method: "GET", Handler: "ListShopProducts", Auth: "none"},
		{Path: "/shop/products/:id", Method: "GET", Handler: "GetShopProduct", Auth: "none"},
		{Path: "/shop/products/slug/:slug", Method: "GET", Handler: "GetShopProductBySlug", Auth: "none"},

		// 장바구니 API (Phase 3)
		{Path: "/cart", Method: "GET", Handler: "GetCart", Auth: "required"},
		{Path: "/cart", Method: "POST", Handler: "AddToCart", Auth: "required"},
		{Path: "/cart/:id", Method: "PUT", Handler: "UpdateCartItem", Auth: "required"},
		{Path: "/cart/:id", Method: "DELETE", Handler: "RemoveFromCart", Auth: "required"},
		{Path: "/cart", Method: "DELETE", Handler: "ClearCart", Auth: "required"},

		// 주문 API (Phase 3)
		{Path: "/orders", Method: "GET", Handler: "ListOrders", Auth: "required"},
		{Path: "/orders", Method: "POST", Handler: "CreateOrder", Auth: "required"},
		{Path: "/orders/:id", Method: "GET", Handler: "GetOrder", Auth: "required"},
		{Path: "/orders/:id/cancel", Method: "POST", Handler: "CancelOrder", Auth: "required"},

		// 결제 API (Phase 4)
		{Path: "/payments/prepare", Method: "POST", Handler: "PreparePayment", Auth: "required"},
		{Path: "/payments/complete", Method: "POST", Handler: "CompletePayment", Auth: "required"},
		{Path: "/payments/:id/cancel", Method: "POST", Handler: "CancelPayment", Auth: "required"},
		{Path: "/payments/:id", Method: "GET", Handler: "GetPayment", Auth: "required"},
		{Path: "/webhooks/:provider", Method: "POST", Handler: "HandleWebhook", Auth: "none"},

		// 다운로드 API (Phase 5)
		{Path: "/downloads", Method: "GET", Handler: "ListUserDownloads", Auth: "required"},
		{Path: "/downloads/:order_item_id/:file_id", Method: "GET", Handler: "GetDownloadURL", Auth: "required"},
		{Path: "/downloads/:token", Method: "GET", Handler: "Download", Auth: "required"},
		{Path: "/orders/:order_item_id/downloads", Method: "GET", Handler: "ListDownloads", Auth: "required"},

		// 정산 API (Phase 6)
		{Path: "/settlements", Method: "GET", Handler: "ListSettlements", Auth: "required"},
		{Path: "/settlements/summary", Method: "GET", Handler: "GetSummary", Auth: "required"},
		{Path: "/settlements/:id", Method: "GET", Handler: "GetSettlement", Auth: "required"},
		{Path: "/admin/settlements", Method: "GET", Handler: "ListAllSettlements", Auth: "required"},
		{Path: "/admin/settlements/:seller_id", Method: "POST", Handler: "CreateSettlement", Auth: "required"},
		{Path: "/admin/settlements/:id/process", Method: "POST", Handler: "ProcessSettlement", Auth: "required"},

		// 쿠폰 API (Phase 8)
		{Path: "/coupons/public", Method: "GET", Handler: "GetPublicCoupons", Auth: "none"},
		{Path: "/coupons/validate", Method: "POST", Handler: "ValidateCoupon", Auth: "required"},
		{Path: "/coupons/apply", Method: "POST", Handler: "ApplyCoupon", Auth: "required"},
		{Path: "/orders/:order_id/coupon", Method: "DELETE", Handler: "RemoveCoupon", Auth: "required"},
		{Path: "/admin/coupons", Method: "GET", Handler: "ListCoupons", Auth: "required"},
		{Path: "/admin/coupons", Method: "POST", Handler: "CreateCoupon", Auth: "required"},
		{Path: "/admin/coupons/:id", Method: "GET", Handler: "GetCoupon", Auth: "required"},
		{Path: "/admin/coupons/:id", Method: "PUT", Handler: "UpdateCoupon", Auth: "required"},
		{Path: "/admin/coupons/:id", Method: "DELETE", Handler: "DeleteCoupon", Auth: "required"},

		// 리뷰 API (Phase 8)
		{Path: "/products/:product_id/reviews", Method: "GET", Handler: "ListProductReviews", Auth: "none"},
		{Path: "/products/:product_id/reviews", Method: "POST", Handler: "CreateReview", Auth: "required"},
		{Path: "/products/:product_id/reviews/summary", Method: "GET", Handler: "GetProductReviewSummary", Auth: "none"},
		{Path: "/reviews", Method: "GET", Handler: "ListMyReviews", Auth: "required"},
		{Path: "/reviews/:id", Method: "GET", Handler: "GetReview", Auth: "none"},
		{Path: "/reviews/:id", Method: "PUT", Handler: "UpdateReview", Auth: "required"},
		{Path: "/reviews/:id", Method: "DELETE", Handler: "DeleteReview", Auth: "required"},
		{Path: "/reviews/:id/helpful", Method: "POST", Handler: "ToggleHelpful", Auth: "required"},
		{Path: "/seller/reviews", Method: "GET", Handler: "ListSellerReviews", Auth: "required"},
		{Path: "/seller/reviews/:id/reply", Method: "POST", Handler: "ReplyToReview", Auth: "required"},

		// 배송 추적 API (Phase 8)
		{Path: "/shipping/carriers", Method: "GET", Handler: "GetCarriers", Auth: "none"},
		{Path: "/seller/orders/:order_id/shipping", Method: "POST", Handler: "RegisterShipping", Auth: "required"},
		{Path: "/orders/:order_id/tracking", Method: "GET", Handler: "TrackShipping", Auth: "required"},
		{Path: "/seller/orders/:order_id/delivered", Method: "POST", Handler: "MarkDelivered", Auth: "required"},
	},
}

// PluginConfig 플러그인 설정
type PluginConfig struct {
	// PG 설정
	Inicis struct {
		MerchantID string `yaml:"merchant_id"`
		SignKey    string `yaml:"sign_key"`
		APIKey     string `yaml:"api_key"`
		IsSandbox  bool   `yaml:"is_sandbox"`
	} `yaml:"inicis"`

	TossPayments struct {
		ClientKey string `yaml:"client_key"`
		SecretKey string `yaml:"secret_key"`
		IsSandbox bool   `yaml:"is_sandbox"`
	} `yaml:"tosspayments"`

	KakaoPay struct {
		CID         string `yaml:"cid"`
		AdminKey    string `yaml:"admin_key"`
		IsSandbox   bool   `yaml:"is_sandbox"`
		ApprovalURL string `yaml:"approval_url"`
		CancelURL   string `yaml:"cancel_url"`
		FailURL     string `yaml:"fail_url"`
	} `yaml:"kakaopay"`

	// 다운로드 설정
	Download struct {
		BaseURL     string `yaml:"base_url"`
		SecretKey   string `yaml:"secret_key"`
		StoragePath string `yaml:"storage_path"`
	} `yaml:"download"`

	// 수수료 설정
	Fee struct {
		PGRate       float64 `yaml:"pg_rate"`       // PG 수수료율 (기본 3.3%)
		PlatformRate float64 `yaml:"platform_rate"` // 플랫폼 수수료율 (기본 5%)
	} `yaml:"fee"`

	// 배송 추적 설정 (Phase 8)
	Shipping struct {
		SweetTrackerAPIKey string `yaml:"sweettracker_api_key"` // SweetTracker API 키 (선택)
	} `yaml:"shipping"`
}

// New 새 커머스 플러그인 생성
func New() *CommercePlugin {
	return &CommercePlugin{}
}

// Name 플러그인 이름 반환
func (p *CommercePlugin) Name() string {
	return "commerce"
}

// Initialize 플러그인 초기화
func (p *CommercePlugin) Initialize(ctx *plugin.PluginContext) error {
	p.db = ctx.DB
	p.redis = ctx.Redis
	p.logger = ctx.Logger

	// TODO: 설정 파일에서 읽어오기
	config := &PluginConfig{}
	config.Download.BaseURL = "http://localhost:8082"
	config.Download.SecretKey = "commerce-download-secret-key"
	config.Download.StoragePath = "./storage/commerce"

	// ============================================
	// Rate Limiter 생성 (Phase 7)
	// ============================================
	if p.redis != nil {
		p.rateLimiter = middleware.NewRateLimiter(p.redis, nil)
		p.logger.Info("Rate limiting enabled")
	}

	// ============================================
	// DI: Repository 생성
	// ============================================
	baseProductRepo := repository.NewProductRepository(p.db)
	var productRepo repository.ProductRepository
	if p.redis != nil {
		// Redis가 있으면 캐싱 적용
		productRepo = repository.NewCachedProductRepository(baseProductRepo, p.redis, nil)
		p.logger.Info("Product repository caching enabled")
	} else {
		productRepo = baseProductRepo
	}
	cartRepo := repository.NewCartRepository(p.db)
	orderRepo := repository.NewOrderRepository(p.db)
	paymentRepo := repository.NewPaymentRepository(p.db)
	downloadRepo := repository.NewDownloadRepository(p.db)
	productFileRepo := repository.NewProductFileRepository(p.db)
	settlementRepo := repository.NewSettlementRepository(p.db)
	couponRepo := repository.NewCouponRepository(p.db)
	couponUsageRepo := repository.NewCouponUsageRepository(p.db)
	reviewRepo := repository.NewReviewRepository(p.db)
	reviewHelpfulRepo := repository.NewReviewHelpfulRepository(p.db)

	// ============================================
	// DI: Gateway Manager 생성 (Phase 4)
	// ============================================
	gatewayManager := gateway.NewGatewayManager()

	// KG이니시스 게이트웨이 등록
	inicisGateway := gateway.NewInicisGateway(&gateway.InicisConfig{
		MerchantID: config.Inicis.MerchantID,
		SignKey:    config.Inicis.SignKey,
		APIKey:     config.Inicis.APIKey,
		IsSandbox:  config.Inicis.IsSandbox,
	})
	gatewayManager.Register(inicisGateway)

	// 토스페이먼츠 게이트웨이 등록
	tossGateway := gateway.NewTossPaymentsGateway(&gateway.TossPaymentsConfig{
		ClientKey: config.TossPayments.ClientKey,
		SecretKey: config.TossPayments.SecretKey,
		IsSandbox: config.TossPayments.IsSandbox,
	})
	gatewayManager.Register(tossGateway)

	// 카카오페이 게이트웨이 등록 (Phase 8)
	kakaoGateway := gateway.NewKakaoPayGateway(&gateway.KakaoPayConfig{
		CID:         config.KakaoPay.CID,
		AdminKey:    config.KakaoPay.AdminKey,
		IsSandbox:   config.KakaoPay.IsSandbox,
		ApprovalURL: config.KakaoPay.ApprovalURL,
		CancelURL:   config.KakaoPay.CancelURL,
		FailURL:     config.KakaoPay.FailURL,
	})
	gatewayManager.Register(kakaoGateway)

	// ============================================
	// DI: Service 생성 (Repository 주입)
	// ============================================
	productService := service.NewProductService(productRepo)
	cartService := service.NewCartService(cartRepo, productRepo)
	orderService := service.NewOrderService(orderRepo, cartRepo, productRepo)
	paymentService := service.NewPaymentService(paymentRepo, orderRepo, productRepo, gatewayManager)
	downloadService := service.NewDownloadService(downloadRepo, orderRepo, productFileRepo)
	settlementService := service.NewSettlementService(settlementRepo, orderRepo)
	couponService := service.NewCouponService(couponRepo, couponUsageRepo, orderRepo)
	reviewService := service.NewReviewService(reviewRepo, reviewHelpfulRepo, orderRepo, productRepo)

	// ============================================
	// DI: Carrier Manager 생성 (Phase 8 - 배송 추적)
	// ============================================
	carrierManager := carrier.NewCarrierManager()

	// CJ대한통운 배송 추적 등록
	cjCarrier := carrier.NewCJCarrier(config.Shipping.SweetTrackerAPIKey)
	carrierManager.Register(cjCarrier)

	// 롯데택배 배송 추적 등록
	lotteCarrier := carrier.NewLotteCarrier(config.Shipping.SweetTrackerAPIKey)
	carrierManager.Register(lotteCarrier)

	// 배송 서비스 생성
	shippingService := service.NewShippingService(orderRepo, carrierManager)

	// ============================================
	// DI: Handler 생성 (Service 주입)
	// ============================================
	p.productHandler = handler.NewProductHandler(productService)
	p.cartHandler = handler.NewCartHandler(cartService)
	p.orderHandler = handler.NewOrderHandler(orderService)
	p.paymentHandler = handler.NewPaymentHandler(paymentService)
	p.downloadHandler = handler.NewDownloadHandler(downloadService, &handler.DownloadHandlerConfig{
		BaseURL:     config.Download.BaseURL,
		SecretKey:   config.Download.SecretKey,
		StoragePath: config.Download.StoragePath,
	})
	p.settlementHandler = handler.NewSettlementHandler(settlementService)
	p.couponHandler = handler.NewCouponHandler(couponService)
	p.reviewHandler = handler.NewReviewHandler(reviewService)
	p.shippingHandler = handler.NewShippingHandler(shippingService)

	p.logger.Info("Commerce plugin initialized")
	return nil
}

// RegisterRoutes 라우트 등록
func (p *CommercePlugin) RegisterRoutes(router gin.IRouter) {
	// ============================================
	// 상품 관리 (판매자용) - Phase 2 구현 완료
	// ============================================
	router.GET("/products", p.productHandler.ListProducts)
	router.POST("/products", p.productHandler.CreateProduct)
	router.GET("/products/:id", p.productHandler.GetProduct)
	router.PUT("/products/:id", p.productHandler.UpdateProduct)
	router.DELETE("/products/:id", p.productHandler.DeleteProduct)

	// ============================================
	// 공개 상점 - Phase 2 구현 완료
	// ============================================
	router.GET("/shop/products", p.productHandler.ListShopProducts)
	router.GET("/shop/products/:id", p.productHandler.GetShopProduct)
	router.GET("/shop/products/slug/:slug", p.productHandler.GetShopProductBySlug)

	// ============================================
	// 장바구니 - Phase 3 구현 완료
	// ============================================
	router.GET("/cart", p.cartHandler.GetCart)
	router.POST("/cart", p.cartHandler.AddToCart)
	router.PUT("/cart/:id", p.cartHandler.UpdateCartItem)
	router.DELETE("/cart/:id", p.cartHandler.RemoveFromCart)
	router.DELETE("/cart", p.cartHandler.ClearCart)

	// ============================================
	// 주문 - Phase 3 구현 완료
	// ============================================
	router.GET("/orders", p.orderHandler.ListOrders)
	router.POST("/orders", p.orderHandler.CreateOrder)
	router.GET("/orders/:id", p.orderHandler.GetOrder)
	router.POST("/orders/:id/cancel", p.orderHandler.CancelOrder)

	// ============================================
	// 결제 - Phase 4 구현 완료 + Rate Limiting (Phase 7)
	// ============================================
	if p.rateLimiter != nil {
		// 결제 관련 엔드포인트는 분당 30회로 제한 (더 엄격한 제한)
		paymentRateLimit := p.rateLimiter.CustomRateLimit(30)
		router.POST("/payments/prepare", paymentRateLimit, p.paymentHandler.PreparePayment)
		router.POST("/payments/complete", paymentRateLimit, p.paymentHandler.CompletePayment)
		router.POST("/payments/:id/cancel", paymentRateLimit, p.paymentHandler.CancelPayment)
		router.GET("/payments/:id", p.paymentHandler.GetPayment)
		router.POST("/webhooks/:provider", p.paymentHandler.HandleWebhook) // 웹훅은 Rate Limit 없음
	} else {
		router.POST("/payments/prepare", p.paymentHandler.PreparePayment)
		router.POST("/payments/complete", p.paymentHandler.CompletePayment)
		router.POST("/payments/:id/cancel", p.paymentHandler.CancelPayment)
		router.GET("/payments/:id", p.paymentHandler.GetPayment)
		router.POST("/webhooks/:provider", p.paymentHandler.HandleWebhook)
	}

	// ============================================
	// 다운로드 - Phase 5 구현 완료
	// ============================================
	router.GET("/downloads", p.downloadHandler.ListUserDownloads)
	router.GET("/downloads/:order_item_id/:file_id", p.downloadHandler.GetDownloadURL)
	router.GET("/downloads/:token", p.downloadHandler.Download)
	router.GET("/orders/:order_item_id/downloads", p.downloadHandler.ListDownloads)

	// ============================================
	// 정산 - Phase 6 구현 완료
	// ============================================
	router.GET("/settlements", p.settlementHandler.ListSettlements)
	router.GET("/settlements/summary", p.settlementHandler.GetSummary)
	router.GET("/settlements/:id", p.settlementHandler.GetSettlement)

	// 관리자 정산 API
	admin := router.Group("/admin")
	admin.GET("/settlements", p.settlementHandler.ListAllSettlements)
	admin.POST("/settlements/:seller_id", p.settlementHandler.CreateSettlement)
	admin.POST("/settlements/:id/process", p.settlementHandler.ProcessSettlement)

	// ============================================
	// 쿠폰 - Phase 8 구현 완료
	// ============================================
	// 공개 쿠폰 목록 (인증 불필요)
	router.GET("/coupons/public", p.couponHandler.GetPublicCoupons)

	// 사용자 쿠폰 API (인증 필요)
	router.POST("/coupons/validate", p.couponHandler.ValidateCoupon)
	router.POST("/coupons/apply", p.couponHandler.ApplyCoupon)
	router.DELETE("/orders/:order_id/coupon", p.couponHandler.RemoveCoupon)

	// 관리자 쿠폰 API
	admin.GET("/coupons", p.couponHandler.ListCoupons)
	admin.POST("/coupons", p.couponHandler.CreateCoupon)
	admin.GET("/coupons/:id", p.couponHandler.GetCoupon)
	admin.PUT("/coupons/:id", p.couponHandler.UpdateCoupon)
	admin.DELETE("/coupons/:id", p.couponHandler.DeleteCoupon)

	// ============================================
	// 리뷰 - Phase 8 구현 완료
	// ============================================
	// 상품 리뷰 (공개)
	router.GET("/products/:product_id/reviews", p.reviewHandler.ListProductReviews)
	router.GET("/products/:product_id/reviews/summary", p.reviewHandler.GetProductReviewSummary)
	router.POST("/products/:product_id/reviews", p.reviewHandler.CreateReview)

	// 사용자 리뷰 API
	router.GET("/reviews", p.reviewHandler.ListMyReviews)
	router.GET("/reviews/:id", p.reviewHandler.GetReview)
	router.PUT("/reviews/:id", p.reviewHandler.UpdateReview)
	router.DELETE("/reviews/:id", p.reviewHandler.DeleteReview)
	router.POST("/reviews/:id/helpful", p.reviewHandler.ToggleHelpful)

	// 판매자 리뷰 API
	seller := router.Group("/seller")
	seller.GET("/reviews", p.reviewHandler.ListSellerReviews)
	seller.POST("/reviews/:id/reply", p.reviewHandler.ReplyToReview)

	// ============================================
	// 배송 추적 - Phase 8 구현 완료
	// ============================================
	// 배송사 목록 (공개)
	router.GET("/shipping/carriers", p.shippingHandler.GetCarriers)

	// 배송 추적 (구매자용)
	router.GET("/orders/:order_id/tracking", p.shippingHandler.TrackShipping)

	// 판매자 배송 관리
	seller.POST("/orders/:order_id/shipping", p.shippingHandler.RegisterShipping)
	seller.POST("/orders/:order_id/delivered", p.shippingHandler.MarkDelivered)

	p.logger.Info("Commerce routes registered")
}

// Shutdown 플러그인 종료
func (p *CommercePlugin) Shutdown() error {
	p.logger.Info("Commerce plugin shutdown")
	return nil
}
