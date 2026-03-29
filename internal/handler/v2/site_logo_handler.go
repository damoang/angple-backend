package v2

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var mmddRegex = regexp.MustCompile(`^\d{2}-\d{2}$`)

// SiteLogoHandler handles site logo API endpoints
type SiteLogoHandler struct {
	logoRepo v2repo.SiteLogoRepository
}

// NewSiteLogoHandler creates a new SiteLogoHandler
func NewSiteLogoHandler(logoRepo v2repo.SiteLogoRepository) *SiteLogoHandler {
	return &SiteLogoHandler{logoRepo: logoRepo}
}

// ListLogos handles GET /api/v1/admin/logos
func (h *SiteLogoHandler) ListLogos(c *gin.Context) {
	logos, err := h.logoRepo.FindAll()
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "로고 목록 조회 실패", err)
		return
	}
	common.V2Success(c, logos)
}

// CreateLogo handles POST /api/v1/admin/logos
func (h *SiteLogoHandler) CreateLogo(c *gin.Context) {
	var req v2domain.CreateSiteLogoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}

	if err := h.validateSchedule(req.ScheduleType, req.RecurringDate, req.StartDate, req.EndDate); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	// default 타입은 활성 상태 1개만 허용
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	if req.ScheduleType == "default" && isActive {
		count, err := h.logoRepo.CountActiveDefault()
		if err != nil {
			common.V2ErrorResponse(c, http.StatusInternalServerError, "기본 로고 확인 실패", err)
			return
		}
		if count > 0 {
			common.V2ErrorResponse(c, http.StatusBadRequest, "활성 상태인 기본 로고는 1개만 허용됩니다", nil)
			return
		}
	}

	logo := &v2domain.SiteLogo{
		Name:          req.Name,
		LogoURL:       req.LogoURL,
		ScheduleType:  req.ScheduleType,
		RecurringDate: req.RecurringDate,
		StartDate:     req.StartDate,
		EndDate:       req.EndDate,
		Priority:      req.Priority,
		IsActive:      isActive,
	}

	if err := h.logoRepo.Create(logo); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "로고 생성 실패", err)
		return
	}

	common.V2Success(c, logo)
}

// CreatePresetLogos handles POST /api/v1/admin/logos/presets
func (h *SiteLogoHandler) CreatePresetLogos(c *gin.Context) {
	var req v2domain.CreatePresetLogosRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	existingLogos, err := h.logoRepo.FindAll()
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "기존 로고 조회 실패", err)
		return
	}

	result := &v2domain.CreatePresetLogosResult{
		Created: []*v2domain.SiteLogo{},
		Skipped: []v2domain.CreatePresetLogoSkipped{},
	}

	existingRecurring := map[string][]*v2domain.SiteLogo{}
	for _, logo := range existingLogos {
		if logo.ScheduleType != "recurring" || !logo.IsActive || logo.RecurringDate == nil {
			continue
		}
		existingRecurring[*logo.RecurringDate] = append(existingRecurring[*logo.RecurringDate], logo)
	}

	for _, item := range req.Items {
		recurringDate := item.RecurringDate
		if err := h.validateSchedule("recurring", &recurringDate, nil, nil); err != nil {
			common.V2ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
			return
		}

		if h.hasRecurringPresetConflict(existingRecurring[recurringDate], item.Name, req.LogoURL) {
			result.Skipped = append(result.Skipped, v2domain.CreatePresetLogoSkipped{
				Name:          item.Name,
				RecurringDate: recurringDate,
				Reason:        "같은 날짜에 활성 반복 로고가 이미 존재합니다",
			})
			continue
		}

		logo := &v2domain.SiteLogo{
			Name:          item.Name,
			LogoURL:       req.LogoURL,
			ScheduleType:  "recurring",
			RecurringDate: &recurringDate,
			Priority:      req.Priority,
			IsActive:      isActive,
		}

		if err := h.logoRepo.Create(logo); err != nil {
			common.V2ErrorResponse(c, http.StatusInternalServerError, "절기 프리셋 로고 생성 실패", err)
			return
		}

		result.Created = append(result.Created, logo)
		existingRecurring[recurringDate] = append(existingRecurring[recurringDate], logo)
	}

	common.V2Success(c, result)
}

