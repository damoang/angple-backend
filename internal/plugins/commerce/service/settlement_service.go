package service

import (
	"errors"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/repository"
	"gorm.io/gorm"
)

// 정산 에러 정의
var (
	ErrSettlementNotFound    = errors.New("settlement not found")
	ErrSettlementForbidden   = errors.New("you are not the owner of this settlement")
	ErrSettlementNotPending  = errors.New("settlement is not in pending status")
	ErrSettlementNoOrders    = errors.New("no orders to settle")
	ErrInvalidPeriod         = errors.New("invalid settlement period")
	ErrDuplicateSettlement   = errors.New("settlement already exists for this period")
)

// SettlementService 정산 서비스 인터페이스
type SettlementService interface {
	// 정산 생성
	CreateSettlement(sellerID uint64, periodStart, periodEnd time.Time) (*domain.SettlementResponse, error)

	// 정산 조회
	GetSettlement(id uint64, sellerID uint64) (*domain.SettlementResponse, error)
	ListSettlements(sellerID uint64, req *domain.SettlementListRequest) ([]*domain.SettlementResponse, int64, error)

	// 정산 처리 (관리자)
	ProcessSettlement(id uint64, adminID uint64, req *domain.ProcessSettlementRequest) error

	// 정산 요약
	GetSettlementSummary(sellerID uint64) (*domain.SettlementSummary, error)

	// 정산 대상 주문 조회
	GetPendingOrders(sellerID uint64, periodStart, periodEnd time.Time) ([]*domain.OrderItem, error)
}

// settlementService 구현체
type settlementService struct {
	settlementRepo repository.SettlementRepository
	orderRepo      repository.OrderRepository
	pgFeeRate      float64 // PG 수수료율 (기본 3.3%)
	platformRate   float64 // 플랫폼 수수료율 (기본 5%)
}

// NewSettlementService 생성자
func NewSettlementService(
	settlementRepo repository.SettlementRepository,
	orderRepo repository.OrderRepository,
) SettlementService {
	return &settlementService{
		settlementRepo: settlementRepo,
		orderRepo:      orderRepo,
		pgFeeRate:      0.033,  // 3.3%
		platformRate:   0.05,   // 5%
	}
}

// CreateSettlement 정산 생성
func (s *settlementService) CreateSettlement(sellerID uint64, periodStart, periodEnd time.Time) (*domain.SettlementResponse, error) {
	// 기간 유효성 검사
	if periodEnd.Before(periodStart) {
		return nil, ErrInvalidPeriod
	}

	// 중복 정산 확인
	existing, err := s.settlementRepo.FindBySellerAndPeriod(sellerID, periodStart, periodEnd)
	if err == nil && existing != nil {
		return nil, ErrDuplicateSettlement
	}

	// 정산 대상 주문 조회
	orders, err := s.settlementRepo.GetPendingSettlementOrders(sellerID, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}

	if len(orders) == 0 {
		return nil, ErrSettlementNoOrders
	}

	// 금액 계산
	var totalSales float64
	var totalRefunds float64

	for _, item := range orders {
		if item.Order != nil && item.Order.Status == domain.OrderStatusRefunded {
			totalRefunds += item.Subtotal
		} else {
			totalSales += item.Subtotal
		}
	}

	// 수수료 계산
	netSales := totalSales - totalRefunds
	pgFees := netSales * s.pgFeeRate
	platformFees := netSales * s.platformRate
	settlementAmount := netSales - pgFees - platformFees

	// 정산 생성
	settlement := &domain.Settlement{
		SellerID:         sellerID,
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		TotalSales:       totalSales,
		TotalRefunds:     totalRefunds,
		PGFees:           pgFees,
		PlatformFees:     platformFees,
		SettlementAmount: settlementAmount,
		Currency:         "KRW",
		Status:           domain.SettlementStatusPending,
	}

	if err := s.settlementRepo.Create(settlement); err != nil {
		return nil, err
	}

	return settlement.ToResponse(), nil
}

// GetSettlement 정산 조회
func (s *settlementService) GetSettlement(id uint64, sellerID uint64) (*domain.SettlementResponse, error) {
	settlement, err := s.settlementRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSettlementNotFound
		}
		return nil, err
	}

	// 소유자 확인 (sellerID가 0이면 관리자)
	if sellerID > 0 && settlement.SellerID != sellerID {
		return nil, ErrSettlementForbidden
	}

	return settlement.ToResponse(), nil
}

// ListSettlements 정산 목록 조회
func (s *settlementService) ListSettlements(sellerID uint64, req *domain.SettlementListRequest) ([]*domain.SettlementResponse, int64, error) {
	var settlements []*domain.Settlement
	var total int64
	var err error

	if sellerID > 0 {
		// 판매자별 조회
		settlements, total, err = s.settlementRepo.ListBySeller(sellerID, req)
	} else {
		// 전체 조회 (관리자)
		settlements, total, err = s.settlementRepo.ListAll(req)
	}

	if err != nil {
		return nil, 0, err
	}

	var responses []*domain.SettlementResponse
	for _, settlement := range settlements {
		responses = append(responses, settlement.ToResponse())
	}

	return responses, total, nil
}

// ProcessSettlement 정산 처리
func (s *settlementService) ProcessSettlement(id uint64, adminID uint64, req *domain.ProcessSettlementRequest) error {
	settlement, err := s.settlementRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSettlementNotFound
		}
		return err
	}

	// 상태 확인
	if settlement.Status != domain.SettlementStatusPending {
		return ErrSettlementNotPending
	}

	// 처리 상태로 변경
	now := time.Now()
	updateData := &domain.Settlement{
		Status:      domain.SettlementStatusProcessing,
		ProcessedAt: &now,
		ProcessedBy: &adminID,
	}

	if req.Notes != "" {
		updateData.Notes = req.Notes
	}

	if err := s.settlementRepo.Update(id, updateData); err != nil {
		return err
	}

	// 실제 송금 처리 (외부 서비스 연동)
	// TODO: 은행 API 연동

	// 완료 상태로 변경
	completeData := &domain.Settlement{
		Status: domain.SettlementStatusCompleted,
	}

	return s.settlementRepo.Update(id, completeData)
}

// GetSettlementSummary 정산 요약
func (s *settlementService) GetSettlementSummary(sellerID uint64) (*domain.SettlementSummary, error) {
	return s.settlementRepo.GetSummaryBySeller(sellerID)
}

// GetPendingOrders 정산 대기 주문 조회
func (s *settlementService) GetPendingOrders(sellerID uint64, periodStart, periodEnd time.Time) ([]*domain.OrderItem, error) {
	return s.settlementRepo.GetPendingSettlementOrders(sellerID, periodStart, periodEnd)
}

// SettlementConfig 정산 설정
type SettlementConfig struct {
	PGFeeRate     float64 // PG 수수료율
	PlatformRate  float64 // 플랫폼 수수료율
}

// SetConfig 정산 설정 변경
func (s *settlementService) SetConfig(config *SettlementConfig) {
	if config.PGFeeRate > 0 {
		s.pgFeeRate = config.PGFeeRate
	}
	if config.PlatformRate > 0 {
		s.platformRate = config.PlatformRate
	}
}
