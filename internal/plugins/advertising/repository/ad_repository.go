package repository

import (
	"regexp"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/advertising/domain"
	"gorm.io/gorm"
)

// extractFirstImage HTML 콘텐츠에서 첫 번째 이미지 URL 추출 (g5_write_message fallback용)
func extractFirstImage(content string) string {
	imgRegex := regexp.MustCompile(`<img[^>]+src=["']([^"']+)["']`)
	matches := imgRegex.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// MemberInfo 멤버 정보 (축하 배너용)
type MemberInfo struct {
	Nickname string
	ImageURL string
}

// AdRepository 광고 저장소 인터페이스
type AdRepository interface {
	// AdUnit CRUD
	CreateAdUnit(unit *domain.AdUnit) error
	UpdateAdUnit(id uint64, unit *domain.AdUnit) error
	DeleteAdUnit(id uint64) error
	FindAdUnitByID(id uint64) (*domain.AdUnit, error)
	FindAdUnitByPosition(position string) (*domain.AdUnit, error)
	ListAdUnits(activeOnly bool) ([]*domain.AdUnit, error)
	ListAdUnitsByType(adType domain.AdType, activeOnly bool) ([]*domain.AdUnit, error)

	// AdRotationConfig CRUD
	CreateRotationConfig(config *domain.AdRotationConfig) error
	UpdateRotationConfig(id uint64, config *domain.AdRotationConfig) error
	DeleteRotationConfig(id uint64) error
	FindRotationConfigByPosition(position string) (*domain.AdRotationConfig, error)
	ListRotationConfigs() ([]*domain.AdRotationConfig, error)

	// CelebrationBanner CRUD
	CreateBanner(banner *domain.CelebrationBanner) error
	UpdateBanner(id uint64, banner *domain.CelebrationBanner) error
	DeleteBanner(id uint64) error
	FindBannerByID(id uint64) (*domain.CelebrationBanner, error)
	ListBanners(activeOnly bool) ([]*domain.CelebrationBanner, error)
	ListBannersByDate(date time.Time) ([]*domain.CelebrationBanner, error)

	// Member lookup (축하 배너용)
	FindMemberByID(memberID string) (*MemberInfo, error)
}

// adRepository GORM 구현체
type adRepository struct {
	db *gorm.DB
}

// NewAdRepository 생성자
func NewAdRepository(db *gorm.DB) AdRepository {
	return &adRepository{db: db}
}

// ============ AdUnit Methods ============

// CreateAdUnit 광고 단위 생성
func (r *adRepository) CreateAdUnit(unit *domain.AdUnit) error {
	now := time.Now()
	unit.CreatedAt = now
	unit.UpdatedAt = now
	return r.db.Create(unit).Error
}

// UpdateAdUnit 광고 단위 수정
func (r *adRepository) UpdateAdUnit(id uint64, unit *domain.AdUnit) error {
	unit.UpdatedAt = time.Now()
	return r.db.Model(&domain.AdUnit{}).Where("id = ?", id).Updates(unit).Error
}

// DeleteAdUnit 광고 단위 삭제
func (r *adRepository) DeleteAdUnit(id uint64) error {
	return r.db.Delete(&domain.AdUnit{}, id).Error
}

// FindAdUnitByID ID로 광고 단위 조회
func (r *adRepository) FindAdUnitByID(id uint64) (*domain.AdUnit, error) {
	var unit domain.AdUnit
	err := r.db.First(&unit, id).Error
	if err != nil {
		return nil, err
	}
	return &unit, nil
}

// FindAdUnitByPosition 위치로 광고 단위 조회
func (r *adRepository) FindAdUnitByPosition(position string) (*domain.AdUnit, error) {
	var unit domain.AdUnit
	err := r.db.Where("position = ? AND is_active = ?", position, true).
		Order("priority DESC").
		First(&unit).Error
	if err != nil {
		return nil, err
	}
	return &unit, nil
}

// ListAdUnits 모든 광고 단위 조회
func (r *adRepository) ListAdUnits(activeOnly bool) ([]*domain.AdUnit, error) {
	var units []*domain.AdUnit
	query := r.db.Order("position ASC, priority DESC")
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	err := query.Find(&units).Error
	return units, err
}

// ListAdUnitsByType 광고 유형별 단위 조회
func (r *adRepository) ListAdUnitsByType(adType domain.AdType, activeOnly bool) ([]*domain.AdUnit, error) {
	var units []*domain.AdUnit
	query := r.db.Where("ad_type = ?", adType).Order("position ASC, priority DESC")
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	err := query.Find(&units).Error
	return units, err
}

// ============ AdRotationConfig Methods ============

// CreateRotationConfig 로테이션 설정 생성
func (r *adRepository) CreateRotationConfig(config *domain.AdRotationConfig) error {
	config.CreatedAt = time.Now()
	return r.db.Create(config).Error
}

// UpdateRotationConfig 로테이션 설정 수정
func (r *adRepository) UpdateRotationConfig(id uint64, config *domain.AdRotationConfig) error {
	return r.db.Model(&domain.AdRotationConfig{}).Where("id = ?", id).Updates(config).Error
}

// DeleteRotationConfig 로테이션 설정 삭제
func (r *adRepository) DeleteRotationConfig(id uint64) error {
	return r.db.Delete(&domain.AdRotationConfig{}, id).Error
}

// FindRotationConfigByPosition 위치별 로테이션 설정 조회
func (r *adRepository) FindRotationConfigByPosition(position string) (*domain.AdRotationConfig, error) {
	var config domain.AdRotationConfig
	err := r.db.Where("position = ?", position).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// ListRotationConfigs 모든 로테이션 설정 조회
func (r *adRepository) ListRotationConfigs() ([]*domain.AdRotationConfig, error) {
	var configs []*domain.AdRotationConfig
	err := r.db.Order("position ASC").Find(&configs).Error
	return configs, err
}

// ============ CelebrationBanner Methods ============

// CreateBanner 배너 생성
func (r *adRepository) CreateBanner(banner *domain.CelebrationBanner) error {
	banner.CreatedAt = time.Now()
	return r.db.Create(banner).Error
}

// UpdateBanner 배너 수정
func (r *adRepository) UpdateBanner(id uint64, banner *domain.CelebrationBanner) error {
	return r.db.Model(&domain.CelebrationBanner{}).Where("id = ?", id).Updates(banner).Error
}

// DeleteBanner 배너 삭제
func (r *adRepository) DeleteBanner(id uint64) error {
	return r.db.Delete(&domain.CelebrationBanner{}, id).Error
}

// FindBannerByID ID로 배너 조회
func (r *adRepository) FindBannerByID(id uint64) (*domain.CelebrationBanner, error) {
	var banner domain.CelebrationBanner
	err := r.db.First(&banner, id).Error
	if err != nil {
		return nil, err
	}
	return &banner, nil
}

// ListBanners 모든 배너 조회
func (r *adRepository) ListBanners(activeOnly bool) ([]*domain.CelebrationBanner, error) {
	var banners []*domain.CelebrationBanner
	query := r.db.Order("display_date DESC, id DESC")
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	err := query.Find(&banners).Error
	return banners, err
}

// ListBannersByDate celebration_banners 테이블 우선 조회, 없으면 g5_write_message fallback
func (r *adRepository) ListBannersByDate(date time.Time) ([]*domain.CelebrationBanner, error) {
	var banners []*domain.CelebrationBanner

	dateFmt := date.Format("2006-01-02")
	month := int(date.Month())
	day := date.Day()

	// 1차: celebration_banners 테이블
	err := r.db.Where("is_active = ?", true).
		Where(
			r.db.Where("display_date = ?", dateFmt).
				Or("yearly_repeat = ? AND MONTH(display_date) = ? AND DAY(display_date) = ?",
					true, month, day),
		).
		Order("sort_order ASC, id DESC").
		Find(&banners).Error

	if err != nil {
		return nil, err
	}

	// celebration_banners에 데이터가 있으면 바로 반환
	if len(banners) > 0 {
		return banners, nil
	}

	// 2차: g5_write_message fallback (마이그레이션 전까지)
	dateDot := date.Format("2006.01.02")

	type MessageRow struct {
		WrID      int    `gorm:"column:wr_id"`
		WrSubject string `gorm:"column:wr_subject"`
		WrContent string `gorm:"column:wr_content"`
		WrLink1   string `gorm:"column:wr_link1"`
		WrLink2   string `gorm:"column:wr_link2"`
		MbID      string `gorm:"column:mb_id"`
	}

	var results []MessageRow
	err = r.db.Table("g5_write_message").
		Select("wr_id, wr_subject, wr_content, wr_link1, wr_link2, mb_id").
		Where("wr_is_comment = 0 AND (wr_subject = ? OR wr_subject = ?)", dateDot, dateFmt).
		Order("wr_id DESC").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	for _, row := range results {
		imageURL := extractFirstImage(row.WrContent)
		// #nosec G115 - WrID는 DB primary key로 항상 양수
		banner := &domain.CelebrationBanner{
			ID:             uint64(row.WrID), //nolint:gosec
			Title:          row.WrSubject,
			Content:        row.WrContent,
			ImageURL:       imageURL,
			LinkURL:        row.WrLink1,
			ExternalURL:    row.WrLink2,
			DisplayDate:    date,
			TargetMemberID: row.MbID,
			IsActive:       true,
		}
		banners = append(banners, banner)
	}

	return banners, nil
}

// FindMemberByID 회원 ID로 닉네임과 프로필 사진 조회
func (r *adRepository) FindMemberByID(memberID string) (*MemberInfo, error) {
	if memberID == "" {
		return nil, nil
	}

	type memberRow struct {
		Nickname string `gorm:"column:mb_nick"`
		ImageURL string `gorm:"column:mb_image_url"`
	}

	var row memberRow
	err := r.db.Table("g5_member").
		Select("mb_nick, mb_image_url").
		Where("mb_id = ?", memberID).
		First(&row).Error

	if err != nil {
		return nil, err
	}

	// 이미지 URL이 상대 경로인 경우 전체 URL로 변환
	imageURL := row.ImageURL
	if imageURL != "" && !regexp.MustCompile(`^https?://`).MatchString(imageURL) {
		imageURL = "https://damoang.net/" + imageURL
	}

	return &MemberInfo{
		Nickname: row.Nickname,
		ImageURL: imageURL,
	}, nil
}
