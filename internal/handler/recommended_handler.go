package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// RecommendedHandler handles recommended posts API
type RecommendedHandler struct {
	basePath string
}

// NewRecommendedHandler creates a new RecommendedHandler
func NewRecommendedHandler(basePath string) *RecommendedHandler {
	return &RecommendedHandler{
		basePath: basePath,
	}
}

// validPeriods defines allowed period values
var validPeriods = map[string]bool{
	"1hour":         true,
	"3hours":        true,
	"6hours":        true,
	"12hours":       true,
	"24hours":       true,
	"48hours":       true,
	"index-widgets": true,
}

// GetRecommended godoc
// @Summary      추천 게시글 조회
// @Description  특정 기간 동안의 추천 게시글 목록을 조회합니다. 파일이 없는 경우 빈 배열을 반환합니다.
// @Tags         recommended
// @Accept       json
// @Produce      json
// @Param        period  path      string  true  "기간 (1hour, 3hours, 6hours, 12hours, 24hours, 48hours, index-widgets)"
// @Success      200     {array}   interface{}  "추천 게시글 목록"
// @Failure      400     {object}  common.APIResponse
// @Failure      500     {object}  common.APIResponse
// @Router       /recommended/{period} [get]
func (h *RecommendedHandler) GetRecommended(c *gin.Context) {
	period := c.Param("period")

	// Validate period to prevent path traversal
	if !validPeriods[period] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid period. Valid values: 1hour, 3hours, 6hours, 12hours, 24hours, 48hours, index-widgets",
		})
		return
	}

	// Construct file path - 최신 데이터 파일 사용 (AI 분석 없어도 됨)
	var filename string
	if period == "index-widgets" {
		filename = "index-widgets.json"
	} else {
		filename = period + ".json" // 1hour.json, 3hours.json 등 (최신 데이터)
	}
	filePath := filepath.Join(h.basePath, filename)
	filePath = filepath.Clean(filePath) // Sanitize path

	// Check if file exists
	fileInfo, err := os.Stat(filePath) // #nosec G304 - path is validated via whitelist
	if err != nil {
		if os.IsNotExist(err) {
			// 파일이 없으면 빈 배열 반환 (개발 환경 대응)
			c.Header("Content-Type", "application/json")
			c.Header("Cache-Control", "no-cache")
			c.JSON(http.StatusOK, []interface{}{})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to access recommended data",
		})
		return
	}

	// Read file content
	content, err := os.ReadFile(filePath) // #nosec G304 - path is validated via whitelist
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to read recommended data",
		})
		return
	}

	// Generate ETag from file modification time and size
	etag := generateETag(fileInfo)

	// Check If-None-Match header for caching
	ifNoneMatch := c.GetHeader("If-None-Match")
	if ifNoneMatch != "" && ifNoneMatch == etag {
		c.Status(http.StatusNotModified)
		return
	}

	// Set cache headers
	c.Header("Content-Type", "application/json")
	c.Header("Cache-Control", "public, max-age=300, must-revalidate")
	c.Header("ETag", etag)
	c.Header("Last-Modified", fileInfo.ModTime().UTC().Format(time.RFC1123))

	c.Data(http.StatusOK, "application/json", content)
}

// GetRecommendedAI godoc
// @Summary      AI 추천 게시글 조회
// @Description  AI 분석 기반 특정 기간 동안의 추천 게시글 목록을 조회합니다. 파일이 없는 경우 빈 배열을 반환합니다.
// @Tags         recommended
// @Accept       json
// @Produce      json
// @Param        period  path      string  true  "기간 (1h, 3h, 6h, 12h, 24h, 48h)"
// @Success      200     {array}   interface{}  "AI 추천 게시글 목록"
// @Failure      400     {object}  common.APIResponse
// @Failure      500     {object}  common.APIResponse
// @Router       /recommended/ai/{period} [get]
func (h *RecommendedHandler) GetRecommendedAI(c *gin.Context) {
	period := c.Param("period")

	// AI 추천용 period 매핑 (1h -> 1hour)
	periodMap := map[string]string{
		"1h":  "1hour",
		"3h":  "3hours",
		"6h":  "6hours",
		"12h": "12hours",
		"24h": "24hours",
		"48h": "48hours",
	}

	fileName, ok := periodMap[period]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid period. Valid values: 1h, 3h, 6h, 12h, 24h, 48h",
		})
		return
	}

	// AI 분석 파일 경로 (ai_ 접두사 추가)
	filePath := filepath.Join(h.basePath, "ai_"+fileName+".json")
	filePath = filepath.Clean(filePath) // Sanitize path

	// Check if file exists
	fileInfo, err := os.Stat(filePath) // #nosec G304 - path is validated via period mapping
	if err != nil {
		if os.IsNotExist(err) {
			// 파일이 없으면 빈 배열 반환 (개발 환경 대응)
			c.Header("Content-Type", "application/json")
			c.Header("Cache-Control", "no-cache")
			c.JSON(http.StatusOK, []interface{}{})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to access AI recommended data",
		})
		return
	}

	// Read file content
	content, err := os.ReadFile(filePath) // #nosec G304 - path is validated via period mapping
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to read AI recommended data",
		})
		return
	}

	// Generate ETag from file modification time and size
	etag := generateETag(fileInfo)

	// Check If-None-Match header for caching
	ifNoneMatch := c.GetHeader("If-None-Match")
	if ifNoneMatch != "" && ifNoneMatch == etag {
		c.Status(http.StatusNotModified)
		return
	}

	// Set cache headers
	c.Header("Content-Type", "application/json")
	c.Header("Cache-Control", "public, max-age=300, must-revalidate")
	c.Header("ETag", etag)
	c.Header("Last-Modified", fileInfo.ModTime().UTC().Format(time.RFC1123))

	c.Data(http.StatusOK, "application/json", content)
}

// generateETag creates an ETag from file info
func generateETag(info os.FileInfo) string {
	return "\"" + strings.ReplaceAll(info.ModTime().UTC().Format(time.RFC3339Nano), ":", "") + "\""
}
