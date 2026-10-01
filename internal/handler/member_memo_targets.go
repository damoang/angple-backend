package handler

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MemoTargetsLimit 는 memo-targets 응답에 담는 상대 ID 의 최대 개수다.
//
// 상대 ID 목록만 돌려주므로 상한까지 가도 응답은 수백 KB 미만이다.
// 상한을 넘는 회원은 truncated=true 를 받고, 클라이언트는 기존 방식
// (목록 화면마다 batch/memo 조회)으로 폴백한다.
const MemoTargetsLimit = 20000

// MemoTargets 는 GET /api/v1/members/me/memo-targets 의 data 본문이다.
type MemoTargets struct {
	Targets   []string `json:"targets"`
	Count     int      `json:"count"`
	Truncated bool     `json:"truncated"`
}

// LoadMemoTargets 는 memberID 가 내용 있는 메모를 남긴 상대 회원 ID 목록을 돌려준다.
//
// unique_keys(member_id, target_member_id) 로 끝나는 조회다(memo 조건만 행 확인).
// limit+1 개까지 읽어 상한 초과 여부를 판정하고, 초과하면 limit 개로 자른 뒤
// Truncated=true 로 표시한다. limit 이 0 이하이면 MemoTargetsLimit 을 쓴다.
func LoadMemoTargets(db *gorm.DB, memberID string, limit int) (MemoTargets, error) {
	if limit <= 0 {
		limit = MemoTargetsLimit
	}
	result := MemoTargets{Targets: []string{}}
	if memberID == "" {
		return result, nil
	}
	var targets []string
	if err := db.Table("g5_member_memo").
		Where("member_id = ? AND memo <> ''", memberID).
		Limit(limit+1).
		Pluck("target_member_id", &targets).Error; err != nil {
		return result, err
	}
	if len(targets) > limit {
		targets = targets[:limit]
		result.Truncated = true
	}
	if targets != nil {
		result.Targets = targets
	}
	result.Count = len(result.Targets)
	return result, nil
}

// MemberMemoTargetsHandler 는 로그인 회원의 메모 대상 ID 목록 API 를 제공한다.
type MemberMemoTargetsHandler struct {
	db *gorm.DB
}

// NewMemberMemoTargetsHandler 는 MemberMemoTargetsHandler 를 만든다.
func NewMemberMemoTargetsHandler(db *gorm.DB) *MemberMemoTargetsHandler {
	return &MemberMemoTargetsHandler{db: db}
}

// GetMemoTargets 는 GET /api/v1/members/me/memo-targets 를 처리한다.
//
// 목록 화면의 작성자 중 메모를 단 상대가 있을 때만 batch/memo 를 부르도록
// 클라이언트가 세션당 1회 받아 두는 목록이다. 조회 실패 시 500 을 돌려주며,
// 클라이언트는 이를 「모름」으로 보고 기존 방식으로 동작한다.
func (h *MemberMemoTargetsHandler) GetMemoTargets(c *gin.Context) {
	memberID := middleware.GetUserID(c)
	if memberID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "로그인이 필요합니다"})
		return
	}
	data, err := LoadMemoTargets(h.db, memberID, MemoTargetsLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "메모 대상 조회 실패"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}
