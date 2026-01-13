package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrSiteNotFound     = errors.New("site not found")
	ErrSubdomainTaken   = errors.New("subdomain already taken")
	ErrInvalidSubdomain = errors.New("invalid subdomain format")
	ErrSiteInactive     = errors.New("site is inactive")
	ErrSiteSuspended    = errors.New("site is suspended")
	ErrUnauthorized     = errors.New("unauthorized access")
	ErrInvalidPlan      = errors.New("invalid plan")
)

type SiteService struct {
	repo *repository.SiteRepository
}

func NewSiteService(repo *repository.SiteRepository) *SiteService {
	return &SiteService{repo: repo}
}

// ========================================
// Public Methods
// ========================================

// GetBySubdomain retrieves a site by subdomain with settings
func (s *SiteService) GetBySubdomain(ctx context.Context, subdomain string) (*domain.SiteResponse, error) {
	site, err := s.repo.FindBySubdomain(ctx, subdomain)
	if err != nil {
		return nil, fmt.Errorf("failed to find site: %w", err)
	}

	if site == nil {
		return nil, ErrSiteNotFound
	}

	// Get settings
	settings, err := s.repo.FindSettingsBySiteID(ctx, site.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find settings: %w", err)
	}

	return site.ToResponse(settings), nil
}

// GetByID retrieves a site by ID with settings
func (s *SiteService) GetByID(ctx context.Context, siteID string) (*domain.SiteResponse, error) {
	site, err := s.repo.FindByID(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to find site: %w", err)
	}

	if site == nil {
		return nil, ErrSiteNotFound
	}

	settings, err := s.repo.FindSettingsBySiteID(ctx, site.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to find settings: %w", err)
	}

	return site.ToResponse(settings), nil
}

// Create creates a new site with initial settings
func (s *SiteService) Create(ctx context.Context, req *domain.CreateSiteRequest) (*domain.SiteResponse, error) {
	// 1. Validate subdomain format
	if !s.ValidateSubdomain(req.Subdomain) {
		return nil, ErrInvalidSubdomain
	}

	// 2. Check subdomain availability
	available, err := s.repo.CheckSubdomainAvailability(ctx, req.Subdomain)
	if err != nil {
		return nil, fmt.Errorf("failed to check subdomain: %w", err)
	}
	if !available {
		return nil, ErrSubdomainTaken
	}

	// 3. Validate plan
	if !s.isValidPlan(req.Plan) {
		return nil, ErrInvalidPlan
	}

	// 4. Determine DB strategy based on plan
	dbStrategy := s.getDBStrategyByPlan(req.Plan)

	// 5. Create site
	siteID := uuid.New().String()
	site := &domain.Site{
		ID:         siteID,
		Subdomain:  strings.ToLower(req.Subdomain),
		SiteName:   req.SiteName,
		OwnerEmail: req.OwnerEmail,
		Plan:       req.Plan,
		DBStrategy: dbStrategy,
		Active:     true,
		Suspended:  false,
	}

	if err := s.repo.Create(ctx, site); err != nil {
		return nil, fmt.Errorf("failed to create site: %w", err)
	}

	// 6. Create initial settings
	activeTheme := "damoang-official"
	if req.ActiveTheme != nil {
		activeTheme = *req.ActiveTheme
	}

	primaryColor := "#3b82f6"
	if req.PrimaryColor != nil {
		primaryColor = *req.PrimaryColor
	}

	secondaryColor := "#8b5cf6"
	if req.SecondaryColor != nil {
		secondaryColor = *req.SecondaryColor
	}

	settings := &domain.SiteSettings{
		SiteID:         siteID,
		ActiveTheme:    activeTheme,
		PrimaryColor:   primaryColor,
		SecondaryColor: secondaryColor,
		SSLEnabled:     true,
	}

	if err := s.repo.CreateSettings(ctx, settings); err != nil {
		return nil, fmt.Errorf("failed to create settings: %w", err)
	}

	return site.ToResponse(settings), nil
}

// GetSettings retrieves site settings
func (s *SiteService) GetSettings(ctx context.Context, siteID string) (*domain.SiteSettings, error) {
	settings, err := s.repo.FindSettingsBySiteID(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to find settings: %w", err)
	}

	if settings == nil {
		return nil, ErrSiteNotFound
	}

	return settings, nil
}

