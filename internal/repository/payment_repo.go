package repository

import (
	"context"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// PaymentRepository handles payment persistence
type PaymentRepository struct {
	db *gorm.DB
}

// NewPaymentRepository creates a new PaymentRepository
func NewPaymentRepository(db *gorm.DB) *PaymentRepository {
	_ = db.AutoMigrate(&domain.Payment{})
	return &PaymentRepository{db: db}
}

// Create inserts a new payment record
func (r *PaymentRepository) Create(ctx context.Context, p *domain.Payment) error {
	return r.db.WithContext(ctx).Create(p).Error
}

// FindByOrderID finds a payment by order ID
func (r *PaymentRepository) FindByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	var p domain.Payment
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&p).Error
	return &p, err
}

// FindByExternalID finds a payment by provider's external ID
func (r *PaymentRepository) FindByExternalID(ctx context.Context, externalID string) (*domain.Payment, error) {
	var p domain.Payment
	err := r.db.WithContext(ctx).Where("external_pay_id = ?", externalID).First(&p).Error
	return &p, err
}

// FindByIdempotencyKey finds a payment by idempotency key
func (r *PaymentRepository) FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	var p domain.Payment
	err := r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&p).Error
	return &p, err
}

// Update updates a payment record
func (r *PaymentRepository) Update(ctx context.Context, p *domain.Payment) error {
	return r.db.WithContext(ctx).Save(p).Error
}

// ListBySiteID lists payments for a site with pagination
func (r *PaymentRepository) ListBySiteID(ctx context.Context, siteID string, page, perPage int) ([]domain.Payment, int64, error) {
	var payments []domain.Payment
	var total int64

	query := r.db.WithContext(ctx).Model(&domain.Payment{}).Where("site_id = ?", siteID)
	query.Count(&total)

	err := query.Order("created_at DESC").
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&payments).Error

	return payments, total, err
}

// ListPendingRetries finds failed payments that need retry
func (r *PaymentRepository) ListPendingRetries(ctx context.Context, limit int) ([]domain.Payment, error) {
	var payments []domain.Payment
	err := r.db.WithContext(ctx).
		Where("status = ? AND retry_count < ? AND next_retry_at <= NOW()", "failed", 3).
		Order("next_retry_at ASC").
		Limit(limit).
		Find(&payments).Error
	return payments, err
}
