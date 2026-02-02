package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// MediaHandler handles media upload/download endpoints
type MediaHandler struct {
	mediaService *service.MediaService
}

// NewMediaHandler creates a new MediaHandler
func NewMediaHandler(mediaService *service.MediaService) *MediaHandler {
	return &MediaHandler{mediaService: mediaService}
}

// UploadImage handles editor image upload with optional resize
// POST /api/v2/media/images
func (h *MediaHandler) UploadImage(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "File is required", nil)
		return
	}

	maxWidth, _ := strconv.Atoi(c.DefaultPostForm("max_width", "1920"))

	result, err := h.mediaService.UploadImage(c.Request.Context(), file, maxWidth)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// UploadAttachment handles general file attachment upload
// POST /api/v2/media/attachments
func (h *MediaHandler) UploadAttachment(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "File is required", nil)
		return
	}

	result, err := h.mediaService.UploadAttachment(c.Request.Context(), file)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// UploadVideo handles video file upload
// POST /api/v2/media/videos
func (h *MediaHandler) UploadVideo(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "File is required", nil)
		return
	}

	result, err := h.mediaService.UploadVideo(c.Request.Context(), file)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// DeleteFile removes a file from storage
// DELETE /api/v2/media/files
func (h *MediaHandler) DeleteFile(c *gin.Context) {
	var req struct {
		Key string `json:"key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "key is required", nil)
		return
	}

	if err := h.mediaService.DeleteFile(c.Request.Context(), req.Key); err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Delete failed: "+err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "file deleted"})
}
