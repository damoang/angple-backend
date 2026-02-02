package v2

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"github.com/damoang/angple-backend/internal/middleware"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MemoHandler handles v2 memo API endpoints
type MemoHandler struct {
	memoRepo v2repo.MemoRepository
}

// NewMemoHandler creates a new MemoHandler
func NewMemoHandler(memoRepo v2repo.MemoRepository) *MemoHandler {
	return &MemoHandler{memoRepo: memoRepo}
}

func (h *MemoHandler) parseUserAndTarget(c *gin.Context) (uint64, uint64, error) {
	userID, err := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
	if err != nil {
		return 0, 0, err
	}
	targetUserID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return 0, 0, err
	}
	return userID, targetUserID, nil
}

// GetMemo handles GET /api/v2/members/:id/memo
func (h *MemoHandler) GetMemo(c *gin.Context) {
	userID, targetUserID, err := h.parseUserAndTarget(c)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}

	memo, err := h.memoRepo.FindByUserAndTarget(userID, targetUserID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			common.V2Success(c, nil)
			return
		}
		common.V2ErrorResponse(c, http.StatusInternalServerError, "메모 조회 실패", err)
		return
	}
	common.V2Success(c, memo)
}

// CreateMemo handles POST /api/v2/members/:id/memo
func (h *MemoHandler) CreateMemo(c *gin.Context) {
	userID, targetUserID, err := h.parseUserAndTarget(c)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}

	var req struct {
		Content string `json:"content" binding:"required"`
		Color   string `json:"color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	color := req.Color
	if color == "" {
		color = "yellow"
	}

	memo := &v2domain.V2Memo{
		UserID:       userID,
		TargetUserID: targetUserID,
		Content:      req.Content,
		Color:        color,
	}
	if err := h.memoRepo.Upsert(memo); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "메모 생성 실패", err)
		return
	}
	common.V2Created(c, memo)
}

// UpdateMemo handles PUT /api/v2/members/:id/memo
func (h *MemoHandler) UpdateMemo(c *gin.Context) {
	userID, targetUserID, err := h.parseUserAndTarget(c)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}

	var req struct {
		Content string `json:"content" binding:"required"`
		Color   string `json:"color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	color := req.Color
	if color == "" {
		color = "yellow"
	}

	memo := &v2domain.V2Memo{
		UserID:       userID,
		TargetUserID: targetUserID,
		Content:      req.Content,
		Color:        color,
	}
	if err := h.memoRepo.Upsert(memo); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "메모 수정 실패", err)
		return
	}
	common.V2Success(c, memo)
}

// DeleteMemo handles DELETE /api/v2/members/:id/memo
func (h *MemoHandler) DeleteMemo(c *gin.Context) {
	userID, targetUserID, err := h.parseUserAndTarget(c)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}

	if err := h.memoRepo.Delete(userID, targetUserID); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "메모 삭제 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "메모 삭제 완료"})
}
