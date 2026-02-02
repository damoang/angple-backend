package middleware

import (
	"context"
	"time"

	"github.com/damoang/angple-backend/pkg/logger"
	"gorm.io/gorm"
)

// AuditLog represents a record of sensitive operations
type AuditLog struct {
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`

	UserID    string `gorm:"column:user_id;index" json:"user_id"`
	Action    string `gorm:"column:action;index" json:"action"`     // login, register, withdraw, plan_change, suspend, etc.
	Resource  string `gorm:"column:resource" json:"resource"`       // user, site, subscription, etc.
	ResourceID string `gorm:"column:resource_id" json:"resource_id"`
	Details   string `gorm:"column:details;type:text" json:"details"`
	ClientIP  string `gorm:"column:client_ip" json:"client_ip"`
	UserAgent string `gorm:"column:user_agent" json:"user_agent"`
	RequestID string `gorm:"column:request_id" json:"request_id"`

	ID int64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}

// AuditLogger handles writing audit log entries
type AuditLogger struct {
	db *gorm.DB
}

// NewAuditLogger creates a new AuditLogger
func NewAuditLogger(db *gorm.DB) *AuditLogger {
	if db != nil {
		_ = db.AutoMigrate(&AuditLog{})
	}
	return &AuditLogger{db: db}
}

// Log writes an audit entry to the database
func (a *AuditLogger) Log(ctx context.Context, userID, action, resource, resourceID, details, clientIP, userAgent, requestID string) {
	if a.db == nil {
		return
	}

	entry := &AuditLog{
		UserID:     userID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Details:    details,
		ClientIP:   clientIP,
		UserAgent:  userAgent,
		RequestID:  requestID,
	}

	// Write async to avoid blocking the request
	go func() {
		if err := a.db.Create(entry).Error; err != nil {
			logger.GetLogger().Error().Err(err).
				Str("action", action).
				Str("user_id", userID).
				Msg("audit log write failed")
		}
	}()
}

// ListAuditLogs retrieves paginated audit logs with optional filters
func (a *AuditLogger) ListAuditLogs(ctx context.Context, userID, action string, page, perPage int) ([]AuditLog, int64, error) {
	var logs []AuditLog
	var total int64

	query := a.db.WithContext(ctx).Model(&AuditLog{})
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}

	query.Count(&total)
	err := query.Order("created_at DESC").
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&logs).Error

	return logs, total, err
}
