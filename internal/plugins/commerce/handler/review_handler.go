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

// ReviewHandler 리뷰 HTTP 핸들러
type ReviewHandler struct {
	service service.ReviewService
}

// NewReviewHandler 생성자
func NewReviewHandler(svc service.ReviewService) *ReviewHandler {
	return &ReviewHandler{service: svc}
}

// CreateReview godoc
// @Summary      리뷰 작성
// @Description  구매 확정된 상품에 대해 리뷰를 작성합니다
// @Tags         commerce-reviews
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.CreateReviewRequest  true  "리뷰 작성 요청"
// @Success      201  {object}  common.APIResponse{data=domain.ReviewResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Router       /plugins/commerce/reviews [post]
func (h *ReviewHandler) CreateReview(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.CreateReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	review, err := h.service.CreateReview(userID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderItemNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Order item not found", err)
		case errors.Is(err, service.ErrOrderItemNotComplete):
			common.ErrorResponse(c, http.StatusBadRequest, "Order is not completed yet", err)
		case errors.Is(err, service.ErrReviewForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "You cannot review this order", err)
		case errors.Is(err, service.ErrReviewAlreadyExists):
			common.ErrorResponse(c, http.StatusConflict, "Review already exists", err)
		case errors.Is(err, service.ErrInvalidRating):
			common.ErrorResponse(c, http.StatusBadRequest, "Rating must be between 1 and 5", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to create review", err)
		}
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: review})
}

// UpdateReview godoc
// @Summary      리뷰 수정
// @Description  자신이 작성한 리뷰를 수정합니다
// @Tags         commerce-reviews
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                         true  "리뷰 ID"
// @Param        request  body      domain.UpdateReviewRequest  true  "리뷰 수정 요청"
// @Success      200  {object}  common.APIResponse{data=domain.ReviewResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Router       /plugins/commerce/reviews/{id} [put]
func (h *ReviewHandler) UpdateReview(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	reviewID, err := h.getReviewID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid review ID", err)
		return
	}

	var req domain.UpdateReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	review, err := h.service.UpdateReview(userID, reviewID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrReviewNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Review not found", err)
		case errors.Is(err, service.ErrReviewForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		case errors.Is(err, service.ErrInvalidRating):
			common.ErrorResponse(c, http.StatusBadRequest, "Rating must be between 1 and 5", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to update review", err)
		}
		return
	}

	common.SuccessResponse(c, review, nil)
}

// DeleteReview godoc
// @Summary      리뷰 삭제
// @Description  자신이 작성한 리뷰를 삭제합니다
// @Tags         commerce-reviews
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "리뷰 ID"
// @Success      204  "No Content"
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Router       /plugins/commerce/reviews/{id} [delete]
func (h *ReviewHandler) DeleteReview(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	reviewID, err := h.getReviewID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid review ID", err)
		return
	}

	err = h.service.DeleteReview(userID, reviewID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrReviewNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Review not found", err)
		case errors.Is(err, service.ErrReviewForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete review", err)
		}
		return
	}

	c.Status(http.StatusNoContent)
}

// GetReview godoc
// @Summary      리뷰 조회
// @Description  리뷰 상세 정보를 조회합니다
// @Tags         commerce-reviews
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "리뷰 ID"
// @Success      200  {object}  common.APIResponse{data=domain.ReviewResponse}
// @Failure      404  {object}  common.APIResponse
// @Router       /plugins/commerce/reviews/{id} [get]
func (h *ReviewHandler) GetReview(c *gin.Context) {
	reviewID, err := h.getReviewID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid review ID", err)
		return
	}

	review, err := h.service.GetReview(reviewID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrReviewNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Review not found", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to get review", err)
		}
		return
	}

	common.SuccessResponse(c, review, nil)
}

// ListProductReviews godoc
// @Summary      상품 리뷰 목록 조회
// @Description  특정 상품의 리뷰 목록을 조회합니다
// @Tags         commerce-reviews
// @Accept       json
// @Produce      json
// @Param        product_id  path      int     true   "상품 ID"
// @Param        page        query     int     false  "페이지 번호"
// @Param        limit       query     int     false  "페이지당 항목 수"
// @Param        rating      query     int     false  "평점 필터 (1-5)"
// @Param        sort_by     query     string  false  "정렬 기준 (created_at, rating, helpful_count)"
// @Param        sort_order  query     string  false  "정렬 방향 (asc, desc)"
// @Success      200  {object}  common.APIResponse{data=[]domain.ReviewResponse}
// @Router       /plugins/commerce/products/{product_id}/reviews [get]
func (h *ReviewHandler) ListProductReviews(c *gin.Context) {
	productID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid product ID", err)
		return
	}

	var req domain.ReviewListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid query parameters", err)
		return
	}

	reviews, total, err := h.service.ListProductReviews(productID, &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to list reviews", err)
		return
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 {
		limit = 20
	}

	meta := &common.Meta{
		Page:  page,
		Limit: limit,
		Total: total,
	}

	common.SuccessResponse(c, reviews, meta)
}

