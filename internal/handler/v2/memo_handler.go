package v2

import (
	"context"
	"errors"
	"log"
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
	// userDB 는 탈퇴 대상 가드가 v2_users.id → username(mb_id) 을 풀 때 쓴다.
	// nil 이면 가드를 끈다(WithWithdrawnTargetGuard 미호출).
	userDB *gorm.DB
}

// NewMemoHandler creates a new MemoHandler
func NewMemoHandler(memoRepo v2repo.MemoRepository) *MemoHandler {
	return &MemoHandler{memoRepo: memoRepo}
}

// memoWithdrawnTargetMessage 는 탈퇴 회원 대상 메모 저장·수정 거부 문구다(v1 과 같은 문구).
const memoWithdrawnTargetMessage = "탈퇴한 회원에게는 메모를 남길 수 없습니다."

// v2MemoMemberWithdrawn 은 mb_id 의 탈퇴 판정이다 — 테스트에서 바꿔 끼울 수 있도록 변수로 둔다.
var v2MemoMemberWithdrawn = middleware.IsWithdrawnMember

// WithWithdrawnTargetGuard 는 탈퇴(숙려 포함) 회원 대상 메모 가드를 켠다.
//
// 탈퇴 회원 정보 비노출 정책을 회원 메모에도 적용한다. 대상이 탈퇴 회원이면
// 조회는 「메모 없음」과 같은 응답, 저장·수정은 거부한다. 삭제는 막지 않는다.
// db 는 v2_users 를 읽을 DB 다.
func (h *MemoHandler) WithWithdrawnTargetGuard(db *gorm.DB) *MemoHandler {
	h.userDB = db
	return h
}

// targetWithdrawn 은 v2 대상 ID(v2_users.id)를 username(= g5_member.mb_id)으로 풀어
// 탈퇴(숙려 포함) 여부를 판정한다. v2_users 에 없는 ID 는 탈퇴가 아니다.
// 조회 실패는 error 로 돌려준다(조회=비노출, 저장=거부는 호출자가 정한다).
func (h *MemoHandler) targetWithdrawn(ctx context.Context, targetUserID uint64) (bool, error) {
	if h.userDB == nil {
		return false, nil
	}
	var username string
	if err := h.userDB.WithContext(ctx).Table("v2_users").
		Select("username").Where("id = ?", targetUserID).
		Limit(1).Scan(&username).Error; err != nil {
		return false, err
	}
	if username == "" {
		return false, nil
	}
	return v2MemoMemberWithdrawn(ctx, username)
}

// rejectWriteToWithdrawnTarget 는 탈퇴 대상(또는 판정 실패)이면 응답을 쓰고 true 를 돌려준다.
func (h *MemoHandler) rejectWriteToWithdrawnTarget(c *gin.Context, targetUserID uint64) bool {
	withdrawn, err := h.targetWithdrawn(c.Request.Context(), targetUserID)
	if err != nil {
		log.Printf("[v2 memoGuard] 탈퇴 판정 실패 — 저장 거부 (%d): %v", targetUserID, err)
		common.V2ErrorResponse(c, http.StatusServiceUnavailable, "메모를 저장하지 못했습니다. 잠시 후 다시 시도해 주세요.", err)
		return true
	}
	if withdrawn {
		common.V2ErrorResponse(c, http.StatusForbidden, memoWithdrawnTargetMessage, nil)
		return true
	}
	return false
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.V2Success(c, nil)
			return
		}
		common.V2ErrorResponse(c, http.StatusInternalServerError, "메모 조회 실패", err)
		return
	}
	// 탈퇴(숙려 포함) 대상 메모는 「메모 없음」과 같은 응답으로 숨긴다(판정 실패 시에도).
	withdrawn, gerr := h.targetWithdrawn(c.Request.Context(), targetUserID)
	if gerr != nil {
		log.Printf("[v2 memoGuard] 탈퇴 판정 실패 — 조회 비노출 (%d): %v", targetUserID, gerr)
	}
	if withdrawn || gerr != nil {
		common.V2Success(c, nil)
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
	if h.rejectWriteToWithdrawnTarget(c, targetUserID) {
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
	if h.rejectWriteToWithdrawnTarget(c, targetUserID) {
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

// GetAllMemos handles GET /api/v2/members/:id/memo/all (admin only)
func (h *MemoHandler) GetAllMemos(c *gin.Context) {
	targetUserID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}

	memos, err := h.memoRepo.FindAllByTarget(targetUserID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "메모 조회 실패", err)
		return
	}
	common.V2Success(c, memos)
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
