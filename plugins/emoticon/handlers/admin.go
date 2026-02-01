//go:build ignore

// 이모티콘 관리자 API 핸들러
package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
)

// AdminListPacks 관리자 전체 팩 목록
// GET /api/plugins/emoticon/admin/packs
func AdminListPacks(c *gin.Context) {
	packs, err := svc.GetAllPacks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "팩 목록 조회 실패"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"packs": packs,
		"total": len(packs),
	})
}

// CreatePack ZIP 업로드로 팩 생성
// POST /api/plugins/emoticon/admin/packs
func CreatePack(c *gin.Context) {
	slug := c.PostForm("slug")
	name := c.PostForm("name")

	if slug == "" || name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug와 name은 필수입니다"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ZIP 파일이 필요합니다"})
		return
	}

	// 임시 파일 저장
	tmpPath := filepath.Join(os.TempDir(), "emoticon_upload_"+slug+".zip")
	if err := c.SaveUploadedFile(file, tmpPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "파일 저장 실패"})
		return
	}
	defer os.Remove(tmpPath)

	pack, err := svc.CreatePackFromZip(tmpPath, slug, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, pack)
}

// UpdatePack 팩 수정
// PUT /api/plugins/emoticon/admin/packs/:id
func UpdatePack(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "잘못된 팩 ID"})
		return
	}

	var req struct {
		Name         string `json:"name"`
		DefaultWidth *int   `json:"default_width"`
		SortOrder    *int   `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.DefaultWidth != nil {
		updates["default_width"] = *req.DefaultWidth
	}
	if req.SortOrder != nil {
		updates["sort_order"] = *req.SortOrder
	}

	pack, err := svc.UpdatePack(id, updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "팩 수정 실패"})
		return
	}

	c.JSON(http.StatusOK, pack)
}

// DeletePack 팩 삭제
// DELETE /api/plugins/emoticon/admin/packs/:id
func DeletePack(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "잘못된 팩 ID"})
		return
	}

	if err := svc.DeletePack(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "팩 삭제 실패"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "팩이 삭제되었습니다"})
}

// ImportLegacy ang-gnu 레거시 이모티콘 일괄 임포트
// POST /api/plugins/emoticon/admin/import-legacy
func ImportLegacy(c *gin.Context) {
	var req struct {
		LegacyDir string `json:"legacy_dir" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "legacy_dir 경로가 필요합니다"})
		return
	}

	packsCreated, itemsCreated, err := svc.ImportLegacy(req.LegacyDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"packs_created": packsCreated,
		"items_created": itemsCreated,
		"message":       "레거시 이모티콘 임포트 완료",
	})
}

// TogglePack 팩 활성/비활성 토글
// POST /api/plugins/emoticon/admin/packs/:id/toggle
func TogglePack(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "잘못된 팩 ID"})
		return
	}

	pack, err := svc.TogglePack(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "토글 실패"})
		return
	}

	c.JSON(http.StatusOK, pack)
}
