package worker

import (
	"log"
	"sync"
	"time"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
)

// DeleteWorker processes pending scheduled deletes in the background
type DeleteWorker struct {
	writeRepo gnurepo.WriteRepository
	sdRepo    gnurepo.ScheduledDeleteRepository
	stop      chan struct{}
	wg        sync.WaitGroup
}

// NewDeleteWorker creates a new DeleteWorker
func NewDeleteWorker(writeRepo gnurepo.WriteRepository, sdRepo gnurepo.ScheduledDeleteRepository) *DeleteWorker {
	return &DeleteWorker{
		writeRepo: writeRepo,
		sdRepo:    sdRepo,
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
			// Soft delete comment
			if err := w.writeRepo.SoftDeleteComment(sd.BoTable, sd.WrID, sd.RequestedBy); err != nil {
				log.Printf("[DeleteWorker] Error soft deleting comment %s/%d: %v", sd.BoTable, sd.WrID, err)
				continue
			}
		} else {
			// Soft delete post (including its comments)
			if err := w.writeRepo.SoftDeletePost(sd.BoTable, sd.WrID, sd.RequestedBy); err != nil {
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