// UpdateSettings updates site settings
func (s *SiteService) UpdateSettings(ctx context.Context, siteID string, req *domain.UpdateSiteSettingsRequest) error {
	// Get existing settings
	settings, err := s.repo.FindSettingsBySiteID(ctx, siteID)
	if err != nil {
		return fmt.Errorf("failed to find settings: %w", err)
	}

	if settings == nil {
		return ErrSiteNotFound
	}

	// Update fields
	if req.ActiveTheme != nil {
		settings.ActiveTheme = *req.ActiveTheme
	}
	if req.LogoURL != nil {
		settings.LogoURL = req.LogoURL
	}
	if req.FaviconURL != nil {
		settings.FaviconURL = req.FaviconURL
	}
	if req.PrimaryColor != nil {
		settings.PrimaryColor = *req.PrimaryColor
	}
	if req.SecondaryColor != nil {
		settings.SecondaryColor = *req.SecondaryColor
	}
	if req.SiteDescription != nil {
		settings.SiteDescription = req.SiteDescription
	}
	if req.SiteKeywords != nil {
		settings.SiteKeywords = req.SiteKeywords
	}
	if req.GoogleAnalyticsID != nil {
		settings.GoogleAnalyticsID = req.GoogleAnalyticsID
	}
	if req.CustomDomain != nil {
		settings.CustomDomain = req.CustomDomain
	}

	return s.repo.UpdateSettings(ctx, settings)
}

// ListActive retrieves all active sites
func (s *SiteService) ListActive(ctx context.Context, limit, offset int) ([]domain.SiteResponse, error) {
	sites, err := s.repo.ListActive(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list sites: %w", err)
	}

	responses := make([]domain.SiteResponse, 0, len(sites))
	for _, site := range sites {
		settings, err := s.repo.FindSettingsBySiteID(ctx, site.ID)
		if err != nil {
			// Settings not found is acceptable, continue with nil settings
			settings = nil
		}
		resp := site.ToResponse(settings)
		responses = append(responses, *resp)
	}

	return responses, nil
}

// ========================================
// User Permission Methods
// ========================================

// AddOwnerPermission adds owner permission to site creator
func (s *SiteService) AddOwnerPermission(ctx context.Context, siteID, userID string) error {
	siteUser := &domain.SiteUser{
		SiteID: siteID,
		UserID: userID,
		Role:   "owner",
	}

	return s.repo.AddUserPermission(ctx, siteUser)
}

// CheckUserPermission checks if user has permission for a site
func (s *SiteService) CheckUserPermission(ctx context.Context, siteID, userID string, requiredRole string) (bool, error) {
	perm, err := s.repo.FindUserPermission(ctx, siteID, userID)
	if err != nil {
		return false, err
	}

	if perm == nil {
		return false, nil
	}

	// Role hierarchy: owner > admin > editor > viewer
	roleLevel := map[string]int{
		"owner":  4,
		"admin":  3,
		"editor": 2,
		"viewer": 1,
	}

	return roleLevel[perm.Role] >= roleLevel[requiredRole], nil
}

// ========================================
// Validation Helpers
// ========================================

// ValidateSubdomain checks if subdomain follows rules:
// - 3-50 characters
// - alphanumeric and hyphens only
// - cannot start/end with hyphen
// - reserved subdomains blocked
func (s *SiteService) ValidateSubdomain(subdomain string) bool {
	// Length check
	if len(subdomain) < 3 || len(subdomain) > 50 {
		return false
	}

	// Format check (alphanumeric and hyphens, no start/end hyphen)
	matched, err := regexp.MatchString(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`, subdomain)
	if err != nil || !matched {
		return false
	}

	// Reserved subdomains
	reserved := []string{
		"www", "api", "admin", "app", "mail", "ftp",
		"blog", "shop", "store", "cdn", "static",
		"assets", "media", "images", "files", "docs",
		"support", "help", "status", "dashboard",
		"angple", "damoang", "test", "demo", "staging",
	}

	for _, r := range reserved {
		if subdomain == r {
			return false
		}
	}

	return true
}

// isValidPlan checks if plan is valid
func (s *SiteService) isValidPlan(plan string) bool {
	validPlans := []string{"free", "pro", "business", "enterprise"}
	for _, p := range validPlans {
		if plan == p {
			return true
		}
	}
	return false
}

// getDBStrategyByPlan determines DB strategy based on plan
func (s *SiteService) getDBStrategyByPlan(plan string) string {
	switch plan {
	case "free":
		return "shared"
	case "pro", "business":
		return "schema"
	case "enterprise":
		return "dedicated"
	default:
		return "shared"
	}
}
