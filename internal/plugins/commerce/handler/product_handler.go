package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/service"
	"github.com/gin-gonic/gin"
)

// ProductHandler 상품 HTTP 핸들러
type ProductHandler struct {
	service service.ProductService
}

// NewProductHandler 생성자
func NewProductHandler(svc service.ProductService) *ProductHandler {
	return &ProductHandler{service: svc}
}

// ListProducts godoc
// @Summary      내 상품 목록 조회
// @Description  판매자 본인의 상품 목록을 조회합니다
// @Tags         commerce-products
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page         query     int     false  "페이지 번호 (기본값: 1)"
// @Param        limit        query     int     false  "페이지당 항목 수 (기본값: 20, 최대: 100)"
// @Param        product_type query     string  false  "상품 유형 (digital, physical)"
// @Param        status       query     string  false  "상태 (draft, published, archived)"
// @Param        search       query     string  false  "검색어"
// @Param        sort_by      query     string  false  "정렬 기준 (created_at, updated_at, price, sales_count, view_count)"
// @Param        sort_order   query     string  false  "정렬 순서 (asc, desc)"
// @Success      200  {object}  common.APIResponse{data=[]domain.ProductResponse}
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/products [get]
func (h *ProductHandler) ListProducts(c *gin.Context) {
	sellerID, err := h.getSellerID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.ProductListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request parameters", err)
		return
	}

	products, meta, err := h.service.ListMyProducts(sellerID, &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch products", err)
		return
	}

	common.SuccessResponse(c, products, meta)
}

// CreateProduct godoc
// @Summary      상품 등록
// @Description  새 상품을 등록합니다
// @Tags         commerce-products
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.CreateProductRequest  true  "상품 등록 요청"
// @Success      201  {object}  common.APIResponse{data=domain.ProductResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/products [post]
func (h *ProductHandler) CreateProduct(c *gin.Context) {
	sellerID, err := h.getSellerID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	product, err := h.service.CreateProduct(sellerID, &req)
	if err != nil {
		if errors.Is(err, service.ErrSlugAlreadyExists) {
			common.ErrorResponse(c, http.StatusConflict, "Slug already exists", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to create product", err)
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: product})
}

// GetProduct godoc
// @Summary      내 상품 상세 조회
// @Description  판매자 본인의 상품 상세 정보를 조회합니다
// @Tags         commerce-products
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "상품 ID"
// @Success      200  {object}  common.APIResponse{data=domain.ProductResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Router       /plugins/commerce/products/{id} [get]
func (h *ProductHandler) GetProduct(c *gin.Context) {
	sellerID, err := h.getSellerID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	productID, err := h.getProductID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid product ID", err)
		return
	}

	product, err := h.service.GetMyProduct(sellerID, productID)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Product not found", err)
			return
		}
		if errors.Is(err, service.ErrProductForbidden) {
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch product", err)
		return
	}

	common.SuccessResponse(c, product, nil)
}

// UpdateProduct godoc
// @Summary      상품 수정
// @Description  상품 정보를 수정합니다
// @Tags         commerce-products
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                          true  "상품 ID"
// @Param        request  body      domain.UpdateProductRequest  true  "상품 수정 요청"
// @Success      200  {object}  common.APIResponse{data=domain.ProductResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/products/{id} [put]
func (h *ProductHandler) UpdateProduct(c *gin.Context) {
	sellerID, err := h.getSellerID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	productID, err := h.getProductID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid product ID", err)
		return
	}

	var req domain.UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	product, err := h.service.UpdateProduct(sellerID, productID, &req)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Product not found", err)
			return
		}
		if errors.Is(err, service.ErrProductForbidden) {
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
			return
		}
		if errors.Is(err, service.ErrSlugAlreadyExists) {
			common.ErrorResponse(c, http.StatusConflict, "Slug already exists", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to update product", err)
		return
	}

	common.SuccessResponse(c, product, nil)
}

// DeleteProduct godoc
// @Summary      상품 삭제
// @Description  상품을 삭제합니다 (소프트 삭제)
// @Tags         commerce-products
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "상품 ID"
// @Success      204  "No Content"
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/products/{id} [delete]
func (h *ProductHandler) DeleteProduct(c *gin.Context) {
	sellerID, err := h.getSellerID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	productID, err := h.getProductID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid product ID", err)
		return
	}

	err = h.service.DeleteProduct(sellerID, productID)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Product not found", err)
			return
		}
		if errors.Is(err, service.ErrProductForbidden) {
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete product", err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListShopProducts godoc
// @Summary      공개 상품 목록 조회
// @Description  공개된 상품 목록을 조회합니다
// @Tags         commerce-shop
// @Accept       json
// @Produce      json
// @Param        page         query     int     false  "페이지 번호 (기본값: 1)"
// @Param        limit        query     int     false  "페이지당 항목 수 (기본값: 20, 최대: 100)"
// @Param        product_type query     string  false  "상품 유형 (digital, physical)"
// @Param        search       query     string  false  "검색어"
// @Param        sort_by      query     string  false  "정렬 기준 (created_at, price, sales_count, view_count)"
// @Param        sort_order   query     string  false  "정렬 순서 (asc, desc)"
// @Success      200  {object}  common.APIResponse{data=[]domain.ProductResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/shop/products [get]
func (h *ProductHandler) ListShopProducts(c *gin.Context) {
	var req domain.ProductListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request parameters", err)
		return
	}

	products, meta, err := h.service.ListShopProducts(&req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch products", err)
		return
	}

	common.SuccessResponse(c, products, meta)
}

// GetShopProduct godoc
// @Summary      공개 상품 상세 조회
// @Description  공개된 상품의 상세 정보를 조회합니다
// @Tags         commerce-shop
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "상품 ID"
// @Success      200  {object}  common.APIResponse{data=domain.ProductResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/shop/products/{id} [get]
func (h *ProductHandler) GetShopProduct(c *gin.Context) {
	productID, err := h.getProductID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid product ID", err)
		return
	}

	product, err := h.service.GetShopProduct(productID)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Product not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch product", err)
		return
	}

	common.SuccessResponse(c, product, nil)
}

// GetShopProductBySlug godoc
// @Summary      슬러그로 공개 상품 조회
// @Description  슬러그로 공개된 상품의 상세 정보를 조회합니다
// @Tags         commerce-shop
// @Accept       json
// @Produce      json
// @Param        slug   path      string  true  "상품 슬러그"
// @Success      200  {object}  common.APIResponse{data=domain.ProductResponse}
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/shop/products/slug/{slug} [get]
func (h *ProductHandler) GetShopProductBySlug(c *gin.Context) {
	slug := c.Param("slug")
	if slug == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid slug", nil)
		return
	}

	product, err := h.service.GetShopProductBySlug(slug)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Product not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch product", err)
		return
	}

	common.SuccessResponse(c, product, nil)
}

// getSellerID JWT에서 판매자 ID 추출
func (h *ProductHandler) getSellerID(c *gin.Context) (uint64, error) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return 0, errors.New("user not authenticated")
	}

	// userID를 uint64로 변환 (mb_id는 문자열이지만 숫자인 경우가 많음)
	// 향후 users 테이블의 id로 변경 필요
	id, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		// 문자열 ID인 경우 해시값 사용 (임시)
		// 실제로는 users 테이블에서 조회해야 함
		return 0, errors.New("invalid user ID format")
	}
	return id, nil
}

// getProductID 경로에서 상품 ID 추출
func (h *ProductHandler) getProductID(c *gin.Context) (uint64, error) {
	idStr := c.Param("id")
	return strconv.ParseUint(idStr, 10, 64)
}
