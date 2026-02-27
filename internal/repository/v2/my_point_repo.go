package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// PointSummary 포인트 요약 응답
type PointSummary struct {
	TotalPoint  int `json:"total_point"`
	TotalEarned int `json:"total_earned"`
	TotalUsed   int `json:"total_used"`
}

// MyPointRepository g5_point + g5_member 기반 포인트 조회
type MyPointRepository interface {
	GetSummary(mbID string) (*PointSummary, error)
	GetHistory(mbID string, page, limit int) ([]v2.G5Point, int64, error)
}

type myPointRepository struct {
	db *gorm.DB
}

// NewMyPointRepository creates a new MyPointRepository
func NewMyPointRepository(db *gorm.DB) MyPointRepository {
	return &myPointRepository{db: db}
}

func (r *myPointRepository) GetSummary(mbID string) (*PointSummary, error) {
	// 현재 보유 포인트
	var mbPoint int
	err := r.db.Table("g5_member").
		Select("COALESCE(mb_point, 0)").
		Where("mb_id = ?", mbID).
		Scan(&mbPoint).Error
	if err != nil {
		return nil, err
	}

	// 총 적립 / 총 사용
	var result struct {
		TotalEarned int
		TotalUsed   int
	}
	err = r.db.Table("g5_point").
		Select("COALESCE(SUM(CASE WHEN po_point > 0 THEN po_point ELSE 0 END), 0) as total_earned, COALESCE(SUM(CASE WHEN po_point < 0 THEN po_point ELSE 0 END), 0) as total_used").
		Where("mb_id = ?", mbID).
		Scan(&result).Error
	if err != nil {
		return nil, err
	}

	return &PointSummary{
		TotalPoint:  mbPoint,
		TotalEarned: result.TotalEarned,
		TotalUsed:   -result.TotalUsed, // 양수로 변환
	}, nil
}

func (r *myPointRepository) GetHistory(mbID string, page, limit int) ([]v2.G5Point, int64, error) {
	var total int64
	err := r.db.Model(&v2.G5Point{}).Where("mb_id = ?", mbID).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	var items []v2.G5Point
	offset := (page - 1) * limit
	err = r.db.Where("mb_id = ?", mbID).
		Order("po_id DESC").
		Offset(offset).
		Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}
