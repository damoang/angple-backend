package repository

import (
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"gorm.io/gorm"
)

// PaymentRepository 결제 저장소 인터페이스
type PaymentRepository interface {
	// 생성/수정
	Create(payment *domain.Payment) error
	Update(id uint64, payment *domain.Payment) error
	UpdateStatus(id uint64, status domain.PaymentStatus) error
	UpdatePaid(id uint64, paidAt time.Time, pgFee float64) error
	UpdateCancelled(id uint64, cancelledAmount float64, cancelReason string) error

	// 조회
	FindByID(id uint64) (*domain.Payment, error)
	FindByIDWithOrder(id uint64) (*domain.Payment, error)
	FindByOrderID(orderID uint64) (*domain.Payment, error)
	FindByPGTID(provider domain.PGProvider, pgTID string) (*domain.Payment, error)
	FindByPGOrderID(provider domain.PGProvider, pgOrderID string) (*domain.Payment, error)

	// 목록 조회
	ListByOrderID(orderID uint64) ([]*domain.Payment, error)
}

// paymentRepository GORM 구현체
type paymentRepository struct {
	db *gorm.DB
}

// NewPaymentRepository 생성자
func NewPaymentRepository(db *gorm.DB) PaymentRepository {
	return &paymentRepository{db: db}
}

// Create 결제 생성
func (r *paymentRepository) Create(payment *domain.Payment) error {
	now := time.Now()
	payment.CreatedAt = now
	payment.UpdatedAt = now
	return r.db.Create(payment).Error
}

// Update 결제 수정
func (r *paymentRepository) Update(id uint64, payment *domain.Payment) error {
	payment.UpdatedAt = time.Now()
	return r.db.Model(&domain.Payment{}).Where("id = ?", id).Updates(payment).Error
}

// UpdateStatus 결제 상태 업데이트
func (r *paymentRepository) UpdateStatus(id uint64, status domain.PaymentStatus) error {
	return r.db.Model(&domain.Payment{}).
		Where("id = ?", id).
		UpdateColumns(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

// UpdatePaid 결제 완료 처리
func (r *paymentRepository) UpdatePaid(id uint64, paidAt time.Time, pgFee float64) error {
	return r.db.Model(&domain.Payment{}).
		Where("id = ?", id).
		UpdateColumns(map[string]interface{}{
			"status":     domain.PaymentStatusPaid,
			"paid_at":    paidAt,
			"pg_fee":     pgFee,
			"updated_at": time.Now(),
		}).Error
}

// UpdateCancelled 결제 취소 처리
func (r *paymentRepository) UpdateCancelled(id uint64, cancelledAmount float64, cancelReason string) error {
	now := time.Now()

	// 기존 결제 조회
	var payment domain.Payment
	if err := r.db.Where("id = ?", id).First(&payment).Error; err != nil {
		return err
	}

	// 상태 결정
	newStatus := domain.PaymentStatusCancelled
	totalCancelledAmount := payment.CancelledAmount + cancelledAmount
	if totalCancelledAmount < payment.Amount {
		newStatus = domain.PaymentStatusPartialCancelled
	}

	return r.db.Model(&domain.Payment{}).
		Where("id = ?", id).
		UpdateColumns(map[string]interface{}{
			"status":           newStatus,
			"cancelled_amount": gorm.Expr("cancelled_amount + ?", cancelledAmount),
			"cancel_reason":    cancelReason,
			"cancelled_at":     now,
			"updated_at":       now,
		}).Error
}

// FindByID ID로 결제 조회
func (r *paymentRepository) FindByID(id uint64) (*domain.Payment, error) {
	var payment domain.Payment
	err := r.db.Where("id = ?", id).First(&payment).Error
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

// FindByIDWithOrder ID로 결제 조회 (주문 정보 포함)
func (r *paymentRepository) FindByIDWithOrder(id uint64) (*domain.Payment, error) {
	var payment domain.Payment
	err := r.db.Preload("Order").Where("id = ?", id).First(&payment).Error
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

// FindByOrderID 주문 ID로 결제 조회
func (r *paymentRepository) FindByOrderID(orderID uint64) (*domain.Payment, error) {
	var payment domain.Payment
	err := r.db.Where("order_id = ?", orderID).Order("created_at DESC").First(&payment).Error
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

// FindByPGTID PG TID로 결제 조회
func (r *paymentRepository) FindByPGTID(provider domain.PGProvider, pgTID string) (*domain.Payment, error) {
	var payment domain.Payment
	err := r.db.Where("pg_provider = ? AND pg_tid = ?", provider, pgTID).First(&payment).Error
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

// FindByPGOrderID PG 주문 ID로 결제 조회
func (r *paymentRepository) FindByPGOrderID(provider domain.PGProvider, pgOrderID string) (*domain.Payment, error) {
	var payment domain.Payment
	err := r.db.Where("pg_provider = ? AND pg_order_id = ?", provider, pgOrderID).First(&payment).Error
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

// ListByOrderID 주문의 결제 목록 조회
func (r *paymentRepository) ListByOrderID(orderID uint64) ([]*domain.Payment, error) {
	var payments []*domain.Payment
	err := r.db.Where("order_id = ?", orderID).Order("created_at DESC").Find(&payments).Error
	if err != nil {
		return nil, err
	}
	return payments, nil
}
