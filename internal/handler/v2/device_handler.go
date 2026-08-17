package v2

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"github.com/damoang/angple-backend/internal/middleware"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// DeviceHandler handles v2 device (FCM token) endpoints
type DeviceHandler struct {
	deviceRepo v2repo.DeviceRepository
}

// NewDeviceHandler creates a new DeviceHandler
func NewDeviceHandler(deviceRepo v2repo.DeviceRepository) *DeviceHandler {
	return &DeviceHandler{deviceRepo: deviceRepo}
}

// RegisterDevice handles POST /api/v2/devices
// Registers (or updates) the FCM token for the authenticated user.
func (h *DeviceHandler) RegisterDevice(c *gin.Context) {
	userID, err := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", err)
		return
	}

	var req struct {
		Token      string `json:"token" binding:"required"`
		Platform   string `json:"platform" binding:"required,oneof=ios android"`
		AppVersion string `json:"app_version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	device := &v2domain.V2Device{
		UserID:     userID,
		Token:      req.Token,
		Platform:   req.Platform,
		AppVersion: req.AppVersion,
	}
	if err := h.deviceRepo.Upsert(device); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "디바이스 등록 실패", err)
		return
	}
	common.V2Created(c, device)
}

// UnregisterDevice handles DELETE /api/v2/devices/:token
// Removes the FCM token (called on logout).
func (h *DeviceHandler) UnregisterDevice(c *gin.Context) {
	userID, err := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", err)
		return
	}

	token := c.Param("token")
	if token == "" {
		common.V2ErrorResponse(c, http.StatusBadRequest, "token이 필요합니다", nil)
		return
	}

	if err := h.deviceRepo.DeleteByUserAndToken(userID, token); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "디바이스 삭제 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "디바이스 삭제 완료"})
}

// ListDevices handles GET /api/v2/devices
// Returns devices owned by the authenticated user (debug/UX use).
func (h *DeviceHandler) ListDevices(c *gin.Context) {
	userID, err := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", err)
		return
	}

	devices, err := h.deviceRepo.ListByUser(userID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "디바이스 조회 실패", err)
		return
	}
	common.V2Success(c, devices)
}

// UnregisterDeviceByToken handles DELETE /api/v2/devices/by-token/:token (무인증).
// 세션이 만료되어 JWT 를 붙일 수 없는 기기가 푸시 등록을 해제하는 경로(#13444
// "로그아웃 상태인데 푸시 수신"). 토큰은 기기만 아는 비추측성 값이므로
// 보유 = 소유 증명으로 간주하고 토큰 단독 삭제를 허용한다.
func (h *DeviceHandler) UnregisterDeviceByToken(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		common.V2ErrorResponse(c, http.StatusBadRequest, "token이 필요합니다", nil)
		return
	}
	if err := h.deviceRepo.DeleteByToken(token); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "디바이스 삭제 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "디바이스 삭제 완료"})
}
