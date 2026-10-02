package handler

import (
	"errors"
	"net/http"

	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// MemberHandler handles member profile image endpoints
type MemberHandler struct {
	memberService *service.MemberService
}

// NewMemberHandler creates a new MemberHandler
func NewMemberHandler(memberService *service.MemberService) *MemberHandler {
	return &MemberHandler{memberService: memberService}
}

// UploadImage handles POST /api/v2/members/me/image
func (h *MemberHandler) UploadImage(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "인증이 필요합니다"})
		return
	}

	file, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "이미지 파일이 필요합니다"})
		return
	}

	url, err := h.memberService.UpdateMemberImage(c.Request.Context(), mbID, file)
	if err != nil {
		var invalid *service.MemberImageValidationError
		switch {
		case errors.As(err, &invalid):
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": invalid.Msg})
		case errors.Is(err, service.ErrMemberImageProcessingTimeout):
			// 원본은 올라갔지만 변환 결과를 기다리다 시간이 지났다. DB 는 그대로다.
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": service.ErrMemberImageProcessingTimeout.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "프로필 이미지 저장에 실패했습니다"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"url": url}})
}

// DeleteImage handles DELETE /api/v2/members/me/image
func (h *MemberHandler) DeleteImage(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "인증이 필요합니다"})
		return
	}

	if err := h.memberService.DeleteMemberImage(c.Request.Context(), mbID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "프로필 이미지가 삭제되었습니다"})
}
