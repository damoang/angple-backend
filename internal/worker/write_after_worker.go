package worker

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"time"

	pkgcache "github.com/damoang/angple-backend/pkg/cache"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/damoang/angple-backend/internal/service"
	"gorm.io/gorm"
)

type BlockedIDsProvider func(ctx context.Context, userID string) []string
type ClearPostMemCacheFunc func(boardSlug string)

type WriteAfterWorker struct {
	db             *gorm.DB
	cacheService   pkgcache.Service
	notiRepo       gnurepo.NotiRepository
	notiPrefRepo   gnurepo.NotiPreferenceRepository
	activitySync   *service.MemberActivitySyncService
	getBlockedIDs  BlockedIDsProvider
	clearPostCache ClearPostMemCacheFunc

	jobs chan any
	stop chan struct{}
	wg   sync.WaitGroup
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
	getBlockedIDs BlockedIDsProvider,
	clearPostCache ClearPostMemCacheFunc,
) *WriteAfterWorker {
	return &WriteAfterWorker{
		db:             db,
		cacheService:   cacheService,
		notiRepo:       notiRepo,
		notiPrefRepo:   notiPrefRepo,
		activitySync:   activitySync,
		getBlockedIDs:  getBlockedIDs,
		clearPostCache: clearPostCache,
		jobs:           make(chan any, 2048),
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
			for {
				select {
				case <-w.stop:
					return
				case job := <-w.jobs:
					w.handle(job)
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

func (w *WriteAfterWorker) Enqueue(job any) {
	select {
	case w.jobs <- job:
	default:
		log.Printf("[WriteAfterWorker] queue full, fallback async processing for %T", job)
		go w.handle(job)
	}
}

func (w *WriteAfterWorker) handle(job any) {
	switch j := job.(type) {
	case PostCreatedJob:
		w.handlePostCreated(j)
	case CommentCreatedJob:
		w.handleCommentCreated(j)
	default:
		log.Printf("[WriteAfterWorker] unknown job type %T", job)
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
