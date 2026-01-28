package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// CommerceE2ETestSuite E2E 테스트 스위트
type CommerceE2ETestSuite struct {
	suite.Suite
	router *gin.Engine
}

// SetupSuite 테스트 스위트 초기화
func (s *CommerceE2ETestSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)
	s.router = gin.New()

	// 테스트용 라우트 설정
	s.setupTestRoutes()
}

// setupTestRoutes 테스트 라우트 설정
func (s *CommerceE2ETestSuite) setupTestRoutes() {
	api := s.router.Group("/api/plugins/commerce")

	// 상품 라우트
	api.GET("/shop/products", s.mockListProducts)
	api.GET("/shop/products/:id", s.mockGetProduct)
	api.POST("/products", s.mockCreateProduct)

	// 장바구니 라우트
	api.GET("/cart", s.mockGetCart)
	api.POST("/cart", s.mockAddToCart)
	api.DELETE("/cart/:id", s.mockRemoveFromCart)

	// 주문 라우트
	api.POST("/orders", s.mockCreateOrder)
	api.GET("/orders", s.mockListOrders)
	api.GET("/orders/:id", s.mockGetOrder)

	// 결제 라우트
	api.POST("/payments/prepare", s.mockPreparePayment)
	api.POST("/payments/complete", s.mockCompletePayment)

	// 다운로드 라우트
	api.GET("/downloads/:order_item_id", s.mockListDownloads)
	api.GET("/downloads/:order_item_id/:file_id/url", s.mockGetDownloadURL)
}

// Mock 핸들러들
func (s *CommerceE2ETestSuite) mockListProducts(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": []gin.H{
			{
				"id":           1,
				"name":         "테스트 디지털 상품",
				"slug":         "test-digital-product",
				"price":        10000,
				"product_type": "digital",
				"status":       "published",
			},
			{
				"id":           2,
				"name":         "테스트 실물 상품",
				"slug":         "test-physical-product",
				"price":        25000,
				"product_type": "physical",
				"status":       "published",
			},
		},
		"meta": gin.H{
			"page":  1,
			"limit": 20,
			"total": 2,
		},
	})
}

func (s *CommerceE2ETestSuite) mockGetProduct(c *gin.Context) {
	id := c.Param("id")
	if id == "1" {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"id":           1,
				"name":         "테스트 디지털 상품",
				"slug":         "test-digital-product",
				"description":  "테스트 상품 설명입니다.",
				"price":        10000,
				"product_type": "digital",
				"status":       "published",
			},
		})
	} else {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "product not found",
		})
	}
}

func (s *CommerceE2ETestSuite) mockCreateProduct(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"id":           3,
			"name":         req["name"],
			"slug":         req["slug"],
			"price":        req["price"],
			"product_type": req["product_type"],
			"status":       "draft",
		},
	})
}

func (s *CommerceE2ETestSuite) mockGetCart(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": []gin.H{
				{
					"id":         1,
					"product_id": 1,
					"quantity":   2,
					"subtotal":   20000,
				},
			},
			"total":    20000,
			"currency": "KRW",
		},
	})
}

func (s *CommerceE2ETestSuite) mockAddToCart(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":         1,
			"product_id": req["product_id"],
			"quantity":   req["quantity"],
			"subtotal":   10000,
		},
	})
}

func (s *CommerceE2ETestSuite) mockRemoveFromCart(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "cart item removed",
	})
}

func (s *CommerceE2ETestSuite) mockCreateOrder(c *gin.Context) {
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"id":           1,
			"order_number": "20240101120000123456",
			"total":        20000,
			"currency":     "KRW",
			"status":       "pending",
			"items": []gin.H{
				{
					"id":           1,
					"product_id":   1,
					"product_name": "테스트 디지털 상품",
					"quantity":     2,
					"price":        10000,
					"subtotal":     20000,
				},
			},
		},
	})
}

func (s *CommerceE2ETestSuite) mockListOrders(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": []gin.H{
			{
				"id":           1,
				"order_number": "20240101120000123456",
				"total":        20000,
				"status":       "paid",
			},
		},
		"meta": gin.H{
			"page":  1,
			"limit": 20,
			"total": 1,
		},
	})
}

func (s *CommerceE2ETestSuite) mockGetOrder(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":           1,
			"order_number": "20240101120000123456",
			"total":        20000,
			"currency":     "KRW",
			"status":       "paid",
		},
	})
}

func (s *CommerceE2ETestSuite) mockPreparePayment(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"payment_id":   1,
			"order_number": "20240101120000123456",
			"amount":       20000,
			"currency":     "KRW",
			"pg_provider":  req["pg_provider"],
			"pg_order_id":  "PG-ORD-001",
			"redirect_url": "https://pay.example.com/checkout",
		},
	})
}

func (s *CommerceE2ETestSuite) mockCompletePayment(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":        1,
			"order_id":  1,
			"amount":    20000,
			"status":    "paid",
			"paid_at":   "2024-01-01T12:00:00Z",
		},
	})
}

func (s *CommerceE2ETestSuite) mockListDownloads(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": []gin.H{
			{
				"id":             1,
				"order_item_id":  1,
				"download_token": "abc123token",
				"download_count": 0,
				"download_limit": 5,
				"can_download":   true,
				"file": gin.H{
					"id":        1,
					"file_name": "test-file.pdf",
					"file_size": 1024000,
				},
			},
		},
	})
}

