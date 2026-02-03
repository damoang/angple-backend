package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// FileHandler handles file upload/download HTTP requests
type FileHandler struct {
	service service.FileService
}

// NewFileHandler creates a new FileHandler
func NewFileHandler(service service.FileService) *FileHandler {
	return &FileHandler{service: service}
}

// UploadEditorImage handles POST /api/v2/upload/editor
// @Summary 에디터 이미지 업로드
// @Description 게시글 에디터에 삽입할 이미지를 업로드합니다
// @Tags upload
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "이미지 파일"
// @Param board_id formData string true "게시판 ID"
// @Param wr_id formData int false "게시글 ID (기본 0)"
// @Success 200 {object} common.APIResponse{data=domain.FileUploadResponse}
// @Failure 400 {object} common.APIResponse
// @Failure 401 {object} common.APIResponse
// @Router /upload/editor [post]
func (h *FileHandler) UploadEditorImage(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "파일을 선택해 주세요", err)
		return
	}

	boardID := c.PostForm("board_id")
	if boardID == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "게시판 ID가 필요합니다", nil)
		return
	}

	wrID := 0
	if id := c.PostForm("wr_id"); id != "" {
		wrID, _ = strconv.Atoi(id)
	}

	result, err := h.service.UploadEditorImage(file, boardID, wrID)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// UploadAttachment handles POST /api/v2/upload/attachment
// @Summary 첨부파일 업로드
// @Description 게시글에 첨부할 파일을 업로드합니다
// @Tags upload
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "첨부 파일"
// @Param board_id formData string true "게시판 ID"
// @Param wr_id formData int false "게시글 ID (기본 0)"
// @Success 200 {object} common.APIResponse{data=domain.FileUploadResponse}
// @Failure 400 {object} common.APIResponse
// @Failure 401 {object} common.APIResponse
// @Router /upload/attachment [post]
func (h *FileHandler) UploadAttachment(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "파일을 선택해 주세요", err)
		return
	}

	boardID := c.PostForm("board_id")
	if boardID == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "게시판 ID가 필요합니다", nil)
		return
	}

	wrID := 0
	if id := c.PostForm("wr_id"); id != "" {
		wrID, _ = strconv.Atoi(id)
	}

	result, err := h.service.UploadAttachment(file, boardID, wrID)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// DownloadFile handles GET /api/v2/files/:board_id/:wr_id/:file_no/download
// @Summary 파일 다운로드
// @Description 첨부파일을 다운로드합니다
// @Tags files
// @Produce octet-stream
// @Param board_id path string true "게시판 ID"
// @Param wr_id path int true "게시글 ID"
// @Param file_no path int true "파일 번호"
// @Success 200 {file} binary
// @Failure 404 {object} common.APIResponse
// @Router /files/{board_id}/{wr_id}/{file_no}/download [get]
func (h *FileHandler) DownloadFile(c *gin.Context) {
	boardID := c.Param("board_id")
	wrID, err := strconv.Atoi(c.Param("wr_id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID입니다", err)
		return
	}
	fileNo, err := strconv.Atoi(c.Param("file_no"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 파일 번호입니다", err)
		return
	}

	info, err := h.service.GetFileForDownload(boardID, wrID, fileNo)
	if err != nil {
		common.ErrorResponse(c, http.StatusNotFound, "파일을 찾을 수 없습니다", err)
		return
	}

	c.Header("Content-Disposition", "attachment; filename=\""+info.Source+"\"")
	c.Header("Content-Type", info.ContentType)
	c.File(info.FilePath)
}
