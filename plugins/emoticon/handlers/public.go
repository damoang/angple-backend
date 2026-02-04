//go:build ignore

// 이모티콘 공개 API 핸들러
package handlers

import (
	"net/http"

	"angple-backend/plugins/emoticon/service"

	"github.com/gin-gonic/gin"
)

var svc *service.Service

// SetService 서비스 인스턴스 설정
func SetService(s *service.Service) {
	svc = s
}

// ListPacks 활성 팩 목록
// GET /api/plugins/emoticon/packs
func ListPacks(c *gin.Context) {
	packs, err := svc.GetActivePacks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "팩 목록 조회 실패"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"packs": packs,
		"total": len(packs),
	})
}

// ListPackItems 팩 내 아이템 목록
// GET /api/plugins/emoticon/packs/:slug/items
func ListPackItems(c *gin.Context) {
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "슬러그가 필요합니다"})
		return
	}

	items, err := svc.GetItemsByPackSlug(slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "팩을 찾을 수 없습니다"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"total": len(items),
	})
}

// ServeImage 이모티콘 이미지 서빙
// GET /api/plugins/emoticon/image/:filename
func ServeImage(c *gin.Context) {
	filename := c.Param("filename")
	path, err := svc.GetAssetPath(filename)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "이미지를 찾을 수 없습니다"})
		return
	}

	c.File(path)
}

// ServeThumb 이모티콘 썸네일 서빙
// GET /api/plugins/emoticon/thumb/:filename
func ServeThumb(c *gin.Context) {
	// 썸네일이 없으면 원본 서빙
	ServeImage(c)
}
