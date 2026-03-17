package worker

import (
	"log"
	"sync"
	"time"

	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"gorm.io/gorm"
)

// DeleteWorker processes pending scheduled deletes in the background
type DeleteWorker struct {
	writeRepo gnurepo.WriteRepository
	sdRepo    gnurepo.ScheduledDeleteRepository
	eventRepo gnurepo.WriteAfterEventRepository
	db        *gorm.DB
	stop      chan struct{}
	wg        sync.WaitGroup
}

// NewDeleteWorker creates a new DeleteWorker
func NewDeleteWorker(db *gorm.DB, writeRepo gnurepo.WriteRepository, sdRepo gnurepo.ScheduledDeleteRepository, eventRepo gnurepo.WriteAfterEventRepository) *DeleteWorker {
	return &DeleteWorker{
		writeRepo: writeRepo,
		sdRepo:    sdRepo,
		eventRepo: eventRepo,
		db:        db,
		stop:      make(chan struct{}),
	}
}

// Start begins the background worker with a 30-second tick interval
func (w *DeleteWorker) Start() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		log.Println("[DeleteWorker] Started")

		for {
			select {
			case <-w.stop:
				log.Println("[DeleteWorker] Stopped")
				return
			case <-ticker.C:
				w.processPending()
			}
		}
	}()
}

// Stop gracefully stops the worker
func (w *DeleteWorker) Stop() {
	close(w.stop)
	w.wg.Wait()
}

// processPending finds and executes all pending deletes whose scheduled_at has passed
func (w *DeleteWorker) processPending() {
	now := time.Now()
	records, err := w.sdRepo.FindPending(now, 50)
	if err != nil {
		log.Printf("[DeleteWorker] Error finding pending deletes: %v", err)
		return
	}

	if len(records) == 0 {
		return
	}

	log.Printf("[DeleteWorker] Processing %d pending deletes", len(records))

	for _, sd := range records {
		if sd.WrIsComment == 1 {
			comment, findErr := w.writeRepo.FindCommentByID(sd.BoTable, sd.WrID)
			if findErr != nil {
				log.Printf("[DeleteWorker] Error finding comment %s/%d before delete: %v", sd.BoTable, sd.WrID, findErr)
				continue
			}
			if err := w.db.Transaction(func(tx *gorm.DB) error {
				now := time.Now()
				if err := tx.Table("g5_write_"+sd.BoTable).
					Where("wr_id = ? AND wr_is_comment = 1", sd.WrID).
					Updates(map[string]interface{}{
						"wr_deleted_at": now,
						"wr_deleted_by": sd.RequestedBy,
					}).Error; err != nil {
					return err
				}
				if err := tx.Table("g5_write_"+sd.BoTable).
					Where("wr_id = ?", comment.WrParent).
					Update("wr_comment", gorm.Expr("GREATEST(COALESCE(wr_comment, 0) - 1, 0)")).
					Error; err != nil {
					return err
				}
				if w.eventRepo != nil {
					postID := comment.WrParent
					if err := w.eventRepo.Create(tx, &gnudomain.WriteAfterEvent{
						EventType:  gnudomain.WriteAfterEventTypeCommentDeleted,
						BoardSlug:  sd.BoTable,
						WriteID:    sd.WrID,
						PostID:     &postID,
						MemberID:   comment.MbID,
						Author:     comment.WrName,
						OccurredAt: now,
						Status:     gnudomain.WriteAfterEventStatusPending,
					}); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				log.Printf("[DeleteWorker] Error soft deleting comment %s/%d: %v", sd.BoTable, sd.WrID, err)
				continue
			}
		} else {
			post, findErr := w.writeRepo.FindPostByIDIncludeDeleted(sd.BoTable, sd.WrID)
			if findErr != nil {
				log.Printf("[DeleteWorker] Error finding post %s/%d before delete: %v", sd.BoTable, sd.WrID, findErr)
				continue
			}
			if err := w.db.Transaction(func(tx *gorm.DB) error {
				now := time.Now()
				if err := tx.Table("g5_write_"+sd.BoTable).
					Where("wr_id = ?", sd.WrID).
					Updates(map[string]interface{}{
						"wr_deleted_at": now,
						"wr_deleted_by": sd.RequestedBy,
					}).Error; err != nil {
					return err
				}
				if w.eventRepo != nil {
					if err := w.eventRepo.Create(tx, &gnudomain.WriteAfterEvent{
						EventType:  gnudomain.WriteAfterEventTypePostDeleted,
						BoardSlug:  sd.BoTable,
						WriteID:    sd.WrID,
						MemberID:   post.MbID,
						Author:     post.WrName,
						Subject:    post.WrSubject,
						OccurredAt: now,
						Status:     gnudomain.WriteAfterEventStatusPending,
					}); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				log.Printf("[DeleteWorker] Error soft deleting post %s/%d: %v", sd.BoTable, sd.WrID, err)
				continue
			}
		}

		// Mark as executed
		if err := w.sdRepo.MarkExecuted(sd.ID); err != nil {
			log.Printf("[DeleteWorker] Error marking as executed %d: %v", sd.ID, err)
		}

		log.Printf("[DeleteWorker] Executed delete: %s/%d (comment=%d, delay=%dm)",
			sd.BoTable, sd.WrID, sd.WrIsComment, sd.DelayMinutes)
	}
}
