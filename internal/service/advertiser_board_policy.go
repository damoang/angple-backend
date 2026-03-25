package service

import (
	"fmt"
	"strings"
	"time"

	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"gorm.io/gorm"
)

const AdvertiserPolicyModeShadow = "shadow"

type advertiserPromotionRow struct {
	PermissionGroup string `gorm:"column:permission_group"`
}

// AdvertiserBoardPolicyService evaluates board-specific advertiser policy.
// The service is designed to be safe-by-default:
// - no policy row => no effect
// - disabled policy => no effect
// - current rollout uses shadow mode only
type AdvertiserBoardPolicyService struct {
	db   *gorm.DB
	repo v2repo.AdvertiserBoardPolicyRepository
}

func NewAdvertiserBoardPolicyService(db *gorm.DB, repo v2repo.AdvertiserBoardPolicyRepository) *AdvertiserBoardPolicyService {
	return &AdvertiserBoardPolicyService{db: db, repo: repo}
}

func (s *AdvertiserBoardPolicyService) EvaluateWrite(boardSlug, memberID string) (*v2domain.V2AdvertiserBoardPolicy, *WriteRestrictionResult, error) {
	policy, err := s.repo.FindByBoardSlug(boardSlug)
	if err != nil {
		return nil, nil, fmt.Errorf("load advertiser board policy: %w", err)
	}

	if policy == nil || !policy.Enabled {
		return policy, nil, nil
	}

	if !policy.AllowActiveAdvertiserWrite {
		return policy, &WriteRestrictionResult{
			CanWrite: false,
			Reason:   "광고주 정책상 글쓰기가 허용되지 않습니다.",
		}, nil
	}

	promotion, err := s.findActivePromotion(memberID)
	if err != nil {
		return nil, nil, fmt.Errorf("load active promotion: %w", err)
	}
	if promotion == nil {
		return policy, &WriteRestrictionResult{
			CanWrite: false,
			Reason:   "활성 광고주만 글을 작성할 수 있습니다.",
		}, nil
	}

	dailyLimit := policy.DailyPostLimit
	if dailyLimit <= 0 {
		dailyLimit = permissionGroupDailyLimit(promotion.PermissionGroup)
	}

	if dailyLimit <= 0 {
		return policy, &WriteRestrictionResult{CanWrite: true, Remaining: -1, DailyLimit: 0}, nil
	}

	todayCount, err := countTodayPostsForBoard(s.db, boardSlug, memberID)
	if err != nil {
		return nil, nil, fmt.Errorf("count advertiser daily posts: %w", err)
	}

	if todayCount >= dailyLimit {
		return policy, &WriteRestrictionResult{
			CanWrite:   false,
			Remaining:  0,
			DailyLimit: dailyLimit,
			Reason:     fmt.Sprintf("오늘 %d개까지 작성 가능합니다. (이미 %d개 작성)", dailyLimit, todayCount),
		}, nil
	}

	return policy, &WriteRestrictionResult{
		CanWrite:   true,
		Remaining:  dailyLimit - todayCount,
		DailyLimit: dailyLimit,
	}, nil
}

func (s *AdvertiserBoardPolicyService) findActivePromotion(memberID string) (*advertiserPromotionRow, error) {
	if memberID == "" {
		return nil, nil
	}

	kst := time.FixedZone("KST", 9*60*60)
	today := time.Now().In(kst).Format("2006-01-02")

	var row advertiserPromotionRow
	err := s.db.Table("promotions").
		Select("permission_group").
		Where("member_id = ? AND is_active = ? AND start_date <= ? AND end_date >= ?", memberID, true, today, today).
		Limit(1).
		Take(&row).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func permissionGroupDailyLimit(group string) int {
	switch strings.ToLower(strings.TrimSpace(group)) {
	case "three":
		return 3
	case "two":
		return 2
	case "one", "":
		return 1
	default:
		return 0
	}
}

func countTodayPostsForBoard(db *gorm.DB, boardSlug, memberID string) (int, error) {
	tableName := fmt.Sprintf("g5_write_%s", boardSlug)
	kst := time.FixedZone("KST", 9*60*60)
	today := time.Now().In(kst).Format("2006-01-02")

	var count int64
	err := db.Table(tableName).
		Where("mb_id = ? AND DATE(wr_datetime) = ? AND wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')", memberID, today).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}
