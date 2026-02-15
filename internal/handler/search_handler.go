package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// SearchHandler handles Elasticsearch-based search endpoints
type SearchHandler struct {
	searchService *service.SearchService
}

// NewSearchHandler creates a new SearchHandler
func NewSearchHandler(searchService *service.SearchService) *SearchHandler {
	return &SearchHandler{searchService: searchService}
}

// Search performs unified search across posts and comments
// GET /api/v2/search?q=keyword&board_id=free&type=posts&page=1&per_page=20
func (h *SearchHandler) Search(c *gin.Context) {
	keyword := c.Query("q")
	if keyword == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "Search query is required", nil)
		return
	}

	boardID := c.Query("board_id")
	searchType := c.DefaultQuery("type", "all") // all, posts, comments
	page := 1
	if val, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil {
		page = val
	}
	perPage := 20
	if val, err := strconv.Atoi(c.DefaultQuery("per_page", "20")); err == nil {
		perPage = val
	}

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	result, err := h.searchService.UnifiedSearch(c.Request.Context(), keyword, boardID, searchType, page, perPage)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Search failed: "+err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
		"meta": gin.H{
			"query":    keyword,
			"board_id": boardID,
			"type":     searchType,
			"page":     page,
			"per_page": perPage,
		},
	})
}

// Autocomplete returns search suggestions
// GET /api/v2/search/autocomplete?q=prefix
func (h *SearchHandler) Autocomplete(c *gin.Context) {
	prefix := c.Query("q")
	if prefix == "" {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []string{}})
		return
	}

	size := 10
	if val, err := strconv.Atoi(c.DefaultQuery("size", "10")); err == nil {
		size = val
	}
	suggestions, err := h.searchService.Autocomplete(c.Request.Context(), prefix, size)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Autocomplete failed", nil)
		return
	}

	if suggestions == nil {
		suggestions = []string{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": suggestions})
}

// BulkIndex triggers bulk indexing for a board (admin)
// POST /api/v2/admin/search/index
func (h *SearchHandler) BulkIndex(c *gin.Context) {
	var req struct {
		BoardID string `json:"board_id" binding:"required"`
		Limit   int    `json:"limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "board_id is required", nil)
		return
	}
	if req.Limit <= 0 {
		req.Limit = 1000
	}

	count, err := h.searchService.BulkIndexPosts(c.Request.Context(), req.BoardID, req.Limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Bulk indexing failed: "+err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"board_id":      req.BoardID,
			"indexed_count": count,
		},
	})
}

// IndexPost indexes a single post (called after post create/update)
// POST /api/v2/admin/search/index-post
func (h *SearchHandler) IndexPost(c *gin.Context) {
	var doc service.PostDocument
	if err := c.ShouldBindJSON(&doc); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request", nil)
		return
	}

	if err := h.searchService.IndexPost(c.Request.Context(), &doc); err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Indexing failed: "+err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "post indexed"})
}

// DeletePostIndex removes a post from the index
// DELETE /api/v2/admin/search/index/:board_id/:post_id
func (h *SearchHandler) DeletePostIndex(c *gin.Context) {
	boardID := c.Param("board_id")
	postID, err := strconv.Atoi(c.Param("post_id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid post_id", nil)
		return
	}

	if err := h.searchService.DeletePost(c.Request.Context(), boardID, postID); err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Delete failed: "+err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "post removed from index"})
}
