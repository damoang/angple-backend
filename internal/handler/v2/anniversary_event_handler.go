package v2

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	"github.com/damoang/angple-backend/internal/middleware"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const anniversaryDrawRelTable = "@event-2026-2nd-anniversary"
const anniversaryDrawRelAction = "@anniversary-draw"

type anniversaryReward struct {
	Result      string
	PointAmount int
	Weight      int64
}

var anniversaryRewards = []anniversaryReward{
	{Result: "함께 만든 큰 울림", PointAmount: 100, Weight: 5},
	{Result: "열린 참여의 기쁨", PointAmount: 50, Weight: 15},
	{Result: "작은 목소리의 힘", PointAmount: 30, Weight: 30},
	{Result: "따뜻한 연결", PointAmount: 10, Weight: 50},
}

type AnniversaryEventHandler struct {
	repo            gnurepo.AnniversaryDrawRepository
	pointWriteRepo  v2repo.GnuboardPointWriteRepository
	pointConfigRepo v2repo.PointConfigRepository
	db              *gorm.DB
}

func NewAnniversaryEventHandler(
	repo gnurepo.AnniversaryDrawRepository,
	pointWriteRepo v2repo.GnuboardPointWriteRepository,
	pointConfigRepo v2repo.PointConfigRepository,
	db *gorm.DB,
) *AnniversaryEventHandler {
	return &AnniversaryEventHandler{
		repo:            repo,
		pointWriteRepo:  pointWriteRepo,
		pointConfigRepo: pointConfigRepo,
		db:              db,
	}
}

type anniversaryDrawResponse struct {
	EventCode             string  `json:"event_code"`
	Participated          bool    `json:"participated"`
	AlreadyParticipated   bool    `json:"already_participated"`
	DrawResult            string  `json:"draw_result,omitempty"`
	PointAmount           int     `json:"point_amount,omitempty"`
	DeferredGrantMessage  string  `json:"deferred_grant_message"`
	GrantScheduledDateKST string  `json:"grant_scheduled_date_kst"`
	GrantedAt             *string `json:"granted_at,omitempty"`
	CreatedAt             *string `json:"created_at,omitempty"`
}

func (h *AnniversaryEventHandler) GetStatus(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	entry, err := h.repo.GetByMember(gnudomain.AnniversaryEventCode2026, mbID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "이벤트 참여 조회에 실패했습니다", err)
		return
	}

	common.V2Success(c, buildAnniversaryDrawResponse(entry, entry != nil))
}

func (h *AnniversaryEventHandler) Participate(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	existing, err := h.repo.GetByMember(gnudomain.AnniversaryEventCode2026, mbID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "이벤트 참여 조회에 실패했습니다", err)
		return
	}
	if existing != nil {
		common.V2Success(c, buildAnniversaryDrawResponse(existing, true))
		return
	}

	reward, err := pickAnniversaryReward()
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "이벤트 결과 생성에 실패했습니다", err)
		return
	}

	entry := &gnudomain.AnniversaryDrawEntry{
		EventCode:   gnudomain.AnniversaryEventCode2026,
		MbID:        mbID,
		DrawResult:  reward.Result,
		PointAmount: reward.PointAmount,
	}
	if err := h.repo.Create(entry); err != nil {
		// Unique constraint race: load the existing row and return it.
		existing, loadErr := h.repo.GetByMember(gnudomain.AnniversaryEventCode2026, mbID)
		if loadErr == nil && existing != nil {
			common.V2Success(c, buildAnniversaryDrawResponse(existing, true))
			return
		}
		common.V2ErrorResponse(c, http.StatusInternalServerError, "이벤트 참여 저장에 실패했습니다", err)
		return
	}

	common.V2Success(c, buildAnniversaryDrawResponse(entry, false))
}

type anniversaryGrantRequest struct {
	Limit int `json:"limit"`
}

