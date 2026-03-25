package v2

import (
	"net/http"
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdvertiserBoardPolicyHandler handles advertiser board policy endpoints.
// This policy is rollout-safe by design: a persisted policy does not affect runtime
// unless explicitly enabled, and current code uses shadow mode only.
type AdvertiserBoardPolicyHandler struct {
	boardRepo           v2repo.BoardRepository
	policyRepo          v2repo.AdvertiserBoardPolicyRepository
	writeRestrictionSvc *service.BoardWriteRestrictionService
	gnuDB               *gorm.DB
}

func NewAdvertiserBoardPolicyHandler(
	boardRepo v2repo.BoardRepository,
	policyRepo v2repo.AdvertiserBoardPolicyRepository,
	writeRestrictionSvc *service.BoardWriteRestrictionService,
	gnuDB *gorm.DB,
) *AdvertiserBoardPolicyHandler {
	return &AdvertiserBoardPolicyHandler{
		boardRepo:           boardRepo,
		policyRepo:          policyRepo,
		writeRestrictionSvc: writeRestrictionSvc,
		gnuDB:               gnuDB,
	}
}

func (h *AdvertiserBoardPolicyHandler) GetPolicy(c *gin.Context) {
	slug := c.Param("slug")

	if middleware.GetUserLevel(c) < 10 {
		common.V2ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
		return
	}

	if _, err := h.boardRepo.FindBySlug(slug); err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시판을 찾을 수 없습니다", err)
		return
	}

	policy, err := h.policyRepo.FindByBoardSlug(slug)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "광고주 정책 조회 실패", err)
		return
	}

	common.V2Success(c, policy)
}

func (h *AdvertiserBoardPolicyHandler) UpdatePolicy(c *gin.Context) {
	slug := c.Param("slug")

	if middleware.GetUserLevel(c) < 10 {
		common.V2ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
		return
	}

	if _, err := h.boardRepo.FindBySlug(slug); err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시판을 찾을 수 없습니다", err)
		return
	}

	var req struct {
		Enabled                      *bool   `json:"enabled"`
		Mode                         *string `json:"mode"`
		AllowActiveAdvertiserRead    *bool   `json:"allow_active_advertiser_read"`
		AllowActiveAdvertiserWrite   *bool   `json:"allow_active_advertiser_write"`
		AllowActiveAdvertiserComment *bool   `json:"allow_active_advertiser_comment"`
		RequireCertification         *bool   `json:"require_certification"`
		DailyPostLimit               *int    `json:"daily_post_limit"`
		AllowedRoutesJSON            *string `json:"allowed_routes_json"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	policy, err := h.policyRepo.FindByBoardSlug(slug)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "광고주 정책 조회 실패", err)
		return
	}

	if req.Enabled != nil {
		policy.Enabled = *req.Enabled
	}
	if req.Mode != nil {
		mode := strings.ToLower(strings.TrimSpace(*req.Mode))
		if mode != "" && mode != "shadow" && mode != "enforce" {
			common.V2ErrorResponse(c, http.StatusBadRequest, "mode는 shadow 또는 enforce만 허용됩니다", nil)
			return
		}
		if mode != "" {
			policy.Mode = mode
		}
	}
	if req.AllowActiveAdvertiserRead != nil {
		policy.AllowActiveAdvertiserRead = *req.AllowActiveAdvertiserRead
	}
	if req.AllowActiveAdvertiserWrite != nil {
		policy.AllowActiveAdvertiserWrite = *req.AllowActiveAdvertiserWrite
	}
	if req.AllowActiveAdvertiserComment != nil {
		policy.AllowActiveAdvertiserComment = *req.AllowActiveAdvertiserComment
	}
	if req.RequireCertification != nil {
		policy.RequireCertification = *req.RequireCertification
	}
	if req.DailyPostLimit != nil {
		if *req.DailyPostLimit < 0 {
			common.V2ErrorResponse(c, http.StatusBadRequest, "daily_post_limit은 0 이상이어야 합니다", nil)
			return
		}
		policy.DailyPostLimit = *req.DailyPostLimit
	}
	if req.AllowedRoutesJSON != nil {
		policy.AllowedRoutesJSON = req.AllowedRoutesJSON
	}

	if policy.Mode == "" {
		policy.Mode = "shadow"
	}
	policy.BoardID = slug

	if err := h.policyRepo.Upsert(policy); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "광고주 정책 저장 실패", err)
		return
	}

	common.V2Success(c, policy)
}

func (h *AdvertiserBoardPolicyHandler) EvaluatePolicy(c *gin.Context) {
	slug := c.Param("slug")

	if middleware.GetUserLevel(c) < 10 {
		common.V2ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
		return
	}

	if _, err := h.boardRepo.FindBySlug(slug); err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시판을 찾을 수 없습니다", err)
		return
	}

	memberID := strings.TrimSpace(c.Query("member_id"))
	if memberID == "" {
		common.V2ErrorResponse(c, http.StatusBadRequest, "member_id가 필요합니다", nil)
		return
	}

	memberLevel, err := h.lookupMemberLevel(memberID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "회원 레벨 조회 실패", err)
		return
	}

	comparison, err := h.writeRestrictionSvc.CompareWithAdvertiserPolicy(slug, memberID, memberLevel)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "광고주 정책 dry-run 평가 실패", err)
		return
	}

	common.V2Success(c, gin.H{
		"board_id":     slug,
		"member_id":    memberID,
		"member_level": memberLevel,
		"base":         comparison.Base,
		"policy":       comparison.Policy,
		"candidate":    comparison.Candidate,
		"would_change": !writeRestrictionMatches(comparison.Base, comparison.Candidate),
	})
}

func (h *AdvertiserBoardPolicyHandler) lookupMemberLevel(memberID string) (int, error) {
	var row struct {
		MbLevel int `gorm:"column:mb_level"`
	}

	if err := h.gnuDB.Table("g5_member").Select("mb_level").Where("mb_id = ?", memberID).Take(&row).Error; err != nil {
		return 0, err
	}
	return row.MbLevel, nil
}

func writeRestrictionMatches(a, b *service.WriteRestrictionResult) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.CanWrite == b.CanWrite &&
		a.Remaining == b.Remaining &&
		a.DailyLimit == b.DailyLimit &&
		a.TotalLimit == b.TotalLimit &&
		a.TotalCount == b.TotalCount &&
		a.Reason == b.Reason
}
