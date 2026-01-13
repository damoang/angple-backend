package handler

import (
	"errors"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gofiber/fiber/v2"
)

type BoardHandler struct {
	service *service.BoardService
}

func NewBoardHandler(service *service.BoardService) *BoardHandler {
	return &BoardHandler{service: service}
}

// CreateBoard - 게시판 생성 (POST /api/v2/boards)
func (h *BoardHandler) CreateBoard(c *fiber.Ctx) error {
	// 1. JWT에서 사용자 정보 추출
	userID, ok := c.Locals("userID").(string)
	if !ok {
		return common.ErrorResponse(c, fiber.StatusUnauthorized, "User not authenticated", nil)
	}

	// 2. 관리자 권한 확인 (레벨 10)
	memberLevel, ok := c.Locals("level").(int)
	if !ok || memberLevel < 10 {
		return common.ErrorResponse(c, fiber.StatusForbidden, "Admin access required", nil)
	}

	// 3. 요청 바디 파싱
	var req domain.CreateBoardRequest
	if err := c.BodyParser(&req); err != nil {
		return common.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", err)
	}

	// 4. 서비스 호출
	board, err := h.service.CreateBoard(&req, userID)
	if err != nil {
		// 중복 체크
		if err.Error() == "board_id already exists" || err.Error() == "board already exists" {
			return common.ErrorResponse(c, fiber.StatusConflict, "Board already exists", err)
		}
		return common.ErrorResponse(c, fiber.StatusBadRequest, "Failed to create board", err)
	}

	// 5. 응답
	return c.Status(fiber.StatusCreated).JSON(common.APIResponse{
		Data: board.ToResponse(),
	})
}

// GetBoard - 게시판 조회 (GET /api/v2/boards/:board_id)
func (h *BoardHandler) GetBoard(c *fiber.Ctx) error {
	boardID := c.Params("board_id")

	board, err := h.service.GetBoard(boardID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return common.ErrorResponse(c, fiber.StatusNotFound, "Board not found", err)
		}
		return common.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch board", err)
	}

	return common.SuccessResponse(c, board.ToResponse(), nil)
}

// ListBoards - 게시판 목록 조회 (GET /api/v2/boards)
func (h *BoardHandler) ListBoards(c *fiber.Ctx) error {
	// 쿼리 파라미터 파싱
	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.Query("page_size", "20"))
	if err != nil {
		pageSize = 20
	}

	boards, total, err := h.service.ListBoards(page, pageSize)
	if err != nil {
		return common.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch boards", err)
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

	return common.SuccessResponse(c, responses, meta)
}

// ListBoardsByGroup - 그룹별 게시판 목록 (GET /api/v2/groups/:group_id/boards)
func (h *BoardHandler) ListBoardsByGroup(c *fiber.Ctx) error {
	groupID := c.Params("group_id")

	boards, err := h.service.ListBoardsByGroup(groupID)
	if err != nil {
		return common.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch boards", err)
	}

	// Response DTO로 변환
	responses := make([]*domain.BoardResponse, len(boards))
	for i, board := range boards {
		responses[i] = board.ToResponse()
	}

	return common.SuccessResponse(c, responses, nil)
}

// UpdateBoard - 게시판 수정 (PUT /api/v2/boards/:board_id)
func (h *BoardHandler) UpdateBoard(c *fiber.Ctx) error {
	boardID := c.Params("board_id")

	// JWT에서 사용자 정보 추출
	userID, ok := c.Locals("userID").(string)
	if !ok {
		return common.ErrorResponse(c, fiber.StatusUnauthorized, "User not authenticated", nil)
	}

	memberLevel, ok := c.Locals("level").(int)
	if !ok {
		memberLevel = 1
	}

	// 요청 바디 파싱
	var req domain.UpdateBoardRequest
	if err := c.BodyParser(&req); err != nil {
		return common.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", err)
	}

	// 서비스 호출
	isAdmin := memberLevel >= 10
	err := h.service.UpdateBoard(boardID, &req, userID, isAdmin)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return common.ErrorResponse(c, fiber.StatusNotFound, "Board not found", err)
		}
		if errors.Is(err, common.ErrForbidden) {
			return common.ErrorResponse(c, fiber.StatusForbidden, "Permission denied", err)
		}
		return common.ErrorResponse(c, fiber.StatusBadRequest, "Failed to update board", err)
	}

	return common.SuccessResponse(c, fiber.Map{
		"message": "Board updated successfully",
	}, nil)
}

// DeleteBoard - 게시판 삭제 (DELETE /api/v2/boards/:board_id)
func (h *BoardHandler) DeleteBoard(c *fiber.Ctx) error {
	boardID := c.Params("board_id")

	// 관리자 권한 확인
	memberLevel, ok := c.Locals("level").(int)
	if !ok || memberLevel < 10 {
		return common.ErrorResponse(c, fiber.StatusForbidden, "Admin access required", nil)
	}

	// 서비스 호출
	err := h.service.DeleteBoard(boardID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return common.ErrorResponse(c, fiber.StatusNotFound, "Board not found", err)
		}
		return common.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to delete board", err)
	}

	return common.SuccessResponse(c, fiber.Map{
		"message": "Board deleted successfully",
	}, nil)
}
