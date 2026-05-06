package handler

import (
	"net/http"
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

type SocialInviteHandler struct {
	service *service.SocialInviteService
}

func NewSocialInviteHandler(service *service.SocialInviteService) *SocialInviteHandler {
	return &SocialInviteHandler{service: service}
}

func (h *SocialInviteHandler) CreateInvite(c *gin.Context) {
	var req domain.CreateSocialInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 데이터가 올바르지 않습니다", err)
		return
	}

	result, err := h.service.CreateInvite(req.TargetMbID, middleware.GetUserID(c))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: result})
}

func (h *SocialInviteHandler) GetInviteInfo(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "토큰이 필요합니다", nil)
		return
	}

	info, err := h.service.GetInviteInfo(token, middleware.GetUserID(c))
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "찾을 수 없") || strings.Contains(errMsg, "만료") || strings.Contains(errMsg, "사용된") {
			common.ErrorResponse(c, http.StatusBadRequest, errMsg, nil)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "초대 정보를 가져올 수 없습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: info})
}

func (h *SocialInviteHandler) ConfirmInvite(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "토큰이 필요합니다", nil)
		return
	}

	currentUserMbID := middleware.GetUserID(c)
	if currentUserMbID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	if err := h.service.ConfirmInvite(token, currentUserMbID); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{"message": "소셜 연동이 완료되었습니다"}})
}
