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
	// MarkDead 는 재시도 상한을 넘긴 이벤트를 종착 상태로 보낸다.
	MarkDead(id int64, errMsg string) error
	// ReclaimStaleProcessing 은 claimed_at 이 오래된 processing 행을 pending 으로 되돌린다.
	ReclaimStaleProcessing(staleBefore time.Time, limit int) (int64, error)
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

// MarkDead 는 재시도 상한을 넘긴 이벤트를 종착 상태로 보낸다.
// ⛔ 삭제하지 않고 상태만 바꾼다 — 왜 포기했는지 추적할 수 있어야 한다.
func (r *writeAfterEventRepository) MarkDead(id int64, errMsg string) error {
	if len(errMsg) > 2000 {
		errMsg = errMsg[:2000]
	}
	return r.db.Model(&domain.WriteAfterEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":      domain.WriteAfterEventStatusDead,
			"retry_count": gorm.Expr("retry_count + 1"),
			"last_error":  errMsg,
		}).Error
}

// ReclaimStaleProcessing 은 claimed_at 이 staleBefore 보다 오래된 processing 행을
// pending 으로 되돌린다.
//
// ⛔ 왜 필요한가 — ClaimPending 은 처리 **전에** status 를 processing 으로 찍는다.
//
//	그 사이 프로세스가 죽으면 그 배치는 아무도 다시 집지 않는다. 회수 로직이 없어
//	2026-03-24 부터 4,774 행이 갇혀 있었다(재시작 1회당 최대 batch×concurrency 건).
//	graceful shutdown 을 넣어도 OOM·SIGKILL·노드 장애는 남으므로 이 그물이 필요하다.
//
// ⛔ available_at 을 흩어서 되돌린다. 한꺼번에 즉시 만기시키면 회수 자체가 부하가 된다.
// ⛔ retry_count 는 올리지 않는다 — 이벤트가 실패한 게 아니라 우리가 흘린 것이다.
func (r *writeAfterEventRepository) ReclaimStaleProcessing(staleBefore time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 500
	}
	res := r.db.Exec(`
		UPDATE `+"`"+`g5_write_after_events`+"`"+`
		   SET status = ?, claimed_at = NULL,
		       available_at = DATE_ADD(?, INTERVAL MOD(id, 60) SECOND)
		 WHERE status = ? AND claimed_at IS NOT NULL AND claimed_at < ?
		 ORDER BY claimed_at ASC
		 LIMIT ?`,
		domain.WriteAfterEventStatusPending,
		time.Now(),
		domain.WriteAfterEventStatusProcessing,
		staleBefore,
		limit,
	)
	return res.RowsAffected, res.Error
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
