package handler

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/damoang/angple-backend/pkg/ginutil"
	"github.com/gin-gonic/gin"
)

// PromotionHandler handles HTTP requests for promotions
type PromotionHandler struct {
	service service.PromotionService
}

// NewPromotionHandler creates a new PromotionHandler
func NewPromotionHandler(service service.PromotionService) *PromotionHandler {
	return &PromotionHandler{service: service}
}

// ListPromotionPosts godoc
// @Summary      직홍게 글 목록 조회
// @Description  활성 광고주의 직접홍보 게시글 목록을 조회합니다
// @Tags         promotion
// @Accept       json
// @Produce      json
// @Param        page   query     int  false  "페이지 번호 (기본값: 1)"  default(1)
// @Param        limit  query     int  false  "페이지당 항목 수 (기본값: 20)"  default(20)
// @Success      200  {object}  common.APIResponse{data=domain.PromotionListResponse}
// @Failure      500  {object}  common.APIResponse
// @Router       /promotion/posts [get]
func (h *PromotionHandler) ListPromotionPosts(c *gin.Context) {
	page := ginutil.QueryInt(c, "page", 1)
	limit := ginutil.QueryInt(c, "limit", 20)

	data, err := h.service.GetPromotionPosts(page, limit)
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch promotion posts", err)
		return
	}

	meta := &common.Meta{
		Page:  page,
		Limit: limit,
		Total: int64(data.Total),
	}

	common.SuccessResponse(c, data.Posts, meta)
}

// GetPromotionPost godoc
// @Summary      직홍게 글 상세 조회
// @Description  직접홍보 게시글 상세 정보를 조회합니다
// @Tags         promotion
// @Accept       json
// @Produce      json
// @Param        id  path  int  true  "게시글 ID"
// @Success      200  {object}  common.APIResponse{data=domain.PromotionPostResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /promotion/posts/{id} [get]
func (h *PromotionHandler) GetPromotionPost(c *gin.Context) {
	id, err := ginutil.ParamInt64(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid post ID", err)
		return
	}

	// Increment views
	_ = h.service.IncrementViews(id)

	data, err := h.service.GetPromotionPostByID(id)
	if err != nil {
		common.ErrorResponse(c, 404, "Promotion post not found", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// GetPromotionPostsForInsert godoc
// @Summary      사잇광고용 글 조회
// @Description  다른 게시판에 삽입할 직홍게 글을 조회합니다
// @Tags         promotion
// @Accept       json
// @Produce      json
// @Param        count  query     int  false  "조회할 글 개수 (기본값: 3)"  default(3)
// @Success      200  {object}  common.APIResponse{data=[]domain.PromotionPostResponse}
// @Failure      500  {object}  common.APIResponse
// @Router       /promotion/posts/insert [get]
func (h *PromotionHandler) GetPromotionPostsForInsert(c *gin.Context) {
	count := ginutil.QueryInt(c, "count", 3)

	data, err := h.service.GetPromotionPostsForInsert(count)
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch promotion posts", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// CreatePromotionPost godoc
// @Summary      직홍게 글 작성
// @Description  직접홍보 게시글을 작성합니다 (광고주만)
// @Tags         promotion
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.CreatePromotionPostRequest  true  "게시글 작성 요청"
// @Success      201  {object}  common.APIResponse{data=domain.PromotionPostResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /promotion/posts [post]
func (h *PromotionHandler) CreatePromotionPost(c *gin.Context) {
	// Get user ID from context (JWT or Damoang cookie)
	memberID := middleware.GetUserID(c)
	if memberID == "" {
		memberID = middleware.GetDamoangUserID(c)
	}
	if memberID == "" {
		common.ErrorResponse(c, 401, "Unauthorized", nil)
		return
	}

	var req domain.CreatePromotionPostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, 400, "Invalid request body", err)
		return
	}

	data, err := h.service.CreatePromotionPost(memberID, &req)
	if err != nil {
		if err.Error() == "only advertisers can create promotion posts" {
			common.ErrorResponse(c, 403, "Only advertisers can create promotion posts", err)
			return
		}
		common.ErrorResponse(c, 500, "Failed to create promotion post", err)
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: data})
}

// UpdatePromotionPost godoc
// @Summary      직홍게 글 수정
// @Description  직접홍보 게시글을 수정합니다 (작성자만)
// @Tags         promotion
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                                true  "게시글 ID"
// @Param        request  body      domain.UpdatePromotionPostRequest  true  "게시글 수정 요청"
// @Success      200  {object}  common.APIResponse{data=domain.PromotionPostResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /promotion/posts/{id} [put]
func (h *PromotionHandler) UpdatePromotionPost(c *gin.Context) {
	id, err := ginutil.ParamInt64(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid post ID", err)
		return
	}

	// Get user ID from context
	memberID := middleware.GetUserID(c)
	if memberID == "" {
		memberID = middleware.GetDamoangUserID(c)
	}
	if memberID == "" {
		common.ErrorResponse(c, 401, "Unauthorized", nil)
		return
	}

	var req domain.UpdatePromotionPostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, 400, "Invalid request body", err)
		return
	}

	data, err := h.service.UpdatePromotionPost(id, memberID, &req)
	if err != nil {
		if err.Error() == "you can only update your own posts" {
			common.ErrorResponse(c, 403, "You can only update your own posts", err)
			return
		}
		if err.Error() == "only advertisers can update promotion posts" {
			common.ErrorResponse(c, 403, "Only advertisers can update promotion posts", err)
			return
		}
		common.ErrorResponse(c, 500, "Failed to update promotion post", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// DeletePromotionPost godoc
// @Summary      직홍게 글 삭제
// @Description  직접홍보 게시글을 삭제합니다 (작성자만)
// @Tags         promotion
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "게시글 ID"
// @Success      200  {object}  common.APIResponse
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /promotion/posts/{id} [delete]
func (h *PromotionHandler) DeletePromotionPost(c *gin.Context) {
	id, err := ginutil.ParamInt64(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid post ID", err)
		return
	}

	// Get user ID from context
	memberID := middleware.GetUserID(c)
	if memberID == "" {
		memberID = middleware.GetDamoangUserID(c)
	}
	if memberID == "" {
		common.ErrorResponse(c, 401, "Unauthorized", nil)
		return
	}

	err = h.service.DeletePromotionPost(id, memberID)
	if err != nil {
		if err.Error() == "you can only delete your own posts" {
			common.ErrorResponse(c, 403, "You can only delete your own posts", err)
			return
		}
		if err.Error() == "only advertisers can delete promotion posts" {
			common.ErrorResponse(c, 403, "Only advertisers can delete promotion posts", err)
			return
		}
		common.ErrorResponse(c, 500, "Failed to delete promotion post", err)
		return
	}

	common.SuccessResponse(c, gin.H{"message": "Post deleted successfully"}, nil)
}

// ============= Admin Endpoints =============

// ListAdvertisers godoc
// @Summary      광고주 목록 조회 (관리자)
// @Description  모든 광고주 목록을 조회합니다
// @Tags         promotion
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  common.APIResponse{data=[]domain.AdvertiserResponse}
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /admin/promotion/advertisers [get]
func (h *PromotionHandler) ListAdvertisers(c *gin.Context) {
	data, err := h.service.GetAllAdvertisers()
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch advertisers", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// CreateAdvertiser godoc
// @Summary      광고주 추가 (관리자)
// @Description  새 광고주를 추가합니다
// @Tags         promotion
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.CreateAdvertiserRequest  true  "광고주 추가 요청"
// @Success      201  {object}  common.APIResponse{data=domain.AdvertiserResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /admin/promotion/advertisers [post]
func (h *PromotionHandler) CreateAdvertiser(c *gin.Context) {
	var req domain.CreateAdvertiserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, 400, "Invalid request body", err)
		return
	}

	data, err := h.service.CreateAdvertiser(&req)
	if err != nil {
		if err.Error() == "member is already an advertiser" {
			common.ErrorResponse(c, 409, "Member is already an advertiser", err)
			return
		}
		common.ErrorResponse(c, 500, "Failed to create advertiser", err)
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: data})
}