// GetProductReviewSummary godoc
// @Summary      상품 리뷰 요약 조회
// @Description  특정 상품의 리뷰 요약 정보를 조회합니다 (총 리뷰 수, 평균 평점, 평점별 분포)
// @Tags         commerce-reviews
// @Accept       json
// @Produce      json
// @Param        product_id  path      int  true  "상품 ID"
// @Success      200  {object}  common.APIResponse{data=domain.ReviewSummary}
// @Router       /plugins/commerce/products/{product_id}/reviews/summary [get]
func (h *ReviewHandler) GetProductReviewSummary(c *gin.Context) {
	productID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid product ID", err)
		return
	}

	summary, err := h.service.GetProductReviewSummary(productID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to get review summary", err)
		return
	}

	common.SuccessResponse(c, summary, nil)
}

// ListMyReviews godoc
// @Summary      내 리뷰 목록 조회
// @Description  로그인한 사용자가 작성한 리뷰 목록을 조회합니다
// @Tags         commerce-reviews
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page       query     int     false  "페이지 번호"
// @Param        limit      query     int     false  "페이지당 항목 수"
// @Param        sort_by    query     string  false  "정렬 기준"
// @Param        sort_order query     string  false  "정렬 방향"
// @Success      200  {object}  common.APIResponse{data=[]domain.ReviewResponse}
// @Failure      401  {object}  common.APIResponse
// @Router       /plugins/commerce/reviews/my [get]
func (h *ReviewHandler) ListMyReviews(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.ReviewListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid query parameters", err)
		return
	}

	reviews, total, err := h.service.ListUserReviews(userID, &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to list reviews", err)
		return
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 {
		limit = 20
	}

	meta := &common.Meta{
		Page:  page,
		Limit: limit,
		Total: total,
	}

	common.SuccessResponse(c, reviews, meta)
}

// ListSellerReviews godoc
// @Summary      판매자 리뷰 목록 조회
// @Description  판매자의 모든 상품에 대한 리뷰 목록을 조회합니다
// @Tags         commerce-reviews
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page       query     int     false  "페이지 번호"
// @Param        limit      query     int     false  "페이지당 항목 수"
// @Success      200  {object}  common.APIResponse{data=[]domain.ReviewResponse}
// @Failure      401  {object}  common.APIResponse
// @Router       /plugins/commerce/seller/reviews [get]
func (h *ReviewHandler) ListSellerReviews(c *gin.Context) {
	sellerID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.ReviewListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid query parameters", err)
		return
	}

	reviews, total, err := h.service.ListSellerReviews(sellerID, &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to list reviews", err)
		return
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 {
		limit = 20
	}

	meta := &common.Meta{
		Page:  page,
		Limit: limit,
		Total: total,
	}

	common.SuccessResponse(c, reviews, meta)
}

// ReplyToReview godoc
// @Summary      리뷰에 답글 작성
// @Description  판매자가 자신의 상품 리뷰에 답글을 작성합니다
// @Tags         commerce-reviews
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                         true  "리뷰 ID"
// @Param        request  body      domain.ReplyReviewRequest   true  "답글 요청"
// @Success      200  {object}  common.APIResponse{data=domain.ReviewResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Router       /plugins/commerce/reviews/{id}/reply [post]
func (h *ReviewHandler) ReplyToReview(c *gin.Context) {
	sellerID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	reviewID, err := h.getReviewID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid review ID", err)
		return
	}

	var req domain.ReplyReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	review, err := h.service.ReplyToReview(sellerID, reviewID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrReviewNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Review not found", err)
		case errors.Is(err, service.ErrSellerReplyForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "You are not the seller of this product", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to reply to review", err)
		}
		return
	}

	common.SuccessResponse(c, review, nil)
}

// ToggleHelpful godoc
// @Summary      리뷰 도움됨 토글
// @Description  리뷰에 '도움됨'을 표시하거나 취소합니다
// @Tags         commerce-reviews
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "리뷰 ID"
// @Success      200  {object}  common.APIResponse{data=map[string]interface{}}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Router       /plugins/commerce/reviews/{id}/helpful [post]
func (h *ReviewHandler) ToggleHelpful(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	reviewID, err := h.getReviewID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid review ID", err)
		return
	}

	isHelpful, err := h.service.ToggleHelpful(userID, reviewID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrReviewNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Review not found", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to toggle helpful", err)
		}
		return
	}

	common.SuccessResponse(c, map[string]interface{}{
		"is_helpful": isHelpful,
		"message":    ternary(isHelpful, "도움됨 표시가 추가되었습니다", "도움됨 표시가 취소되었습니다"),
	}, nil)
}

// getUserID JWT에서 사용자 ID 추출
func (h *ReviewHandler) getUserID(c *gin.Context) (uint64, error) {
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

// getReviewID 경로에서 리뷰 ID 추출
func (h *ReviewHandler) getReviewID(c *gin.Context) (uint64, error) {
	idStr := c.Param("id")
	return strconv.ParseUint(idStr, 10, 64)
}

// ternary 삼항 연산자 헬퍼
func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
