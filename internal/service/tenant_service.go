package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/repository"
	"gorm.io/gorm"
)

// TenantService handles tenant management business logic
type TenantService struct {
	siteRepo   *repository.SiteRepository
	db         *gorm.DB
	dbResolver *middleware.TenantDBResolver
}

// NewTenantService creates a new TenantService
func NewTenantService(siteRepo *repository.SiteRepository, db *gorm.DB, dbResolver *middleware.TenantDBResolver) *TenantService {
	return &TenantService{
		siteRepo:   siteRepo,
		db:         db,
		dbResolver: dbResolver,
	}
}

// TenantDetail tenant detail response
type TenantDetail struct {
	Site     *domain.SiteResponse  `json:"site"`
	Settings *domain.SiteSettings  `json:"settings"`
	Limits   middleware.PlanLimits `json:"limits"`
	Users    []domain.SiteUser     `json:"users"`
}

// ListTenants lists tenants with optional status filter
func (s *TenantService) ListTenants(ctx context.Context, page, perPage int, status string) ([]domain.SiteResponse, int64, error) {
	var sites []domain.Site
	var total int64

	query := s.db.WithContext(ctx).Model(&domain.Site{})
	switch status {
	case "active":
		query = query.Where("active = ? AND suspended = ?", true, false)
	case "suspended":
		query = query.Where("suspended = ?", true)
	case "inactive":
		query = query.Where("active = ?", false)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("created_at DESC").
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&sites).Error
	if err != nil {
		return nil, 0, err
	}

	responses := make([]domain.SiteResponse, len(sites))
	for i, site := range sites {
		settings, err := s.siteRepo.FindSettingsBySiteID(ctx, site.ID)
		if err != nil {
			log.Printf("warning: failed to get settings for site %s: %v", site.ID, err)
		}
		responses[i] = *site.ToResponse(settings)
	}
	return responses, total, nil
}

// GetTenantDetail returns full tenant details with limits and users
func (s *TenantService) GetTenantDetail(ctx context.Context, siteID string) (*TenantDetail, error) {
	site, err := s.siteRepo.FindByID(ctx, siteID)
	if err != nil || site == nil {
		return nil, errors.New("테넌트를 찾을 수 없습니다")
	}

	settings, err := s.siteRepo.FindSettingsBySiteID(ctx, siteID)
	if err != nil {
		log.Printf("warning: failed to get settings for site %s: %v", siteID, err)
	}
	users, err := s.siteRepo.ListSiteUsers(ctx, siteID)
	if err != nil {
		log.Printf("warning: failed to list users for site %s: %v", siteID, err)
	}
	limits := middleware.GetPlanLimits(site.Plan)

	return &TenantDetail{
		Site:     site.ToResponse(settings),
		Settings: settings,
		Limits:   limits,
		Users:    users,
	}, nil
}

// SuspendTenant suspends a tenant
func (s *TenantService) SuspendTenant(ctx context.Context, siteID, _ string) error {
	site, err := s.siteRepo.FindByID(ctx, siteID)
	if err != nil || site == nil {
		return errors.New("테넌트를 찾을 수 없습니다")
	}
	if site.Suspended {
		return errors.New("이미 정지된 테넌트입니다")
	}

	site.Suspended = true
	return s.siteRepo.Update(ctx, site)
}

// UnsuspendTenant unsuspends a tenant
func (s *TenantService) UnsuspendTenant(ctx context.Context, siteID string) error {
	site, err := s.siteRepo.FindByID(ctx, siteID)
	if err != nil || site == nil {
		return errors.New("테넌트를 찾을 수 없습니다")
	}
	if !site.Suspended {
		return errors.New("정지 상태가 아닌 테넌트입니다")
	}

	site.Suspended = false
	return s.siteRepo.Update(ctx, site)
}

// ChangePlan changes a tenant's subscription plan
func (s *TenantService) ChangePlan(ctx context.Context, siteID, newPlan string) error {
	validPlans := map[string]bool{"free": true, "pro": true, "business": true, "enterprise": true}
	if !validPlans[newPlan] {
		return fmt.Errorf("유효하지 않은 플랜: %s", newPlan)
	}

	site, err := s.siteRepo.FindByID(ctx, siteID)
	if err != nil || site == nil {
		return errors.New("테넌트를 찾을 수 없습니다")
	}

	oldPlan := site.Plan
	if oldPlan == newPlan {
		return errors.New("현재와 동일한 플랜입니다")
	}

	// DB 전략 변경 (플랜에 따라)
	oldStrategy := site.DBStrategy
	newStrategy := s.getDBStrategyByPlan(newPlan)
	site.Plan = newPlan
	site.DBStrategy = newStrategy

	if err := s.siteRepo.Update(ctx, site); err != nil {
		return err
	}

	// schema 전략으로 업그레이드 시 스키마 생성
	if oldStrategy == dbStrategyShared && newStrategy == dbStrategySchema && s.dbResolver != nil {
		schemaName := fmt.Sprintf("tenant_%s", siteID)
		if err := s.dbResolver.CreateSchema(schemaName); err != nil {
			return fmt.Errorf("스키마 생성 실패: %w", err)
		}
		// DB schema name 저장
		site.DBSchemaName = &schemaName
		return s.siteRepo.Update(ctx, site)
	}

	return nil
}

// UsageStats represents usage statistics for a tenant
type UsageStats struct {
	SiteID          string             `json:"site_id"`
	Period          string             `json:"period"`
	TotalPageViews  int64              `json:"total_page_views"`
	TotalVisitors   int64              `json:"total_unique_visitors"`
	TotalAPICalls   int64              `json:"total_api_calls"`
	TotalPosts      int64              `json:"total_posts_created"`
	TotalComments   int64              `json:"total_comments_created"`
	StorageUsedMB   float64            `json:"storage_used_mb"`
	BandwidthUsedMB float64            `json:"bandwidth_used_mb"`
	DailyUsage      []domain.SiteUsage `json:"daily_usage"`
}

// GetUsage returns usage stats for a tenant over the given number of days
func (s *TenantService) GetUsage(ctx context.Context, siteID string, days int) (*UsageStats, error) {
	site, err := s.siteRepo.FindByID(ctx, siteID)
	if err != nil || site == nil {
		return nil, errors.New("테넌트를 찾을 수 없습니다")
	}

	since := time.Now().AddDate(0, 0, -days)
	var usages []domain.SiteUsage
	err = s.db.WithContext(ctx).
		Where("site_id = ? AND date >= ?", siteID, since).
		Order("date ASC").
		Find(&usages).Error
	if err != nil {
		return nil, err
	}

	stats := &UsageStats{
		SiteID:     siteID,
		Period:     fmt.Sprintf("last_%d_days", days),
		DailyUsage: usages,
	}
	for _, u := range usages {
		stats.TotalPageViews += int64(u.PageViews)
		stats.TotalVisitors += int64(u.UniqueVisitors)
		stats.TotalAPICalls += int64(u.APICalls)
		stats.TotalPosts += int64(u.PostsCreated)
		stats.TotalComments += int64(u.CommentsCreated)
		stats.StorageUsedMB += u.StorageUsedMB
		stats.BandwidthUsedMB += u.BandwidthUsedMB
	}

	return stats, nil
}

func (s *TenantService) getDBStrategyByPlan(plan string) string {
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
