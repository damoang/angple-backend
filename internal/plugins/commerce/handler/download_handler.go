package handler

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/plugins/commerce/service"
	"github.com/gin-gonic/gin"
)

// DownloadHandler 다운로드 HTTP 핸들러
type DownloadHandler struct {
	service    service.DownloadService
	baseURL    string
	secretKey  string
	storagePath string
}

// DownloadHandlerConfig 핸들러 설정
type DownloadHandlerConfig struct {
	BaseURL     string
	SecretKey   string
	StoragePath string
}

// NewDownloadHandler 생성자
func NewDownloadHandler(svc service.DownloadService, config *DownloadHandlerConfig) *DownloadHandler {
	return &DownloadHandler{
		service:     svc,
		baseURL:     config.BaseURL,
		secretKey:   config.SecretKey,
		storagePath: config.StoragePath,
	}
}

// ListDownloads godoc
// @Summary      다운로드 목록 조회
// @Description  주문 아이템의 다운로드 가능한 파일 목록을 조회합니다
// @Tags         commerce-downloads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        order_item_id  path      int  true  "주문 아이템 ID"
// @Success      200  {object}  common.APIResponse{data=[]domain.DownloadResponse}
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Router       /plugins/commerce/order-items/{id}/downloads [get]
func (h *DownloadHandler) ListDownloads(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	orderItemID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid order item ID", err)
		return
	}

	downloads, err := h.service.ListDownloads(userID, orderItemID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDownloadNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Downloads not found", err)
		case errors.Is(err, service.ErrDownloadForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch downloads", err)
		}
		return
	}

	common.SuccessResponse(c, downloads, nil)
}

// GetDownloadURL godoc
// @Summary      다운로드 URL 생성
// @Description  파일 다운로드를 위한 서명된 URL을 생성합니다
// @Tags         commerce-downloads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        order_item_id  path      int  true  "주문 아이템 ID"
// @Param        file_id        path      int  true  "파일 ID"
// @Success      200  {object}  common.APIResponse{data=domain.DownloadURLResponse}
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      410  {object}  common.APIResponse
// @Router       /plugins/commerce/downloads/{order_item_id}/{file_id} [get]
func (h *DownloadHandler) GetDownloadURL(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	orderItemID, err := strconv.ParseUint(c.Param("order_item_id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid order item ID", err)
		return
	}

	fileID, err := strconv.ParseUint(c.Param("file_id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid file ID", err)
		return
	}

	downloadURL, err := h.service.GenerateDownloadURL(userID, orderItemID, fileID, h.baseURL, h.secretKey)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDownloadNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Download not found", err)
		case errors.Is(err, service.ErrDownloadForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		case errors.Is(err, service.ErrDownloadExpired):
			common.ErrorResponse(c, http.StatusGone, "Download expired", err)
		case errors.Is(err, service.ErrDownloadLimitReached):
			common.ErrorResponse(c, http.StatusGone, "Download limit reached", err)
		case errors.Is(err, service.ErrFileNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "File not found", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate download URL", err)
		}
		return
	}

	common.SuccessResponse(c, downloadURL, nil)
}

// Download godoc
// @Summary      파일 다운로드
// @Description  서명된 토큰으로 파일을 다운로드합니다
// @Tags         commerce-downloads
// @Produce      octet-stream
// @Security     BearerAuth
// @Param        token  path      string  true  "다운로드 토큰"
// @Param        sig    query     string  true  "서명"
// @Param        exp    query     int     true  "만료 시간 (Unix timestamp)"
// @Success      200  {file}    binary
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      410  {object}  common.APIResponse
// @Router       /plugins/commerce/downloads/by-token/{token} [get]
func (h *DownloadHandler) Download(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	token := c.Param("token")
	if token == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "Token is required", nil)
		return
	}

	signature := c.Query("sig")
	expStr := c.Query("exp")

	// 만료 시간 파싱
	expUnix, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid expiration time", err)
		return
	}
	expiresAt := time.Unix(expUnix, 0)

	// 서명 검증
	if !h.service.VerifySignature(token, signature, h.secretKey, expiresAt) {
		common.ErrorResponse(c, http.StatusForbidden, "Invalid or expired signature", service.ErrInvalidSignature)
		return
	}

	// 다운로드 처리
	file, err := h.service.ProcessDownload(token, signature, userID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDownloadNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Download not found", err)
		case errors.Is(err, service.ErrDownloadForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		case errors.Is(err, service.ErrDownloadExpired):
			common.ErrorResponse(c, http.StatusGone, "Download expired", err)
		case errors.Is(err, service.ErrDownloadLimitReached):
			common.ErrorResponse(c, http.StatusGone, "Download limit reached", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to process download", err)
		}
		return
	}

	// 파일 경로 구성
	filePath := filepath.Join(h.storagePath, file.FilePath)

	// 파일 존재 확인
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		common.ErrorResponse(c, http.StatusNotFound, "File not found on server", err)
		return
	}

	// 파일 열기
	f, err := os.Open(filePath)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to open file", err)
		return
	}
	defer f.Close()

	// 파일 정보
	fileInfo, err := f.Stat()
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to get file info", err)
		return
	}

	// 다운로드 헤더 설정
	fileName := file.DisplayName
	if fileName == "" {
		fileName = file.FileName
	}

	c.Header("Content-Disposition", "attachment; filename=\""+fileName+"\"")
	c.Header("Content-Type", file.FileType)
	c.Header("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
	c.Header("Cache-Control", "private, no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	// 파일 스트리밍
	c.Status(http.StatusOK)
	io.Copy(c.Writer, f)
}

// ListUserDownloads godoc
// @Summary      내 다운로드 목록
// @Description  사용자의 전체 다운로드 가능한 파일 목록을 조회합니다
// @Tags         commerce-downloads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  common.APIResponse{data=[]domain.DownloadResponse}
// @Failure      401  {object}  common.APIResponse
// @Router       /plugins/commerce/downloads [get]
func (h *DownloadHandler) ListUserDownloads(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	downloads, err := h.service.ListUserDownloads(userID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch downloads", err)
		return
	}

	common.SuccessResponse(c, downloads, nil)
}

// getUserID JWT에서 사용자 ID 추출
func (h *DownloadHandler) getUserID(c *gin.Context) (uint64, error) {
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
