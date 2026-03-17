package worker

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"time"

	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	pkgcache "github.com/damoang/angple-backend/pkg/cache"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gorm.io/gorm"
)

type BlockedIDsProvider func(ctx context.Context, userID string) []string
type ClearPostMemCacheFunc func(boardSlug string)

var (
	writeAfterEventsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "write_after_events_processed_total",
			Help: "Total number of processed write-after events",
		},
		[]string{"event_type", "result"},
	)
	writeAfterEventsRetryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "write_after_events_retry_total",
			Help: "Total number of write-after event retries",
		},
		[]string{"event_type"},
	)
	writeAfterQueueDepth = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "write_after_events_queue_depth",
			Help: "Number of pending write-after events ready to process",
		},
	)
	writeAfterEventLagSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "write_after_event_lag_seconds",
			Help:    "Lag between event occurrence and processing attempt",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
		},
		[]string{"event_type"},
	)
)

type WriteAfterWorker struct {
	db             *gorm.DB
	cacheService   pkgcache.Service
	notiRepo       gnurepo.NotiRepository
	notiPrefRepo   gnurepo.NotiPreferenceRepository
	activitySync   *service.MemberActivitySyncService
	getBlockedIDs  BlockedIDsProvider
	clearPostCache ClearPostMemCacheFunc

	repo         gnurepo.WriteAfterEventRepository
	pollInterval time.Duration
	batchSize    int
	stop         chan struct{}
	wg           sync.WaitGroup
}

type PostCreatedJob struct {
	BoardSlug string
	WriteID   int
	MemberID  string
	Author    string
	Subject   string
	CreatedAt time.Time
}

type CommentCreatedJob struct {
	BoardSlug string
	WriteID   int
	PostID    int
	ParentID  *int
	MemberID  string
	Author    string
	CreatedAt time.Time
}

func NewWriteAfterWorker(
	db *gorm.DB,
	cacheService pkgcache.Service,
	notiRepo gnurepo.NotiRepository,
	notiPrefRepo gnurepo.NotiPreferenceRepository,
	activitySync *service.MemberActivitySyncService,
	repo gnurepo.WriteAfterEventRepository,
	getBlockedIDs BlockedIDsProvider,
	clearPostCache ClearPostMemCacheFunc,
) *WriteAfterWorker {
	return &WriteAfterWorker{
		db:             db,
		cacheService:   cacheService,
		notiRepo:       notiRepo,
		notiPrefRepo:   notiPrefRepo,
		activitySync:   activitySync,
		repo:           repo,
		getBlockedIDs:  getBlockedIDs,
		clearPostCache: clearPostCache,
		pollInterval:   2 * time.Second,
		batchSize:      100,
		stop:           make(chan struct{}),
	}
}

func (w *WriteAfterWorker) Start(concurrency int) {
	if concurrency < 1 {
		concurrency = 1
	}
	for i := 0; i < concurrency; i++ {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			ticker := time.NewTicker(w.pollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-w.stop:
					return
				case <-ticker.C:
					w.processBatch()
				}
			}
		}()
	}
	log.Printf("[WriteAfterWorker] Started with %d workers", concurrency)
}

func (w *WriteAfterWorker) Stop() {
	close(w.stop)
	w.wg.Wait()
	log.Printf("[WriteAfterWorker] Stopped")
}

func (w *WriteAfterWorker) processBatch() {
	if w.repo == nil {
		return
	}
	now := time.Now()
	if pendingCount, err := w.repo.CountPending(now); err == nil {
		writeAfterQueueDepth.Set(float64(pendingCount))
	}

	events, err := w.repo.ClaimPending(now, w.batchSize)
	if err != nil {
		log.Printf("[WriteAfterWorker] claim pending failed: %v", err)
		return
	}
	for _, event := range events {
		writeAfterEventLagSeconds.WithLabelValues(event.EventType).Observe(time.Since(event.OccurredAt).Seconds())
		if err := w.handleEvent(event); err != nil {
			writeAfterEventsProcessedTotal.WithLabelValues(event.EventType, "error").Inc()
			writeAfterEventsRetryTotal.WithLabelValues(event.EventType).Inc()
			if markErr := w.repo.MarkFailed(event.ID, gnurepo.TrimWriteAfterEventError(err)); markErr != nil {
				log.Printf("[WriteAfterWorker] mark failed %d: %v", event.ID, markErr)
			}
			continue
		}
		writeAfterEventsProcessedTotal.WithLabelValues(event.EventType, "success").Inc()
		if err := w.repo.MarkProcessed(event.ID); err != nil {
			log.Printf("[WriteAfterWorker] mark processed %d: %v", event.ID, err)
		}
	}
}

