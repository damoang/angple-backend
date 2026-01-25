package handler

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// MemberHandler handles member validation requests
type MemberHandler struct {
	validationService service.MemberValidationService
}

// NewMemberHandler creates a new MemberHandler
func NewMemberHandler(validationService service.MemberValidationService) *MemberHandler {
	return &MemberHandler{
		validationService: validationService,
	}
}

// CheckUserIDRequest request for checking user ID
type CheckUserIDRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

// CheckNicknameRequest request for checking nickname
type CheckNicknameRequest struct {
	Nickname      string `json:"nickname" binding:"required"`
	ExcludeUserID string `json:"exclude_user_id,omitempty"`
}

// CheckEmailRequest request for checking email
type CheckEmailRequest struct {
	Email         string `json:"email" binding:"required"`
	ExcludeUserID string `json:"exclude_user_id,omitempty"`
}

// CheckPhoneRequest request for checking phone
type CheckPhoneRequest struct {
	Phone         string `json:"phone" binding:"required"`
	ExcludeUserID string `json:"exclude_user_id,omitempty"`
}

// CheckUserID handles POST /api/v2/members/check-id
// @Summary 회원 ID 중복 확인
// @Description 회원가입 시 사용할 아이디의 유효성 및 중복을 확인합니다
// @Tags members
// @Accept json
// @Produce json
// @Param request body CheckUserIDRequest true "확인할 회원 ID"
// @Success 200 {object} common.APIResponse{data=service.ValidationResult}
// @Failure 400 {object} common.APIResponse
// @Router /members/check-id [post]
func (h *MemberHandler) CheckUserID(c *gin.Context) {
	var req CheckUserIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.", err)
		return
	}

	result := h.validationService.ValidateUserID(req.UserID)

	if result.Valid {
		c.JSON(http.StatusOK, common.APIResponse{Data: result})
	} else {
		c.JSON(http.StatusOK, common.APIResponse{Data: result})
	}
}

// CheckNickname handles POST /api/v2/members/check-nickname
// @Summary 닉네임 중복 확인
// @Description 회원가입/수정 시 사용할 닉네임의 유효성 및 중복을 확인합니다
// @Tags members
// @Accept json
// @Produce json
// @Param request body CheckNicknameRequest true "확인할 닉네임"
// @Success 200 {object} common.APIResponse{data=service.ValidationResult}
// @Failure 400 {object} common.APIResponse
// @Router /members/check-nickname [post]
func (h *MemberHandler) CheckNickname(c *gin.Context) {
	var req CheckNicknameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.", err)
		return
	}

	result := h.validationService.ValidateNickname(req.Nickname, req.ExcludeUserID)
	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// CheckEmail handles POST /api/v2/members/check-email
// @Summary 이메일 중복 확인
// @Description 회원가입/수정 시 사용할 이메일의 유효성 및 중복을 확인합니다
// @Tags members
// @Accept json
// @Produce json
// @Param request body CheckEmailRequest true "확인할 이메일"
// @Success 200 {object} common.APIResponse{data=service.ValidationResult}
// @Failure 400 {object} common.APIResponse
// @Router /members/check-email [post]
func (h *MemberHandler) CheckEmail(c *gin.Context) {
	var req CheckEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.", err)
		return
	}

	result := h.validationService.ValidateEmail(req.Email, req.ExcludeUserID)
	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// CheckPhone handles POST /api/v2/members/check-phone
// @Summary 휴대폰번호 중복 확인
// @Description 회원가입/수정 시 사용할 휴대폰번호의 유효성 및 중복을 확인합니다
// @Tags members
// @Accept json
// @Produce json
// @Param request body CheckPhoneRequest true "확인할 휴대폰번호"
// @Success 200 {object} common.APIResponse{data=service.ValidationResult}
// @Failure 400 {object} common.APIResponse
// @Router /members/check-phone [post]
func (h *MemberHandler) CheckPhone(c *gin.Context) {
	var req CheckPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.", err)
		return
	}

	result := h.validationService.ValidatePhone(req.Phone, req.ExcludeUserID)
	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// GetNickname handles GET /api/v2/members/:id/nickname
// @Summary 회원 닉네임 조회
// @Description 회원 ID로 닉네임을 조회합니다
// @Tags members
// @Produce json
// @Param id path string true "회원 ID"
// @Success 200 {object} common.APIResponse
// @Failure 404 {object} common.APIResponse
// @Router /members/{id}/nickname [get]
func (h *MemberHandler) GetNickname(c *gin.Context) {
	// 이 기능은 MemberRepository를 직접 사용해야 하므로 별도 서비스 메서드 필요
	// 현재는 ValidationService만 있으므로 추후 확장
	memberID := c.Param("id")

	c.JSON(http.StatusOK, common.APIResponse{
		Data: gin.H{
			"user_id":  memberID,
			"nickname": "", // TODO: 실제 조회 구현
		},
	})
}