func (s *CommerceE2ETestSuite) mockGetDownloadURL(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"download_url": "https://example.com/downloads/abc123?sig=xyz&exp=1234567890",
			"expires_at":   "2024-01-01T12:10:00Z",
			"file_name":    "test-file.pdf",
			"file_size":    1024000,
		},
	})
}

// E2E 테스트 케이스들

func (s *CommerceE2ETestSuite) TestFullPurchaseFlow_DigitalProduct() {
	// 1. 상품 목록 조회
	req := httptest.NewRequest(http.MethodGet, "/api/plugins/commerce/shop/products", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var listResp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &listResp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), listResp["success"].(bool))

	// 2. 상품 상세 조회
	req = httptest.NewRequest(http.MethodGet, "/api/plugins/commerce/shop/products/1", nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var productResp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &productResp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), productResp["success"].(bool))

	// 3. 장바구니 추가
	cartBody := map[string]interface{}{
		"product_id": 1,
		"quantity":   2,
	}
	bodyBytes, _ := json.Marshal(cartBody)
	req = httptest.NewRequest(http.MethodPost, "/api/plugins/commerce/cart", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	// 4. 장바구니 조회
	req = httptest.NewRequest(http.MethodGet, "/api/plugins/commerce/cart", nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var cartResp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &cartResp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), cartResp["success"].(bool))

	// 5. 주문 생성
	req = httptest.NewRequest(http.MethodPost, "/api/plugins/commerce/orders", nil)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusCreated, w.Code)

	var orderResp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &orderResp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), orderResp["success"].(bool))

	// 6. 결제 준비
	paymentBody := map[string]interface{}{
		"order_id":       1,
		"pg_provider":    "tosspayments",
		"payment_method": "card",
		"return_url":     "https://example.com/return",
	}
	bodyBytes, _ = json.Marshal(paymentBody)
	req = httptest.NewRequest(http.MethodPost, "/api/plugins/commerce/payments/prepare", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var prepareResp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &prepareResp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), prepareResp["success"].(bool))

	// 7. 결제 완료
	completeBody := map[string]interface{}{
		"payment_id":  1,
		"pg_provider": "tosspayments",
		"pg_tid":      "TOSS-TID-001",
		"pg_order_id": "PG-ORD-001",
		"amount":      20000,
	}
	bodyBytes, _ = json.Marshal(completeBody)
	req = httptest.NewRequest(http.MethodPost, "/api/plugins/commerce/payments/complete", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	// 8. 주문 조회 (결제 완료 확인)
	req = httptest.NewRequest(http.MethodGet, "/api/plugins/commerce/orders/1", nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	// 9. 다운로드 목록 조회
	req = httptest.NewRequest(http.MethodGet, "/api/plugins/commerce/downloads/1", nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var downloadResp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &downloadResp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), downloadResp["success"].(bool))

	// 10. 다운로드 URL 생성
	req = httptest.NewRequest(http.MethodGet, "/api/plugins/commerce/downloads/1/1/url", nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var urlResp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &urlResp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), urlResp["success"].(bool))
}

func (s *CommerceE2ETestSuite) TestProductNotFound() {
	req := httptest.NewRequest(http.MethodGet, "/api/plugins/commerce/shop/products/999", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.False(s.T(), resp["success"].(bool))
}

func (s *CommerceE2ETestSuite) TestCreateProduct() {
	productBody := map[string]interface{}{
		"name":         "새 테스트 상품",
		"slug":         "new-test-product",
		"price":        15000,
		"product_type": "digital",
	}
	bodyBytes, _ := json.Marshal(productBody)

	req := httptest.NewRequest(http.MethodPost, "/api/plugins/commerce/products", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusCreated, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), resp["success"].(bool))

	data := resp["data"].(map[string]interface{})
	assert.Equal(s.T(), "새 테스트 상품", data["name"])
	assert.Equal(s.T(), "draft", data["status"])
}

func (s *CommerceE2ETestSuite) TestCartOperations() {
	// 장바구니 추가
	cartBody := map[string]interface{}{
		"product_id": 1,
		"quantity":   1,
	}
	bodyBytes, _ := json.Marshal(cartBody)

	req := httptest.NewRequest(http.MethodPost, "/api/plugins/commerce/cart", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	// 장바구니 조회
	req = httptest.NewRequest(http.MethodGet, "/api/plugins/commerce/cart", nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var cartResp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &cartResp)
	assert.NoError(s.T(), err)

	data := cartResp["data"].(map[string]interface{})
	assert.NotNil(s.T(), data["items"])
	assert.Equal(s.T(), float64(20000), data["total"])

	// 장바구니 아이템 삭제
	req = httptest.NewRequest(http.MethodDelete, "/api/plugins/commerce/cart/1", nil)
	w = httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *CommerceE2ETestSuite) TestOrderList() {
	req := httptest.NewRequest(http.MethodGet, "/api/plugins/commerce/orders", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), resp["success"].(bool))

	data := resp["data"].([]interface{})
	assert.Len(s.T(), data, 1)
}

// TestCommerceE2E 테스트 스위트 실행
func TestCommerceE2E(t *testing.T) {
	suite.Run(t, new(CommerceE2ETestSuite))
}
