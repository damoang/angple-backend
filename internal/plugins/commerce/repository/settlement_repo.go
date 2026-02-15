package repository

import (
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"gorm.io/gorm"
)

// SettlementRepository 정산 저장소 인터페이스
type SettlementRepository interface {
	// 생성/수정
	Create(settlement *domain.Settlement) error
	Update(id uint64, settlement *domain.Settlement) error

	// 조회
	FindByID(id uint64) (*domain.Settlement, error)
	FindBySellerAndPeriod(sellerID uint64, periodStart, periodEnd time.Time) (*domain.Settlement, error)

	// 목록 조회
	ListBySeller(sellerID uint64, req *domain.SettlementListRequest) ([]*domain.Settlement, int64, error)
	ListAll(req *domain.SettlementListRequest) ([]*domain.Settlement, int64, error)
	ListPending() ([]*domain.Settlement, error)

	// 통계
	GetSummaryBySeller(sellerID uint64) (*domain.SettlementSummary, error)

	// 정산 생성을 위한 주문 조회
	GetPendingSettlementOrders(sellerID uint64, periodStart, periodEnd time.Time) ([]*domain.OrderItem, error)
}

// settlementRepository GORM 구현체
type settlementRepository struct {
	db *gorm.DB
}

// NewSettlementRepository 생성자
func NewSettlementRepository(db *gorm.DB) SettlementRepository {
	return &settlementRepository{db: db}
}

// Create 정산 레코드 생성
func (r *settlementRepository) Create(settlement *domain.Settlement) error {
	now := time.Now()
	settlement.CreatedAt = now
	settlement.UpdatedAt = now
	return r.db.Create(settlement).Error
}

// Update 정산 레코드 수정
func (r *settlementRepository) Update(id uint64, settlement *domain.Settlement) error {
	settlement.UpdatedAt = time.Now()
	return r.db.Model(&domain.Settlement{}).Where("id = ?", id).Updates(settlement).Error
}

// FindByID ID로 정산 조회
func (r *settlementRepository) FindByID(id uint64) (*domain.Settlement, error) {
	var settlement domain.Settlement
	err := r.db.Where("id = ?", id).First(&settlement).Error
	if err != nil {
		return nil, err
	}
	return &settlement, nil
}

// FindBySellerAndPeriod 판매자와 기간으로 정산 조회
func (r *settlementRepository) FindBySellerAndPeriod(sellerID uint64, periodStart, periodEnd time.Time) (*domain.Settlement, error) {
	var settlement domain.Settlement
	err := r.db.Where("seller_id = ? AND period_start = ? AND period_end = ?",
		sellerID, periodStart, periodEnd).First(&settlement).Error
	if err != nil {
		return nil, err
	}
	return &settlement, nil
}

// ListBySeller 판매자의 정산 목록 조회
func (r *settlementRepository) ListBySeller(sellerID uint64, req *domain.SettlementListRequest) ([]*domain.Settlement, int64, error) {
	var settlements []*domain.Settlement
	var total int64

	query := r.db.Model(&domain.Settlement{}).Where("seller_id = ?", sellerID)

	// 상태 필터
	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	// 연도 필터
	if req.Year > 0 {
		query = query.Where("YEAR(period_start) = ?", req.Year)
	}

	// 월 필터
	if req.Month > 0 {
		query = query.Where("MONTH(period_start) = ?", req.Month)
	}

	// 카운트
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 정렬
	sortBy := "created_at"
	if req.SortBy != "" {
		sortBy = req.SortBy
	}
	sortOrder := "DESC"
	if req.SortOrder == "asc" {
		sortOrder = "ASC"
	}
	query = query.Order(sortBy + " " + sortOrder)

	// 페이징
	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	if err := query.Offset(offset).Limit(limit).Find(&settlements).Error; err != nil {
		return nil, 0, err
	}

	return settlements, total, nil
}

