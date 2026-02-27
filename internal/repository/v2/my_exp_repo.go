package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// ExpSummary 경험치 요약 응답
type ExpSummary struct {
	TotalExp     int `json:"total_exp"`
	CurrentLevel int `json:"current_level"`
	NextLevelExp int `json:"next_level_exp"`
	LevelProgress int `json:"level_progress"`
}

// MyExpRepository g5_na_xp + g5_member 기반 경험치 조회
type MyExpRepository interface {
	GetSummary(mbID string) (*ExpSummary, error)
	GetHistory(mbID string, page, limit int) ([]v2.G5NaXp, int64, error)
}

type myExpRepository struct {
	db *gorm.DB
}

// NewMyExpRepository creates a new MyExpRepository
func NewMyExpRepository(db *gorm.DB) MyExpRepository {
	return &myExpRepository{db: db}
}

func (r *myExpRepository) GetSummary(mbID string) (*ExpSummary, error) {
	var member struct {
		AsExp   int
		AsLevel int
		AsMax   int
	}
	err := r.db.Table("g5_member").
		Select("COALESCE(as_exp, 0) as as_exp, COALESCE(as_level, 0) as as_level, COALESCE(as_max, 0) as as_max").
		Where("mb_id = ?", mbID).
		Scan(&member).Error
	if err != nil {
		return nil, err
	}

	// 레벨 진행률 계산
	var levelProgress int
	if member.AsMax > 0 {
		levelProgress = member.AsExp * 100 / member.AsMax
		if levelProgress > 100 {
			levelProgress = 100
		}
	}

	return &ExpSummary{
		TotalExp:      member.AsExp,
		CurrentLevel:  member.AsLevel,
		NextLevelExp:  member.AsMax,
		LevelProgress: levelProgress,
	}, nil
}

func (r *myExpRepository) GetHistory(mbID string, page, limit int) ([]v2.G5NaXp, int64, error) {
	var total int64
	err := r.db.Model(&v2.G5NaXp{}).Where("mb_id = ?", mbID).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	var items []v2.G5NaXp
	offset := (page - 1) * limit
	err = r.db.Where("mb_id = ?", mbID).
		Order("xp_id DESC").
		Offset(offset).
		Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}
