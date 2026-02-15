package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DB strategy and plan constants
const (
	dbStrategySchema    = "schema"
	dbStrategyShared    = "shared"
	dbStrategyDedicated = "dedicated"
	planFree            = "free"
	planPro             = "pro"
	planBusiness        = "business"
	planEnterprise      = "enterprise"
)

// ProvisioningService handles one-click community creation and subscription management
type ProvisioningService struct {
	siteRepo   *repository.SiteRepository
	subRepo    *repository.SubscriptionRepository
	dbResolver *middleware.TenantDBResolver
	db         *gorm.DB
	baseDomain string // e.g. "angple.com"
}

// NewProvisioningService creates a new ProvisioningService
func NewProvisioningService(
	siteRepo *repository.SiteRepository,
	subRepo *repository.SubscriptionRepository,
	dbResolver *middleware.TenantDBResolver,
	db *gorm.DB,
	baseDomain string,
) *ProvisioningService {
	return &ProvisioningService{
		siteRepo:   siteRepo,
		subRepo:    subRepo,
		dbResolver: dbResolver,
		db:         db,
		baseDomain: baseDomain,
	}
}

// planPricing holds pricing configuration
var planPricing = map[string]domain.PlanPricing{
	planFree:       {Plan: planFree, MonthlyKRW: 0, YearlyKRW: 0, TrialDays: 0},
	planPro:        {Plan: planPro, MonthlyKRW: 29000, YearlyKRW: 290000, TrialDays: 14},
	planBusiness:   {Plan: planBusiness, MonthlyKRW: 99000, YearlyKRW: 990000, TrialDays: 14},
	planEnterprise: {Plan: planEnterprise, MonthlyKRW: 299000, YearlyKRW: 2990000, TrialDays: 30},
}

// ProvisionCommunity creates a new community site with all required infrastructure
//
//nolint:gocyclo // orchestration function with necessary sequential steps
func (s *ProvisioningService) ProvisionCommunity(ctx context.Context, req *domain.ProvisionRequest) (*domain.ProvisionResponse, error) {
	// 1. Check subdomain availability
	available, err := s.siteRepo.CheckSubdomainAvailability(ctx, req.Subdomain)
	if err != nil {
		return nil, fmt.Errorf("서브도메인 확인 실패: %w", err)
	}
	if !available {
		return nil, errors.New("이미 사용 중인 서브도메인입니다")
	}

	// 2. Check owner site limit (max 5 per email)
	count, err := s.siteRepo.CountSitesByOwner(ctx, req.OwnerEmail)
	if err != nil {
		return nil, fmt.Errorf("사이트 수 확인 실패: %w", err)
	}
	if count >= 5 {
		return nil, errors.New("최대 5개의 사이트만 생성 가능합니다")
	}

	// 3. Determine DB strategy by plan
	dbStrategy := getDBStrategyByPlan(req.Plan)
	siteID := uuid.New().String()

	// 4. Create site
	site := &domain.Site{
		ID:         siteID,
		Subdomain:  req.Subdomain,
		SiteName:   req.SiteName,
		OwnerEmail: req.OwnerEmail,
		Plan:       req.Plan,
		DBStrategy: dbStrategy,
		Active:     true,
	}

	// Set trial period for paid plans
	pricing := planPricing[req.Plan]
	if pricing.TrialDays > 0 {
		trialEnd := time.Now().AddDate(0, 0, pricing.TrialDays)
		site.TrialEndsAt = &trialEnd
	}

	if err := s.siteRepo.Create(ctx, site); err != nil {
		return nil, fmt.Errorf("사이트 생성 실패: %w", err)
	}

	// 5. Create site settings
	theme := "damoang-official"
	if req.Theme != nil && *req.Theme != "" {
		theme = *req.Theme
	}
	settings := &domain.SiteSettings{
		SiteID:      siteID,
		ActiveTheme: theme,
		SSLEnabled:  true,
	}
	if err := s.siteRepo.CreateSettings(ctx, settings); err != nil {
		return nil, fmt.Errorf("사이트 설정 생성 실패: %w", err)
	}

	// 6. Create owner permission
	siteUser := &domain.SiteUser{
		SiteID: siteID,
		UserID: req.OwnerEmail,
		Role:   "owner",
	}
	if err := s.siteRepo.AddUserPermission(ctx, siteUser); err != nil {
		return nil, fmt.Errorf("소유자 권한 설정 실패: %w", err)
	}

	// 7. Create schema if needed
	if dbStrategy == dbStrategySchema && s.dbResolver != nil {
		schemaName := fmt.Sprintf("tenant_%s", siteID[:8])
		if err := s.dbResolver.CreateSchema(schemaName); err != nil {
			return nil, fmt.Errorf("스키마 생성 실패: %w", err)
		}
		site.DBSchemaName = &schemaName
		if err := s.siteRepo.Update(ctx, site); err != nil {
			return nil, fmt.Errorf("사이트 스키마 업데이트 실패: %w", err)
		}
	}

	// 8. Create subscription
	now := time.Now()
	sub := &domain.Subscription{
		SiteID:             siteID,
		Plan:               req.Plan,
		BillingCycle:       "monthly",
		MonthlyPriceKRW:    pricing.MonthlyKRW,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
	}
	if pricing.TrialDays > 0 {
		sub.Status = "trialing"
		sub.CurrentPeriodEnd = now.AddDate(0, 0, pricing.TrialDays)
	} else {
		sub.Status = paymentStatusActive
	}
	if err := s.subRepo.Create(ctx, sub); err != nil {
		return nil, fmt.Errorf("구독 생성 실패: %w", err)
	}

	resp := &domain.ProvisionResponse{
		SiteID:     siteID,
		Subdomain:  req.Subdomain,
		SiteURL:    fmt.Sprintf("https://%s.%s", req.Subdomain, s.baseDomain),
		Plan:       req.Plan,
		DBStrategy: dbStrategy,
		Message:    "커뮤니티가 생성되었습니다",
	}
	if site.TrialEndsAt != nil {
		resp.TrialEndsAt = site.TrialEndsAt.Format(time.RFC3339)
	}

	return resp, nil
}