// ListAll 전체 정산 목록 조회 (관리자용)
func (r *settlementRepository) ListAll(req *domain.SettlementListRequest) ([]*domain.Settlement, int64, error) {
	var settlements []*domain.Settlement
	var total int64

	query := r.db.Model(&domain.Settlement{})

	// 상태 필터
	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	// 연도 필터
	if req.Year > 0 {
		query = query.Where("YEAR(period_start) = ?", req.Year)
	}

	// 월 필터
	if req.Month > 0 {
		query = query.Where("MONTH(period_start) = ?", req.Month)
	}

	// 카운트
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 정렬
	sortBy := "created_at"
	if req.SortBy != "" {
		sortBy = req.SortBy
	}
	sortOrder := "DESC"
	if req.SortOrder == "asc" {
		sortOrder = "ASC"
	}
	query = query.Order(sortBy + " " + sortOrder)

	// 페이징
	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	if err := query.Offset(offset).Limit(limit).Find(&settlements).Error; err != nil {
		return nil, 0, err
	}

	return settlements, total, nil
}

// ListPending 대기 중인 정산 목록
func (r *settlementRepository) ListPending() ([]*domain.Settlement, error) {
	var settlements []*domain.Settlement
	err := r.db.Where("status = ?", domain.SettlementStatusPending).
		Order("created_at ASC").
		Find(&settlements).Error
	if err != nil {
		return nil, err
	}
	return settlements, nil
}

// GetSummaryBySeller 판매자의 정산 요약
func (r *settlementRepository) GetSummaryBySeller(sellerID uint64) (*domain.SettlementSummary, error) {
	var summary domain.SettlementSummary
	summary.Currency = "KRW"

	// 전체 매출
	r.db.Model(&domain.Settlement{}).
		Where("seller_id = ?", sellerID).
		Select("COALESCE(SUM(total_sales), 0)").
		Scan(&summary.TotalSales)

	// 전체 환불
	r.db.Model(&domain.Settlement{}).
		Where("seller_id = ?", sellerID).
		Select("COALESCE(SUM(total_refunds), 0)").
		Scan(&summary.TotalRefunds)

	// 전체 PG 수수료
	r.db.Model(&domain.Settlement{}).
		Where("seller_id = ?", sellerID).
		Select("COALESCE(SUM(pg_fees), 0)").
		Scan(&summary.TotalPGFees)

	// 전체 플랫폼 수수료
	r.db.Model(&domain.Settlement{}).
		Where("seller_id = ?", sellerID).
		Select("COALESCE(SUM(platform_fees), 0)").
		Scan(&summary.TotalPlatformFees)

	// 완료된 정산 금액
	r.db.Model(&domain.Settlement{}).
		Where("seller_id = ? AND status = ?", sellerID, domain.SettlementStatusCompleted).
		Select("COALESCE(SUM(settlement_amount), 0)").
		Scan(&summary.TotalSettled)

	// 대기 중인 정산 금액
	r.db.Model(&domain.Settlement{}).
		Where("seller_id = ? AND status = ?", sellerID, domain.SettlementStatusPending).
		Select("COALESCE(SUM(settlement_amount), 0)").
		Scan(&summary.PendingAmount)

	return &summary, nil
}

// GetPendingSettlementOrders 정산 대기 중인 주문 아이템 조회
func (r *settlementRepository) GetPendingSettlementOrders(sellerID uint64, periodStart, periodEnd time.Time) ([]*domain.OrderItem, error) {
	var items []*domain.OrderItem

	err := r.db.Table("commerce_order_items AS oi").
		Joins("JOIN commerce_orders AS o ON oi.order_id = o.id").
		Joins("JOIN commerce_products AS p ON oi.product_id = p.id").
		Where("p.seller_id = ?", sellerID).
		Where("o.status IN ?", []string{string(domain.OrderStatusCompleted), string(domain.OrderStatusDelivered)}).
		Where("o.paid_at >= ? AND o.paid_at < ?", periodStart, periodEnd).
		Where("oi.settlement_status = ?", "pending").
		Preload("Order").
		Find(&items).Error

	if err != nil {
		return nil, err
	}

	return items, nil
}
