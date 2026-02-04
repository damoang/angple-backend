package domain

import (
	"time"
)

// SettlementStatus 정산 상태
type SettlementStatus string

const (
	SettlementStatusPending    SettlementStatus = "pending"    // 정산 대기
	SettlementStatusProcessing SettlementStatus = "processing" // 처리 중
	SettlementStatusCompleted  SettlementStatus = "completed"  // 완료
	SettlementStatusFailed     SettlementStatus = "failed"     // 실패
)

// Settlement 정산 엔티티
type Settlement struct {
	ID       uint64 `gorm:"primaryKey" json:"id"`
	SellerID uint64 `gorm:"column:seller_id;not null" json:"seller_id"`

	// 정산 기간
	PeriodStart time.Time `gorm:"column:period_start;not null" json:"period_start"`
	PeriodEnd   time.Time `gorm:"column:period_end;not null" json:"period_end"`

	// 금액
	TotalSales       float64 `gorm:"column:total_sales;type:decimal(12,2);default:0" json:"total_sales"`
	TotalRefunds     float64 `gorm:"column:total_refunds;type:decimal(12,2);default:0" json:"total_refunds"`
	PGFees           float64 `gorm:"column:pg_fees;type:decimal(12,2);default:0" json:"pg_fees"`
	PlatformFees     float64 `gorm:"column:platform_fees;type:decimal(12,2);default:0" json:"platform_fees"`
	SettlementAmount float64 `gorm:"column:settlement_amount;type:decimal(12,2);default:0" json:"settlement_amount"`
	Currency         string  `gorm:"size:3;default:'KRW'" json:"currency"`

	// 상태
	Status SettlementStatus `gorm:"size:20;default:'pending'" json:"status"`

	// 입금 정보
	BankName    string `gorm:"column:bank_name;size:50" json:"bank_name,omitempty"`
	BankAccount string `gorm:"column:bank_account;size:50" json:"bank_account,omitempty"`
	BankHolder  string `gorm:"column:bank_holder;size:50" json:"bank_holder,omitempty"`

	// 처리 정보
	ProcessedAt *time.Time `gorm:"column:processed_at" json:"processed_at,omitempty"`
	ProcessedBy *uint64    `gorm:"column:processed_by" json:"processed_by,omitempty"`

	// 메타
	Notes    string `gorm:"type:text" json:"notes,omitempty"`
	MetaData string `gorm:"column:meta_data;type:json" json:"meta_data,omitempty"`

	// 타임스탬프
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

// TableName GORM 테이블명
func (Settlement) TableName() string {
	return "commerce_settlements"
}

// SettlementListRequest 정산 목록 조회 요청
type SettlementListRequest struct {
	Page      int    `form:"page" binding:"omitempty,gte=1"`
	Limit     int    `form:"limit" binding:"omitempty,gte=1,lte=100"`
	Status    string `form:"status" binding:"omitempty,oneof=pending processing completed failed"`
	Year      int    `form:"year" binding:"omitempty,gte=2020,lte=2100"`
	Month     int    `form:"month" binding:"omitempty,gte=1,lte=12"`
	SortBy    string `form:"sort_by" binding:"omitempty,oneof=created_at period_start settlement_amount"`
	SortOrder string `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// ProcessSettlementRequest 정산 처리 요청
type ProcessSettlementRequest struct {
	Notes string `json:"notes" binding:"omitempty,max=1000"`
}

// SettlementResponse 정산 응답 DTO
type SettlementResponse struct {
	ID               uint64     `json:"id"`
	SellerID         uint64     `json:"seller_id"`
	PeriodStart      time.Time  `json:"period_start"`
	PeriodEnd        time.Time  `json:"period_end"`
	TotalSales       float64    `json:"total_sales"`
	TotalRefunds     float64    `json:"total_refunds"`
	PGFees           float64    `json:"pg_fees"`
	PlatformFees     float64    `json:"platform_fees"`
	SettlementAmount float64    `json:"settlement_amount"`
	Currency         string     `json:"currency"`
	Status           string     `json:"status"`
	BankName         string     `json:"bank_name,omitempty"`
	BankAccount      string     `json:"bank_account,omitempty"`
	BankHolder       string     `json:"bank_holder,omitempty"`
	ProcessedAt      *time.Time `json:"processed_at,omitempty"`
	Notes            string     `json:"notes,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// ToResponse Settlement를 SettlementResponse로 변환
func (s *Settlement) ToResponse() *SettlementResponse {
	return &SettlementResponse{
		ID:               s.ID,
		SellerID:         s.SellerID,
		PeriodStart:      s.PeriodStart,
		PeriodEnd:        s.PeriodEnd,
		TotalSales:       s.TotalSales,
		TotalRefunds:     s.TotalRefunds,
		PGFees:           s.PGFees,
		PlatformFees:     s.PlatformFees,
		SettlementAmount: s.SettlementAmount,
		Currency:         s.Currency,
		Status:           string(s.Status),
		BankName:         s.BankName,
		BankAccount:      s.BankAccount,
		BankHolder:       s.BankHolder,
		ProcessedAt:      s.ProcessedAt,
		Notes:            s.Notes,
		CreatedAt:        s.CreatedAt,
	}
}

// SettlementSummary 정산 요약
type SettlementSummary struct {
	TotalSales        float64 `json:"total_sales"`
	TotalRefunds      float64 `json:"total_refunds"`
	TotalPGFees       float64 `json:"total_pg_fees"`
	TotalPlatformFees float64 `json:"total_platform_fees"`
	TotalSettled      float64 `json:"total_settled"`
	PendingAmount     float64 `json:"pending_amount"`
	Currency          string  `json:"currency"`
}

// SellerBankInfo 판매자 정산 계좌 정보
type SellerBankInfo struct {
	BankName    string `json:"bank_name" binding:"required,max=50"`
	BankAccount string `json:"bank_account" binding:"required,max=50"`
	BankHolder  string `json:"bank_holder" binding:"required,max=50"`
}