func (w *WriteAfterWorker) handleEvent(event gnudomain.WriteAfterEvent) error {
	switch event.EventType {
	case gnudomain.WriteAfterEventTypePostCreated:
		w.handlePostCreated(PostCreatedJob{
			BoardSlug: event.BoardSlug,
			WriteID:   event.WriteID,
			MemberID:  event.MemberID,
			Author:    event.Author,
			Subject:   event.Subject,
			CreatedAt: event.OccurredAt,
		})
		return nil
	case gnudomain.WriteAfterEventTypeCommentCreated:
		postID := 0
		if event.PostID != nil {
			postID = *event.PostID
		}
		w.handleCommentCreated(CommentCreatedJob{
			BoardSlug: event.BoardSlug,
			WriteID:   event.WriteID,
			PostID:    postID,
			ParentID:  event.ParentID,
			MemberID:  event.MemberID,
			Author:    event.Author,
			CreatedAt: event.OccurredAt,
		})
		return nil
	case gnudomain.WriteAfterEventTypePostUpdated, gnudomain.WriteAfterEventTypePostDeleted, gnudomain.WriteAfterEventTypePostRestored:
		w.handlePostChanged(event)
		return nil
	case gnudomain.WriteAfterEventTypeCommentUpdated, gnudomain.WriteAfterEventTypeCommentDeleted, gnudomain.WriteAfterEventTypeCommentRestored:
		return w.handleCommentChanged(event)
	default:
		return gnurepo.FormatUnknownWriteAfterEvent(event.EventType)
	}
}

func (w *WriteAfterWorker) handlePostCreated(job PostCreatedJob) {
	if w.cacheService != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = w.cacheService.InvalidatePosts(ctx, job.BoardSlug)
		cancel()
	}
	if w.clearPostCache != nil {
		w.clearPostCache(job.BoardSlug)
	}
	if w.activitySync != nil {
		if err := w.activitySync.SyncLegacyPost(job.BoardSlug, job.WriteID); err != nil {
			log.Printf("[WriteAfterWorker] activity sync failed for post %s/%d: %v", job.BoardSlug, job.WriteID, err)
		}
	}
	if w.db == nil || w.notiRepo == nil || w.notiPrefRepo == nil {
		return
	}

	authorName := job.Author
	if authorName == "" {
		authorName = job.MemberID
	}

	var followerIDs []string
	w.db.Table("g5_member_follow").Select("mb_id").Where("target_id = ?", job.MemberID).Pluck("mb_id", &followerIDs)
	for _, fid := range followerIDs {
		if pref, _ := w.notiPrefRepo.Get(fid); !pref.NotiFollow {
			continue
		}
		_ = w.notiRepo.Create(&gnurepo.Notification{
			PhToCase: "follow", PhFromCase: "write", BoTable: job.BoardSlug,
			WrID: job.WriteID, MbID: fid, RelMbID: job.MemberID,
			RelMbNick:  authorName,
			RelMsg:     fmt.Sprintf("%s님이 새 글을 작성했습니다: %s", authorName, job.Subject),
			RelURL:     fmt.Sprintf("/%s/%d", job.BoardSlug, job.WriteID),
			PhReaded:   "N",
			PhDatetime: job.CreatedAt,
			WrParent:   job.WriteID,
		})
	}

	var subscriberIDs []string
	w.db.Table("g5_board_subscribe").Select("mb_id").Where("bo_table = ? AND mb_id != ?", job.BoardSlug, job.MemberID).Pluck("mb_id", &subscriberIDs)
	followerSet := make(map[string]bool, len(followerIDs))
	for _, fid := range followerIDs {
		followerSet[fid] = true
	}
	for _, sid := range subscriberIDs {
		if followerSet[sid] {
			continue
		}
		if pref, _ := w.notiPrefRepo.Get(sid); !pref.NotiFollow {
			continue
		}
		_ = w.notiRepo.Create(&gnurepo.Notification{
			PhToCase: "subscribe", PhFromCase: "write", BoTable: job.BoardSlug,
			WrID: job.WriteID, MbID: sid, RelMbID: job.MemberID,
			RelMbNick:  authorName,
			RelMsg:     fmt.Sprintf("%s 게시판에 새 글: %s", job.BoardSlug, job.Subject),
			RelURL:     fmt.Sprintf("/%s/%d", job.BoardSlug, job.WriteID),
			PhReaded:   "N",
			PhDatetime: job.CreatedAt,
			WrParent:   job.WriteID,
		})
	}
}