// GetSubscription returns the subscription for a site
func (s *ProvisioningService) GetSubscription(ctx context.Context, siteID string) (*domain.SubscriptionResponse, error) {
	sub, err := s.subRepo.FindBySiteID(ctx, siteID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, errors.New("구독 정보를 찾을 수 없습니다")
	}

	resp := &domain.SubscriptionResponse{
		Plan:               sub.Plan,
		Status:             sub.Status,
		BillingCycle:       sub.BillingCycle,
		MonthlyPriceKRW:    sub.MonthlyPriceKRW,
		CurrentPeriodStart: sub.CurrentPeriodStart.Format(time.RFC3339),
		CurrentPeriodEnd:   sub.CurrentPeriodEnd.Format(time.RFC3339),
	}
	if sub.CanceledAt != nil {
		resp.CanceledAt = sub.CanceledAt.Format(time.RFC3339)
	}
	return resp, nil
}

// ChangePlan upgrades or downgrades the subscription
func (s *ProvisioningService) ChangePlan(ctx context.Context, siteID string, req *domain.ChangePlanRequest) error {
	sub, err := s.subRepo.FindBySiteID(ctx, siteID)
	if err != nil || sub == nil {
		return errors.New("구독 정보를 찾을 수 없습니다")
	}

	if sub.Plan == req.Plan {
		return errors.New("현재와 동일한 플랜입니다")
	}

	pricing, ok := planPricing[req.Plan]
	if !ok {
		return fmt.Errorf("유효하지 않은 플랜: %s", req.Plan)
	}

	// Update subscription
	sub.Plan = req.Plan
	sub.MonthlyPriceKRW = pricing.MonthlyKRW
	if req.BillingCycle != "" {
		sub.BillingCycle = req.BillingCycle
	}

	if err := s.subRepo.Update(ctx, sub); err != nil {
		return err
	}

	// Update site plan and DB strategy
	site, err := s.siteRepo.FindByID(ctx, siteID)
	if err != nil || site == nil {
		return errors.New("사이트를 찾을 수 없습니다")
	}

	newStrategy := getDBStrategyByPlan(req.Plan)
	oldStrategy := site.DBStrategy
	site.Plan = req.Plan
	site.DBStrategy = newStrategy

	if err := s.siteRepo.Update(ctx, site); err != nil {
		return err
	}

	// Create schema if upgrading from shared to schema
	if oldStrategy == dbStrategyShared && newStrategy == dbStrategySchema && s.dbResolver != nil {
		schemaName := fmt.Sprintf("tenant_%s", siteID[:8])
		if err := s.dbResolver.CreateSchema(schemaName); err != nil {
			return fmt.Errorf("스키마 생성 실패: %w", err)
		}
		site.DBSchemaName = &schemaName
		if err := s.siteRepo.Update(ctx, site); err != nil {
			return fmt.Errorf("사이트 스키마 업데이트 실패: %w", err)
		}
	}

	return nil
}

// CancelSubscription cancels the subscription at period end
func (s *ProvisioningService) CancelSubscription(ctx context.Context, siteID string) error {
	sub, err := s.subRepo.FindBySiteID(ctx, siteID)
	if err != nil || sub == nil {
		return errors.New("구독 정보를 찾을 수 없습니다")
	}
	if sub.Status == paymentStatusCanceled {
		return errors.New("이미 취소된 구독입니다")
	}

	now := time.Now()
	sub.CanceledAt = &now
	sub.Status = paymentStatusCanceled
	return s.subRepo.Update(ctx, sub)
}

// GetInvoices returns invoices for a site
func (s *ProvisioningService) GetInvoices(ctx context.Context, siteID string, page, perPage int) ([]domain.Invoice, int64, error) {
	offset := (page - 1) * perPage
	return s.subRepo.ListInvoices(ctx, siteID, perPage, offset)
}

// GetPricing returns plan pricing info
func (s *ProvisioningService) GetPricing() []domain.PlanPricing {
	return []domain.PlanPricing{
		planPricing[planFree],
		planPricing[planPro],
		planPricing[planBusiness],
		planPricing[planEnterprise],
	}
}

// DeleteCommunity deactivates a community and its subscription
func (s *ProvisioningService) DeleteCommunity(ctx context.Context, siteID string) error {
	site, err := s.siteRepo.FindByID(ctx, siteID)
	if err != nil || site == nil {
		return errors.New("사이트를 찾을 수 없습니다")
	}

	// Cancel subscription
	sub, err := s.subRepo.FindBySiteID(ctx, siteID)
	if err != nil {
		return fmt.Errorf("구독 조회 실패: %w", err)
	}
	if sub != nil && sub.Status != paymentStatusCanceled {
		now := time.Now()
		sub.CanceledAt = &now
		sub.Status = paymentStatusCanceled
		if err := s.subRepo.Update(ctx, sub); err != nil {
			return fmt.Errorf("구독 취소 업데이트 실패: %w", err)
		}
	}

	// Deactivate site
	return s.siteRepo.Delete(ctx, siteID)
}

func getDBStrategyByPlan(plan string) string {
	switch plan {
	case planFree:
		return dbStrategyShared
	case planPro, planBusiness:
		return dbStrategySchema
	case planEnterprise:
		return dbStrategyDedicated
	default:
		return dbStrategyShared
	}
}