func (h *AnniversaryEventHandler) AdminGrantPending(c *gin.Context) {
	if h.pointWriteRepo == nil || h.pointConfigRepo == nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "포인트 지급 기능이 비활성화되어 있습니다", nil)
		return
	}

	var req anniversaryGrantRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청입니다", err)
			return
		}
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	pointConfig, err := h.pointConfigRepo.GetPointConfig()
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "포인트 설정 조회에 실패했습니다", err)
		return
	}

	entries, err := h.repo.ListPending(gnudomain.AnniversaryEventCode2026, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "미지급 이벤트 조회에 실패했습니다", err)
		return
	}

	granted := 0
	alreadyGranted := 0
	failed := 0

	for _, entry := range entries {
		pointPoID, lookupErr := h.findExistingPointLog(entry.ID)
		if lookupErr != nil {
			failed++
			continue
		}
		if pointPoID != 0 {
			if err := h.repo.MarkGranted(entry.ID, pointPoID, time.Now()); err != nil {
				failed++
				continue
			}
			alreadyGranted++
			continue
		}

		content := "다모앙 2주년 이벤트 포인트"
		relID := strconv.FormatUint(entry.ID, 10)

		if err := h.pointWriteRepo.AddPoint(
			entry.MbID,
			entry.PointAmount,
			content,
			anniversaryDrawRelTable,
			relID,
			anniversaryDrawRelAction,
			pointConfig,
		); err != nil {
			failed++
			continue
		}

		pointPoID, lookupErr = h.findExistingPointLog(entry.ID)
		if lookupErr != nil || pointPoID == 0 {
			failed++
			continue
		}
		if err := h.repo.MarkGranted(entry.ID, pointPoID, time.Now()); err != nil {
			failed++
			continue
		}
		granted++
	}

	common.V2Success(c, gin.H{
		"event_code":        gnudomain.AnniversaryEventCode2026,
		"requested_limit":   limit,
		"pending_fetched":   len(entries),
		"granted":           granted,
		"already_granted":   alreadyGranted,
		"failed":            failed,
		"grant_message_kst": "포인트는 KST 기준으로 순차 지급됩니다.",
	})
}

func buildAnniversaryDrawResponse(entry *gnudomain.AnniversaryDrawEntry, alreadyParticipated bool) anniversaryDrawResponse {
	resp := anniversaryDrawResponse{
		EventCode:             gnudomain.AnniversaryEventCode2026,
		Participated:          entry != nil,
		AlreadyParticipated:   alreadyParticipated,
		DeferredGrantMessage:  "포인트는 이벤트 종료 후 2026년 3월 31일 이후 순차 지급됩니다. Points will be granted after the event.",
		GrantScheduledDateKST: "2026-03-31 KST",
	}

	if entry == nil {
		return resp
	}

	resp.DrawResult = entry.DrawResult
	resp.PointAmount = entry.PointAmount
	createdAt := entry.CreatedAt.Format(time.RFC3339)
	resp.CreatedAt = &createdAt
	if entry.GrantedAt != nil {
		grantedAt := entry.GrantedAt.Format(time.RFC3339)
		resp.GrantedAt = &grantedAt
	}
	return resp
}

func pickAnniversaryReward() (anniversaryReward, error) {
	var total int64
	for _, reward := range anniversaryRewards {
		total += reward.Weight
	}

	n, err := rand.Int(rand.Reader, big.NewInt(total))
	if err != nil {
		return anniversaryReward{}, err
	}

	cursor := n.Int64()
	for _, reward := range anniversaryRewards {
		if cursor < reward.Weight {
			return reward, nil
		}
		cursor -= reward.Weight
	}

	return anniversaryRewards[len(anniversaryRewards)-1], nil
}

func (h *AnniversaryEventHandler) findExistingPointLog(entryID uint64) (int, error) {
	var pointID int
	err := h.db.Table("g5_point").
		Select("po_id").
		Where("po_rel_table = ? AND po_rel_id = ? AND po_rel_action = ?",
			anniversaryDrawRelTable,
			strconv.FormatUint(entryID, 10),
			anniversaryDrawRelAction,
		).
		Order("po_id DESC").
		Limit(1).
		Scan(&pointID).Error
	return pointID, err
}