// UpdateLogo handles PUT /api/v1/admin/logos/:id
func (h *SiteLogoHandler) UpdateLogo(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 로고 ID", err)
		return
	}

	logo, err := h.logoRepo.FindByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			common.V2ErrorResponse(c, http.StatusNotFound, "로고를 찾을 수 없습니다", nil)
			return
		}
		common.V2ErrorResponse(c, http.StatusInternalServerError, "로고 조회 실패", err)
		return
	}

	var req v2domain.UpdateSiteLogoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}

	if req.Name != nil {
		logo.Name = *req.Name
	}
	if req.LogoURL != nil {
		logo.LogoURL = *req.LogoURL
	}
	if req.ScheduleType != nil {
		logo.ScheduleType = *req.ScheduleType
	}
	if req.RecurringDate != nil {
		logo.RecurringDate = req.RecurringDate
	}
	if req.StartDate != nil {
		logo.StartDate = req.StartDate
	}
	if req.EndDate != nil {
		logo.EndDate = req.EndDate
	}
	if req.Priority != nil {
		logo.Priority = *req.Priority
	}
	if req.IsActive != nil {
		logo.IsActive = *req.IsActive
	}

	if err := h.validateSchedule(logo.ScheduleType, logo.RecurringDate, logo.StartDate, logo.EndDate); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	// default 타입 활성화 시 기존 활성 default 확인
	if logo.ScheduleType == "default" && logo.IsActive {
		count, err := h.logoRepo.CountActiveDefault()
		if err != nil {
			common.V2ErrorResponse(c, http.StatusInternalServerError, "기본 로고 확인 실패", err)
			return
		}
		// 자기 자신 제외
		existingLogo, _ := h.logoRepo.FindByID(id)
		if existingLogo != nil && existingLogo.ScheduleType == "default" && existingLogo.IsActive {
			count--
		}
		if count > 0 {
			common.V2ErrorResponse(c, http.StatusBadRequest, "활성 상태인 기본 로고는 1개만 허용됩니다", nil)
			return
		}
	}

	if err := h.logoRepo.Update(logo); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "로고 수정 실패", err)
		return
	}

	common.V2Success(c, logo)
}

// DeleteLogo handles DELETE /api/v1/admin/logos/:id
func (h *SiteLogoHandler) DeleteLogo(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 로고 ID", err)
		return
	}

	if _, err := h.logoRepo.FindByID(id); err != nil {
		if err == gorm.ErrRecordNotFound {
			common.V2ErrorResponse(c, http.StatusNotFound, "로고를 찾을 수 없습니다", nil)
			return
		}
		common.V2ErrorResponse(c, http.StatusInternalServerError, "로고 조회 실패", err)
		return
	}

	if err := h.logoRepo.Delete(id); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "로고 삭제 실패", err)
		return
	}

	common.V2Success(c, gin.H{"message": "로고가 삭제되었습니다"})
}

// GetActiveLogo handles GET /api/v1/logos/active
func (h *SiteLogoHandler) GetActiveLogo(c *gin.Context) {
	kst := time.FixedZone("KST", 9*60*60)
	now := time.Now().In(kst)
	mmdd := now.Format("01-02")
	today := now.Format("2006-01-02")

	logo, err := h.logoRepo.FindActiveLogo(mmdd, today)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			common.V2Success(c, gin.H{"active": nil, "schedules": []interface{}{}})
			return
		}
		common.V2ErrorResponse(c, http.StatusInternalServerError, "로고 조회 실패", err)
		return
	}

	schedules, err := h.logoRepo.FindAllActive()
	if err != nil {
		schedules = []*v2domain.SiteLogo{}
	}

	common.V2Success(c, gin.H{
		"active":    logo,
		"schedules": schedules,
	})
}

func (h *SiteLogoHandler) hasRecurringPresetConflict(existing []*v2domain.SiteLogo, name, logoURL string) bool {
	for _, logo := range existing {
		if logo.Name == name || logo.LogoURL == logoURL {
			return true
		}
	}
	return false
}

func (h *SiteLogoHandler) validateSchedule(scheduleType string, recurringDate, startDate, endDate *string) error {
	switch scheduleType {
	case "recurring":
		if recurringDate == nil || *recurringDate == "" {
			return &validationError{"recurring 타입은 recurring_date(MM-DD)가 필수입니다"}
		}
		if !mmddRegex.MatchString(*recurringDate) {
			return &validationError{"recurring_date는 MM-DD 형식이어야 합니다 (예: 03-01)"}
		}
	case "date_range":
		if startDate == nil || *startDate == "" || endDate == nil || *endDate == "" {
			return &validationError{"date_range 타입은 start_date와 end_date가 필수입니다"}
		}
		if *startDate > *endDate {
			return &validationError{"start_date는 end_date보다 이전이어야 합니다"}
		}
	case "default":
		// no additional validation
	}
	return nil
}

type validationError struct {
	msg string
}

func (e *validationError) Error() string {
	return e.msg
}
