package handler

import (
	"errors"
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/damoang/angple-backend/pkg/ginutil"
	"github.com/gin-gonic/gin"
)

type BoardHandler struct {
	service *service.BoardService
}

func NewBoardHandler(service *service.BoardService) *BoardHandler {
	return &BoardHandler{service: service}
}

// CreateBoard - 게시판 생성 (POST /api/v2/boards)
func (h *BoardHandler) CreateBoard(c *gin.Context) {
	// 1. JWT에서 사용자 정보 추출
	userID := middleware.GetUserID(c)

	// 2. 관리자 권한 확인 (레벨 10)
	levelVal, exists := c.Get("level")
	memberLevel := 1
	if exists {
		if level, ok := levelVal.(int); ok {
			memberLevel = level
		}
	}
	if memberLevel < 10 {
		common.ErrorResponse(c, http.StatusForbidden, "Admin access required", nil)
		return
	}

	// 3. 요청 바디 파싱
	var req domain.CreateBoardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// 4. 서비스 호출
	board, err := h.service.CreateBoard(&req, userID)
	if err != nil {
		// 중복 체크
		if err.Error() == "board_id already exists" || err.Error() == "board already exists" {
			common.ErrorResponse(c, http.StatusConflict, "Board already exists", err)
			return
		}
		common.ErrorResponse(c, http.StatusBadRequest, "Failed to create board", err)
		return
	}

	// 5. 응답
	c.JSON(http.StatusCreated, common.APIResponse{
		Data: board.ToResponse(),
	})
}

// GetBoard - 게시판 조회 (GET /api/v2/boards/:board_id)
func (h *BoardHandler) GetBoard(c *gin.Context) {
	boardID := c.Param("board_id")

	board, err := h.service.GetBoard(boardID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Board not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch board", err)
		return
	}

	common.SuccessResponse(c, board.ToResponse(), nil)
}

// ListBoards - 게시판 목록 조회 (GET /api/v2/boards)
func (h *BoardHandler) ListBoards(c *gin.Context) {
	// 쿼리 파라미터 파싱
	page := ginutil.QueryInt(c, "page", 1)
	pageSize := ginutil.QueryInt(c, "page_size", 20)

	boards, total, err := h.service.ListBoards(page, pageSize)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch boards", err)
		return
	}

	// Response DTO로 변환
	responses := make([]*domain.BoardResponse, len(boards))
	for i, board := range boards {
		responses[i] = board.ToResponse()
	}

	// 메타 정보
	meta := &common.Meta{
		Page:  page,
		Limit: pageSize,
		Total: total,
	}

	common.SuccessResponse(c, responses, meta)
}

// ListBoardsByGroup - 그룹별 게시판 목록 (GET /api/v2/groups/:group_id/boards)
func (h *BoardHandler) ListBoardsByGroup(c *gin.Context) {
	groupID := c.Param("group_id")

	boards, err := h.service.ListBoardsByGroup(groupID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch boards", err)
		return
	}

	// Response DTO로 변환
	responses := make([]*domain.BoardResponse, len(boards))
	for i, board := range boards {
		responses[i] = board.ToResponse()
	}

	common.SuccessResponse(c, responses, nil)
}

// UpdateBoard - 게시판 수정 (PUT /api/v2/boards/:board_id)
func (h *BoardHandler) UpdateBoard(c *gin.Context) {
	boardID := c.Param("board_id")

	// JWT에서 사용자 정보 추출
	userID := middleware.GetUserID(c)

	levelVal, exists := c.Get("level")
	memberLevel := 1
	if exists {
		if level, ok := levelVal.(int); ok {
			memberLevel = level
		}
	}

	// 요청 바디 파싱
	var req domain.UpdateBoardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// 서비스 호출
	isAdmin := memberLevel >= 10
	err := h.service.UpdateBoard(boardID, &req, userID, isAdmin)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Board not found", err)
			return
		}
		if errors.Is(err, common.ErrForbidden) {
			common.ErrorResponse(c, http.StatusForbidden, "Permission denied", err)
			return
		}
		common.ErrorResponse(c, http.StatusBadRequest, "Failed to update board", err)
		return
	}

	common.SuccessResponse(c, gin.H{
		"message": "Board updated successfully",
	}, nil)
}

// DeleteBoard - 게시판 삭제 (DELETE /api/v2/boards/:board_id)
func (h *BoardHandler) DeleteBoard(c *gin.Context) {
	boardID := c.Param("board_id")

	// 관리자 권한 확인
	levelVal, exists := c.Get("level")
	memberLevel := 1
	if exists {
		if level, ok := levelVal.(int); ok {
			memberLevel = level
		}
	}
	if memberLevel < 10 {
		common.ErrorResponse(c, http.StatusForbidden, "Admin access required", nil)
		return
	}

	// 서비스 호출
	err := h.service.DeleteBoard(boardID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Board not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete board", err)
		return
	}

	common.SuccessResponse(c, gin.H{
		"message": "Board deleted successfully",
	}, nil)
}