// UpdateAdvertiser godoc
// @Summary      광고주 수정 (관리자)
// @Description  광고주 정보를 수정합니다
// @Tags         promotion
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                             true  "광고주 ID"
// @Param        request  body      domain.UpdateAdvertiserRequest  true  "광고주 수정 요청"
// @Success      200  {object}  common.APIResponse{data=domain.AdvertiserResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /admin/promotion/advertisers/{id} [put]
func (h *PromotionHandler) UpdateAdvertiser(c *gin.Context) {
	id, err := ginutil.ParamInt64(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid advertiser ID", err)
		return
	}

	var req domain.UpdateAdvertiserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, 400, "Invalid request body", err)
		return
	}

	data, err := h.service.UpdateAdvertiser(id, &req)
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to update advertiser", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// DeleteAdvertiser godoc
// @Summary      광고주 삭제 (관리자)
// @Description  광고주를 삭제합니다
// @Tags         promotion
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "광고주 ID"
// @Success      200  {object}  common.APIResponse
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /admin/promotion/advertisers/{id} [delete]
func (h *PromotionHandler) DeleteAdvertiser(c *gin.Context) {
	id, err := ginutil.ParamInt64(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid advertiser ID", err)
		return
	}

	if err := h.service.DeleteAdvertiser(id); err != nil {
		common.ErrorResponse(c, 500, "Failed to delete advertiser", err)
		return
	}

	common.SuccessResponse(c, gin.H{"message": "Advertiser deleted successfully"}, nil)
}
