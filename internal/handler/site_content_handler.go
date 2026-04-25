package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// SiteContentHandler exposes the Angple Sites builder content endpoints
// (issue #1288 PoC).
type SiteContentHandler struct {
	service *service.SiteContentService
}

// NewSiteContentHandler constructs a SiteContentHandler.
func NewSiteContentHandler(svc *service.SiteContentService) *SiteContentHandler {
	return &SiteContentHandler{service: svc}
}

// parseSiteID extracts and validates the :id path parameter.
func (h *SiteContentHandler) parseSiteID(c *gin.Context) (int64, bool) {
	raw := c.Param("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid site_id", err)
		return 0, false
	}
	return id, true
}

// GetPublic returns published content for SSR.
//
// GET /api/v1/sites/:id/content/:key
func (h *SiteContentHandler) GetPublic(c *gin.Context) {
	siteID, ok := h.parseSiteID(c)
	if !ok {
		return
	}
	key := c.Param("key")
	if key == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "content_key is required", nil)
		return
	}
	row, err := h.service.Get(c.Request.Context(), siteID, key)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Content not found", err)
			return
		}
		if errors.Is(err, common.ErrBadRequest) {
			common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to get content", err)
		return
	}
	common.SuccessResponse(c, gin.H{"content": row.ToResponse()}, nil)
}

// GetAdmin returns content for admin authoring. Requires admin middleware upstream.
//
// GET /api/v1/admin/sites/:id/content/:key
func (h *SiteContentHandler) GetAdmin(c *gin.Context) {
	h.GetPublic(c) // Same logic; auth is enforced by middleware on the route group.
}

// Upsert creates or updates content. Requires admin middleware upstream.
//
// PUT /api/v1/admin/sites/:id/content/:key
func (h *SiteContentHandler) Upsert(c *gin.Context) {
	siteID, ok := h.parseSiteID(c)
	if !ok {
		return
	}
	key := c.Param("key")
	if key == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "content_key is required", nil)
		return
	}
	var req domain.AngpleSiteContentUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}
	row, err := h.service.Upsert(c.Request.Context(), siteID, key, &req)
	if err != nil {
		if errors.Is(err, common.ErrBadRequest) {
			common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to save content", err)
		return
	}
	common.SuccessResponse(c, gin.H{"content": row.ToResponse()}, nil)
}

// Delete removes content. Requires admin middleware upstream.
//
// DELETE /api/v1/admin/sites/:id/content/:key
func (h *SiteContentHandler) Delete(c *gin.Context) {
	siteID, ok := h.parseSiteID(c)
	if !ok {
		return
	}
	key := c.Param("key")
	if key == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "content_key is required", nil)
		return
	}
	if err := h.service.Delete(c.Request.Context(), siteID, key); err != nil {
		if errors.Is(err, common.ErrBadRequest) {
			common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete content", err)
		return
	}
	common.SuccessResponse(c, gin.H{"message": "deleted"}, nil)
}
