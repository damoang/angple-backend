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

	// ⛔ 한 방 UPDATE 로 하면 안 된다 — 2026-08-21 카나리에서 데드락이 실증됐다.
	//
	//	UPDATE ... WHERE status='processing' ORDER BY claimed_at LIMIT n 은
	//	claimed_at 에 인덱스가 없어 **filesort** 가 되고, 그 과정에서 processing 범위의
	//	행을 넓게 잠근다. 같은 시각 정상 처리 중인 MarkProcessed(단일 id UPDATE)와
	//	락 순서가 엇갈려 Error 1213 이 났다:
	//	  [WriteAfterWorker] mark processed 2689520: Error 1213 Deadlock found
	//	멱등이라 데이터는 안 잃지만 그만큼 헛돈다.
	//
	// 그래서 **읽기와 쓰기를 나눈다.** SELECT 는 락 없는 일관된 읽기로 id 만 뽑고,
	// UPDATE 는 PK 로만 잠근다. 잠그는 행이 정확히 대상 행뿐이라 범위가 겹치지 않는다.
	var ids []int64
	if err := r.db.Model(&domain.WriteAfterEvent{}).
		Where("status = ? AND claimed_at IS NOT NULL AND claimed_at < ?",
			domain.WriteAfterEventStatusProcessing, staleBefore).
		Order("claimed_at ASC").
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	// ⛔ status 조건을 UPDATE 에도 다시 건다. SELECT 와 UPDATE 사이에 워커가 그 행을
	//	processed 로 바꿨을 수 있다 — 조건이 없으면 **끝난 이벤트를 되살려** 중복 실행한다.
	// ⛔ available_at 을 60초에 흩는다. 한꺼번에 만기시키면 회수가 그 자체로 부하가 된다.
	// ⛔ retry_count 는 올리지 않는다 — 이벤트가 실패한 게 아니라 우리가 흘린 것이다.
	// ⛔ FORCE INDEX (PRIMARY) 를 빼면 안 된다. id IN (...) 을 줘도 옵티마이저가
	//	status='processing' 이 선택적일 때 idx_status_available 를 고른다
	//	(2026-08-21 EXPLAIN 실증: key=idx_status_available). 그러면 결국 processing
	//	범위를 훑어 락을 잡아, 이 수정의 목적이 통째로 무효가 된다.
	//	PRIMARY 를 강제하면 key=PRIMARY / rows=대상건수 로 **정확히 그 행만** 잠근다.
	res := r.db.Exec(`
		UPDATE `+"`"+`g5_write_after_events`+"`"+` FORCE INDEX (`+"`"+`PRIMARY`+"`"+`)
		   SET status = ?, claimed_at = NULL,
		       available_at = DATE_ADD(?, INTERVAL MOD(id, 60) SECOND)
		 WHERE id IN (?) AND status = ?`,
		domain.WriteAfterEventStatusPending,
		time.Now(),
		ids,
		domain.WriteAfterEventStatusProcessing,
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