func (w *WriteAfterWorker) handleCommentCreated(job CommentCreatedJob) {
	if w.cacheService != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = w.cacheService.InvalidateComments(ctx, job.BoardSlug, job.PostID)
		_ = w.cacheService.InvalidatePosts(ctx, job.BoardSlug)
		cancel()
	}
	if w.clearPostCache != nil {
		w.clearPostCache(job.BoardSlug)
	}
	if w.activitySync != nil {
		if err := w.activitySync.SyncLegacyComment(job.BoardSlug, job.WriteID); err != nil {
			log.Printf("[WriteAfterWorker] activity sync failed for comment %s/%d: %v", job.BoardSlug, job.WriteID, err)
		}
	}
	if w.db == nil || w.notiRepo == nil || w.notiPrefRepo == nil {
		return
	}

	tableName := "g5_write_" + job.BoardSlug
	var postAuthor struct {
		MbID      string `gorm:"column:mb_id"`
		WrSubject string `gorm:"column:wr_subject"`
	}
	if err := w.db.Table(tableName).Select("mb_id, wr_subject").Where("wr_id = ? AND wr_is_comment = 0", job.PostID).Scan(&postAuthor).Error; err != nil || postAuthor.MbID == "" {
		return
	}

	if job.ParentID != nil && *job.ParentID > 0 {
		var parentAuthorMbID string
		if err := w.db.Table(tableName).Select("mb_id").Where("wr_id = ?", *job.ParentID).Scan(&parentAuthorMbID).Error; err == nil && parentAuthorMbID != "" && parentAuthorMbID != job.MemberID {
			if !w.isBlocked(parentAuthorMbID, job.MemberID) {
				if pref, _ := w.notiPrefRepo.Get(parentAuthorMbID); pref.NotiReply {
					_ = w.notiRepo.Create(&gnurepo.Notification{
						PhToCase:      "comment_reply",
						PhFromCase:    "comment",
						BoTable:       job.BoardSlug,
						WrID:          job.WriteID,
						MbID:          parentAuthorMbID,
						RelMbID:       job.MemberID,
						RelMbNick:     job.Author,
						RelMsg:        fmt.Sprintf("%s님이 회원님의 댓글에 답글을 남겼습니다.", job.Author),
						RelURL:        fmt.Sprintf("/%s/%d#comment_%d", job.BoardSlug, job.PostID, job.WriteID),
						PhReaded:      "N",
						PhDatetime:    job.CreatedAt,
						ParentSubject: postAuthor.WrSubject,
						WrParent:      job.PostID,
					})
				}
			}
		}
	}

	if postAuthor.MbID == job.MemberID || w.isBlocked(postAuthor.MbID, job.MemberID) {
		return
	}
	if pref, _ := w.notiPrefRepo.Get(postAuthor.MbID); pref.NotiComment {
		_ = w.notiRepo.Create(&gnurepo.Notification{
			PhToCase:      "comment",
			PhFromCase:    "comment",
			BoTable:       job.BoardSlug,
			WrID:          job.WriteID,
			MbID:          postAuthor.MbID,
			RelMbID:       job.MemberID,
			RelMbNick:     job.Author,
			RelMsg:        fmt.Sprintf("%s님이 회원님의 글에 댓글을 남겼습니다.", job.Author),
			RelURL:        fmt.Sprintf("/%s/%d#comment_%d", job.BoardSlug, job.PostID, job.WriteID),
			PhReaded:      "N",
			PhDatetime:    job.CreatedAt,
			ParentSubject: postAuthor.WrSubject,
			WrParent:      job.PostID,
		})
	}
}

func (w *WriteAfterWorker) handlePostChanged(event gnudomain.WriteAfterEvent) {
	if w.cacheService != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = w.cacheService.InvalidatePost(ctx, event.BoardSlug, event.WriteID)
		_ = w.cacheService.InvalidatePosts(ctx, event.BoardSlug)
		cancel()
	}
	if w.clearPostCache != nil {
		w.clearPostCache(event.BoardSlug)
	}
	if w.activitySync != nil {
		if err := w.activitySync.SyncLegacyPost(event.BoardSlug, event.WriteID); err != nil {
			log.Printf("[WriteAfterWorker] activity sync failed for post %s/%d: %v", event.BoardSlug, event.WriteID, err)
		}
	}
}

func (w *WriteAfterWorker) handleCommentChanged(event gnudomain.WriteAfterEvent) error {
	if event.PostID == nil {
		return fmt.Errorf("missing post_id for comment event %s/%d", event.BoardSlug, event.WriteID)
	}
	if w.cacheService != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = w.cacheService.InvalidateComments(ctx, event.BoardSlug, *event.PostID)
		_ = w.cacheService.InvalidatePosts(ctx, event.BoardSlug)
		cancel()
	}
	if w.clearPostCache != nil {
		w.clearPostCache(event.BoardSlug)
	}
	if w.activitySync != nil {
		if err := w.activitySync.SyncLegacyComment(event.BoardSlug, event.WriteID); err != nil {
			log.Printf("[WriteAfterWorker] activity sync failed for comment %s/%d: %v", event.BoardSlug, event.WriteID, err)
		}
	}
	return nil
}

func (w *WriteAfterWorker) isBlocked(targetUserID, actorUserID string) bool {
	if w.getBlockedIDs == nil || targetUserID == "" || actorUserID == "" {
		return false
	}
	return slices.Contains(w.getBlockedIDs(context.Background(), targetUserID), actorUserID)
}

func ClearPostMemCache(postMemCache *sync.Map) func(string) {
	return func(slug string) {
		if postMemCache == nil {
			return
		}
		postMemCache.Range(func(key, value interface{}) bool {
			keyStr, ok := key.(string)
			if ok && strings.HasPrefix(keyStr, "posts:"+slug+":") {
				postMemCache.Delete(key)
			}
			return true
		})
	}
}
