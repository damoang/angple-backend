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

// CartHandler 장바구니 HTTP 핸들러
type CartHandler struct {
	service service.CartService
}

// NewCartHandler 생성자
func NewCartHandler(svc service.CartService) *CartHandler {
	return &CartHandler{service: svc}
}

// GetCart godoc
// @Summary      장바구니 조회
// @Description  사용자의 장바구니를 조회합니다
// @Tags         commerce-cart
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  common.APIResponse{data=domain.CartResponse}
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/cart [get]
func (h *CartHandler) GetCart(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	cart, err := h.service.GetCart(userID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch cart", err)
		return
	}

	common.SuccessResponse(c, cart, nil)
}

// AddToCart godoc
// @Summary      장바구니에 상품 추가
// @Description  장바구니에 상품을 추가합니다. 이미 있는 상품이면 수량이 증가합니다.
// @Tags         commerce-cart
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.AddToCartRequest  true  "장바구니 추가 요청"
// @Success      201  {object}  common.APIResponse{data=domain.CartItemResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/cart [post]
func (h *CartHandler) AddToCart(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.AddToCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	item, err := h.service.AddToCart(userID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProductNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Product not found", err)
		case errors.Is(err, service.ErrProductNotAvailable):
			common.ErrorResponse(c, http.StatusBadRequest, "Product is not available", err)
		case errors.Is(err, service.ErrInsufficientStock):
			common.ErrorResponse(c, http.StatusConflict, "Insufficient stock", err)
		case errors.Is(err, service.ErrInvalidQuantity):
			common.ErrorResponse(c, http.StatusBadRequest, "Invalid quantity", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to add to cart", err)
		}
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: item})
}

// UpdateCartItem godoc
// @Summary      장바구니 아이템 수량 변경
// @Description  장바구니 아이템의 수량을 변경합니다
// @Tags         commerce-cart
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                       true  "장바구니 아이템 ID"
// @Param        request  body      domain.UpdateCartRequest  true  "수량 변경 요청"
// @Success      200  {object}  common.APIResponse{data=domain.CartItemResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/cart/{id} [put]
func (h *CartHandler) UpdateCartItem(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	cartID, err := h.getCartID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid cart item ID", err)
		return
	}

	var req domain.UpdateCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	item, err := h.service.UpdateCartItem(userID, cartID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCartItemNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Cart item not found", err)
		case errors.Is(err, service.ErrCartItemForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		case errors.Is(err, service.ErrProductNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Product not found", err)
		case errors.Is(err, service.ErrInsufficientStock):
			common.ErrorResponse(c, http.StatusConflict, "Insufficient stock", err)
		case errors.Is(err, service.ErrInvalidQuantity):
			common.ErrorResponse(c, http.StatusBadRequest, "Invalid quantity", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to update cart item", err)
		}
		return
	}

	common.SuccessResponse(c, item, nil)
}

// RemoveFromCart godoc
// @Summary      장바구니에서 아이템 삭제
// @Description  장바구니에서 아이템을 삭제합니다
// @Tags         commerce-cart
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "장바구니 아이템 ID"
// @Success      204  "No Content"
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/cart/{id} [delete]
func (h *CartHandler) RemoveFromCart(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	cartID, err := h.getCartID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid cart item ID", err)
		return
	}

	err = h.service.RemoveFromCart(userID, cartID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCartItemNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Cart item not found", err)
		case errors.Is(err, service.ErrCartItemForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to remove from cart", err)
		}
		return
	}

	c.Status(http.StatusNoContent)
}

// ClearCart godoc
// @Summary      장바구니 비우기
// @Description  장바구니의 모든 아이템을 삭제합니다
// @Tags         commerce-cart
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      204  "No Content"
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/cart [delete]
func (h *CartHandler) ClearCart(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	err = h.service.ClearCart(userID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to clear cart", err)
		return
	}

	c.Status(http.StatusNoContent)
}

// getUserID JWT에서 사용자 ID 추출
func (h *CartHandler) getUserID(c *gin.Context) (uint64, error) {
	userIDStr := middleware.GetUserID(c)
	if userIDStr == "" {
		return 0, errors.New("user not authenticated")
	}

	id, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return 0, errors.New("invalid user ID format")
	}
	return id, nil
}

// getCartID 경로에서 장바구니 아이템 ID 추출
func (h *CartHandler) getCartID(c *gin.Context) (uint64, error) {
	idStr := c.Param("id")
	return strconv.ParseUint(idStr, 10, 64)
}
