package gnuboard

import (
	"fmt"
	"strings"
	"time"

	domain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WriteAfterEventRepository interface {
	Create(db *gorm.DB, event *domain.WriteAfterEvent) error
	ClaimPending(now time.Time, limit int) ([]domain.WriteAfterEvent, error)
	MarkProcessed(id int64) error
	MarkFailed(id int64, errMsg string) error
	MarkFailedWithDelay(id int64, errMsg string, delay time.Duration) error
	CountPending(now time.Time) (int64, error)
}

type writeAfterEventRepository struct {
	db *gorm.DB
}

func NewWriteAfterEventRepository(db *gorm.DB) WriteAfterEventRepository {
	return &writeAfterEventRepository{db: db}
}

func (r *writeAfterEventRepository) Create(db *gorm.DB, event *domain.WriteAfterEvent) error {
	if db == nil {
		db = r.db
	}
	if event.Status == "" {
		event.Status = domain.WriteAfterEventStatusPending
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	if event.AvailableAt.IsZero() {
		event.AvailableAt = event.OccurredAt
	}
	return db.Create(event).Error
}

func (r *writeAfterEventRepository) ClaimPending(now time.Time, limit int) ([]domain.WriteAfterEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	var events []domain.WriteAfterEvent
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND available_at <= ?", domain.WriteAfterEventStatusPending, now).
			Order("available_at ASC, id ASC").
			Limit(limit).
			Find(&events).Error; err != nil {
			return err
		}

		if len(events) == 0 {
			return nil
		}

		ids := make([]int64, 0, len(events))
		for _, event := range events {
			ids = append(ids, event.ID)
		}
		if err := tx.Model(&domain.WriteAfterEvent{}).
			Where("id IN ?", ids).
			Updates(map[string]interface{}{
				"status":     domain.WriteAfterEventStatusProcessing,
				"claimed_at": now,
			}).Error; err != nil {
			return err
		}

		for i := range events {
			events[i].Status = domain.WriteAfterEventStatusProcessing
			events[i].ClaimedAt = &now
		}
		return nil
	})
	return events, err
}

func (r *writeAfterEventRepository) MarkProcessed(id int64) error {
	now := time.Now()
	return r.db.Model(&domain.WriteAfterEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":       domain.WriteAfterEventStatusProcessed,
			"processed_at": now,
			"last_error":   nil,
		}).Error
}

func (r *writeAfterEventRepository) MarkFailed(id int64, errMsg string) error {
	return r.MarkFailedWithDelay(id, errMsg, 5*time.Second)
}

func (r *writeAfterEventRepository) MarkFailedWithDelay(id int64, errMsg string, delay time.Duration) error {
	if len(errMsg) > 2000 {
		errMsg = errMsg[:2000]
	}
	if delay <= 0 {
		delay = 5 * time.Second
	}
	return r.db.Model(&domain.WriteAfterEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":       domain.WriteAfterEventStatusPending,
			"retry_count":  gorm.Expr("retry_count + 1"),
			"last_error":   errMsg,
			"available_at": time.Now().Add(delay),
		}).Error
}

func (r *writeAfterEventRepository) CountPending(now time.Time) (int64, error) {
	var count int64
	err := r.db.Model(&domain.WriteAfterEvent{}).
		Where("status = ? AND available_at <= ?", domain.WriteAfterEventStatusPending, now).
		Count(&count).Error
	return count, err
}

func TrimWriteAfterEventError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 2000 {
		return msg[:2000]
	}
	return msg
}

func FormatUnknownWriteAfterEvent(eventType string) error {
	return fmt.Errorf("unknown write-after event type %s", eventType)
}
