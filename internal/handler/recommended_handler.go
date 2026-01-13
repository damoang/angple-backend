package handler

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
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
func (h *RecommendedHandler) GetRecommended(c *fiber.Ctx) error {
	period := c.Params("period")

	// Validate period to prevent path traversal
	if !validPeriods[period] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid period. Valid values: 1hour, 3hours, 6hours, 12hours, 24hours, 48hours, index-widgets",
		})
	}

	// Construct file path - 최신 데이터 파일 사용 (AI 분석 없어도 됨)
	var filename string
	if period == "index-widgets" {
		filename = "index-widgets.json"
	} else {
		filename = period + ".json" // 1hour.json, 3hours.json 등 (최신 데이터)
	}
	filePath := filepath.Join(h.basePath, filename)

	// Check if file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 파일이 없으면 빈 배열 반환 (개발 환경 대응)
			c.Set("Content-Type", "application/json")
			c.Set("Cache-Control", "no-cache")
			return c.JSON([]interface{}{})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to access recommended data",
		})
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read recommended data",
		})
	}

	// Generate ETag from file modification time and size
	etag := generateETag(fileInfo)

	// Check If-None-Match header for caching
	ifNoneMatch := c.Get("If-None-Match")
	if ifNoneMatch != "" && ifNoneMatch == etag {
		return c.SendStatus(fiber.StatusNotModified)
	}

	// Set cache headers
	c.Set("Content-Type", "application/json")
	c.Set("Cache-Control", "public, max-age=300, must-revalidate")
	c.Set("ETag", etag)
	c.Set("Last-Modified", fileInfo.ModTime().UTC().Format(time.RFC1123))

	return c.Send(content)
}

// generateETag creates an ETag from file info
func generateETag(info os.FileInfo) string {
	return "\"" + strings.ReplaceAll(info.ModTime().UTC().Format(time.RFC3339Nano), ":", "") + "\""
}
