package repository

import (
	"context"
	"errors"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// SubscriptionRepository handles subscription and invoice persistence
type SubscriptionRepository struct {
	db *gorm.DB
}

// NewSubscriptionRepository creates a new SubscriptionRepository
func NewSubscriptionRepository(db *gorm.DB) *SubscriptionRepository {
	return &SubscriptionRepository{db: db}
}

// AutoMigrate creates subscription tables
func (r *SubscriptionRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&domain.Subscription{}, &domain.Invoice{})
}

// ========================================
// Subscription CRUD
// ========================================

// FindBySiteID retrieves a subscription by site ID
func (r *SubscriptionRepository) FindBySiteID(ctx context.Context, siteID string) (*domain.Subscription, error) {
	var sub domain.Subscription
	err := r.db.WithContext(ctx).Where("site_id = ?", siteID).First(&sub).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &sub, nil
}

// Create creates a new subscription
func (r *SubscriptionRepository) Create(ctx context.Context, sub *domain.Subscription) error {
	return r.db.WithContext(ctx).Create(sub).Error
}

// Update updates an existing subscription
func (r *SubscriptionRepository) Update(ctx context.Context, sub *domain.Subscription) error {
	return r.db.WithContext(ctx).Save(sub).Error
}

// ========================================
// Invoice CRUD
// ========================================

// CreateInvoice creates a new invoice
func (r *SubscriptionRepository) CreateInvoice(ctx context.Context, inv *domain.Invoice) error {
	return r.db.WithContext(ctx).Create(inv).Error
}

// ListInvoices retrieves invoices for a site
func (r *SubscriptionRepository) ListInvoices(ctx context.Context, siteID string, limit, offset int) ([]domain.Invoice, int64, error) {
	var invoices []domain.Invoice
	var total int64

	query := r.db.WithContext(ctx).Model(&domain.Invoice{}).Where("site_id = ?", siteID)
	query.Count(&total)

	err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&invoices).Error
	return invoices, total, err
}

// FindInvoiceByID retrieves an invoice by ID
func (r *SubscriptionRepository) FindInvoiceByID(ctx context.Context, invoiceID int64) (*domain.Invoice, error) {
	var inv domain.Invoice
	err := r.db.WithContext(ctx).Where("id = ?", invoiceID).First(&inv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &inv, nil
}

// UpdateInvoice updates an invoice
func (r *SubscriptionRepository) UpdateInvoice(ctx context.Context, inv *domain.Invoice) error {
	return r.db.WithContext(ctx).Save(inv).Error
}
