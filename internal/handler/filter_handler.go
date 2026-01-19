package handler

import (
	"net/http"
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/gin-gonic/gin"
)

// FilterHandler handles content filtering requests
type FilterHandler struct {
	// Filter words list (loaded from config or DB)
	filterWords []string
}

// NewFilterHandler creates a new FilterHandler
func NewFilterHandler(filterWords []string) *FilterHandler {
	if filterWords == nil {
		// Default filter words (should be loaded from DB in production)
		filterWords = []string{}
	}
	return &FilterHandler{filterWords: filterWords}
}

// CheckFilterRequest request for checking content filter
type CheckFilterRequest struct {
	Subject string `json:"subject"`
	Content string `json:"content"`
}

// CheckFilterResponse response for filter check
type CheckFilterResponse struct {
	Subject string `json:"subject"`
	Content string `json:"content"`
}

// Check handles POST /api/v2/filter/check
// @Summary 금지어 필터 검사
// @Description 제목과 내용에서 금지어를 검사합니다
// @Tags filter
// @Accept json
// @Produce json
// @Param request body CheckFilterRequest true "검사할 내용"
// @Success 200 {object} common.APIResponse{data=CheckFilterResponse}
// @Failure 400 {object} common.APIResponse
// @Router /filter/check [post]
func (h *FilterHandler) Check(c *gin.Context) {
	var req CheckFilterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.", err)
		return
	}

	// Strip HTML tags for checking
	subject := stripTags(req.Subject)
	content := stripTags(req.Content)

	response := CheckFilterResponse{
		Subject: "",
		Content: "",
	}

	// Check each filter word
	for _, word := range h.filterWords {
		if word == "" {
			continue
		}

		// Case-insensitive search in subject
		if strings.Contains(strings.ToLower(subject), strings.ToLower(word)) {
			response.Subject = word
			break
		}

		// Case-insensitive search in content
		if strings.Contains(strings.ToLower(content), strings.ToLower(word)) {
			response.Content = word
			break
		}
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: response})
}

// stripTags removes HTML tags from a string
func stripTags(s string) string {
	// Simple HTML tag removal (for basic filtering)
	result := strings.Builder{}
	inTag := false

	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			result.WriteRune(r)
		}
	}

	return result.String()
}
