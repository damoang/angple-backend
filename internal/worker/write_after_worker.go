package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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
	writeAfterActivitySyncFailuresTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "write_after_activity_sync_failures_total",
			Help: "Total number of member activity sync failures inside write-after processing",
		},
		[]string{"event_type"},
	)
	// 재시도 상한을 넘겨 포기한 이벤트. ⛔ 이 값이 늘면 "조용히 버려지는 중"이라는 뜻이다 —
	// 0 이 정상이고, 오르면 원인을 봐야 한다(원글 삭제·엔드포인트 장애 등).
	writeAfterEventDeadTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "write_after_events_dead_total",
			Help: "Total number of write-after events abandoned after exceeding the retry cap",
		},
		[]string{"event_type"},
	)
	// 흘린 processing 행을 회수한 수. ⛔ 평시 0 에 가까워야 한다. 재시작마다 크게 튀면
	// graceful shutdown 이 동작하지 않는다는 신호다.
	writeAfterReclaimedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "write_after_events_reclaimed_total",
			Help: "Total number of stale processing events reclaimed back to pending",
		},
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

	repo           gnurepo.WriteAfterEventRepository
	pollInterval   time.Duration
	batchSize      int
	stop           chan struct{}
	wg             sync.WaitGroup
	httpClient     *http.Client
	webBaseURL     string
	internalSecret string
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
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		webBaseURL:     strings.TrimRight(getAffiliateSyncBaseURL(), "/"),
		internalSecret: os.Getenv("INTERNAL_SECRET"),
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
	// stale processing 회수 스윕 — 기동 직후 1회 + 이후 주기적으로.
	// ⛔ graceful shutdown 을 넣어도 OOM·SIGKILL·노드 장애는 남는다. 이 그물이 마지막 방어선이다.
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.reclaimStale() // 기동 직후 — 직전 종료에서 흘린 분을 즉시 회수
		ticker := time.NewTicker(reclaimInterval)
		defer ticker.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-ticker.C:
				w.reclaimStale()
			}
		}
	}()

	log.Printf("[WriteAfterWorker] Started with %d workers", concurrency)
}

// reclaimStale 은 claimed_at 이 오래된 processing 행을 pending 으로 되돌린다.
// ⛔ staleAfter 는 한 배치의 최악 처리시간보다 넉넉해야 한다. 짧으면 **정상 처리 중인**
//
//	이벤트를 회수해 중복 실행이 된다(제휴 sync 는 멱등이지만 다른 이벤트는 아닐 수 있다).
//	batchSize 100 × httpClient 타임아웃 5s = 최악 500s 이므로 15분으로 잡는다.
func (w *WriteAfterWorker) reclaimStale() {
	if w.repo == nil {
		return
	}
	n, err := w.repo.ReclaimStaleProcessing(time.Now().Add(-staleAfter), reclaimLimit)
	if err != nil {
		log.Printf("[WriteAfterWorker] reclaim stale failed: %v", err)
		return
	}
	if n > 0 {
		writeAfterReclaimedTotal.Add(float64(n))
		log.Printf("[WriteAfterWorker] reclaimed %d stale processing events", n)
	}
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
			if markErr := w.markFailed(event, err); markErr != nil {
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
	case gnudomain.WriteAfterEventTypeAffiliatePostSync,
		gnudomain.WriteAfterEventTypeAffiliateCommentSync,
		gnudomain.WriteAfterEventTypeAffiliatePostDelete,
		gnudomain.WriteAfterEventTypeAffiliateCommentDelete:
		return w.handleAffiliateEvent(event)
	default:
		return gnurepo.FormatUnknownWriteAfterEvent(event.EventType)
	}
}

// getAffiliateSyncBaseURL 은 제휴 동기화를 보낼 web 주소를 고른다.
//
// ⛔ 공용 도메인 폴백은 **조용한 고장**을 만든다. 2026-08-08~08-21, 운영에
//
//	WEB_INTERNAL_URL·WEB_BASE_URL 이 둘 다 없어 이 폴백이 걸렸고, 내부 호출이
//	파드 → 공용 인터넷 → Cloudflare → CloudFront → ALB → web 을 한 바퀴 돌았다.
//	엣지에서 X-Internal-Secret 헤더가 유실돼 403, 공개 rate limiter 에 429 —
//	**13일간 성공 0건**, pending 17.9만 건, 오리진 트래픽의 12% 를 먹었다.
//	아무도 몰랐던 이유는 이 폴백이 아무 소리도 내지 않았기 때문이다.
//
// 그래서 폴백 자체는 남기되(로컬 개발 호환) **반드시 흔적을 남긴다.**
func getAffiliateSyncBaseURL() string {
	if url := strings.TrimSpace(os.Getenv("WEB_INTERNAL_URL")); url != "" {
		return url
	}
	if url := strings.TrimSpace(os.Getenv("WEB_BASE_URL")); url != "" {
		return url
	}
	const publicFallback = "https://damoang.net"
	log.Printf("[WriteAfterWorker] WEB_INTERNAL_URL/WEB_BASE_URL 미설정 — 공용 도메인(%s)으로 폴백한다. "+
		"운영에서 이 로그가 보이면 배선 결함이다: 내부 호출이 CDN 을 경유해 403/429 로 전량 실패한다.",
		publicFallback)
	return publicFallback
}

func (w *WriteAfterWorker) markFailed(event gnudomain.WriteAfterEvent, err error) error {
	if w.repo == nil {
		return nil
	}
	if isAffiliateEventType(event.EventType) {
		// ⛔ 재시도 상한 — 없으면 영구 실패 이벤트(원글 삭제 등)가 큐에 남아 영원히 돈다.
		//    2026-08-21 실측: retry_count 최대 1,961회. 17.9만 건이 시간당 재시도하며
		//    오리진 트래픽의 12% 를 먹고 CDN 요금을 3중으로 물렸다.
		if event.RetryCount >= gnudomain.WriteAfterEventMaxRetry {
			writeAfterEventDeadTotal.WithLabelValues(event.EventType).Inc()
			log.Printf("[WriteAfterWorker] giving up after %d retries: %s id=%d: %v",
				event.RetryCount, event.EventType, event.ID, err)
			return w.repo.MarkDead(event.ID, gnurepo.TrimWriteAfterEventError(err))
		}
		delay := affiliateRetryDelay(event.RetryCount)
		return w.repo.MarkFailedWithDelay(event.ID, gnurepo.TrimWriteAfterEventError(err), delay)
	}
	return w.repo.MarkFailed(event.ID, gnurepo.TrimWriteAfterEventError(err))
}

func isAffiliateEventType(eventType string) bool {
	switch eventType {
	case gnudomain.WriteAfterEventTypeAffiliatePostSync,
		gnudomain.WriteAfterEventTypeAffiliateCommentSync,
		gnudomain.WriteAfterEventTypeAffiliatePostDelete,
		gnudomain.WriteAfterEventTypeAffiliateCommentDelete:
		return true
	default:
		return false
	}
}

const (
	// staleAfter 는 이 시간이 지나도 processing 인 행을 "흘린 것"으로 본다.
	// batchSize(100) × httpClient 타임아웃(5s) = 최악 500s 보다 넉넉해야 한다.
	staleAfter = 15 * time.Minute
	// reclaimInterval 은 회수 스윕 주기다.
	reclaimInterval = 5 * time.Minute
	// reclaimLimit 는 한 번에 되돌릴 최대 건수 — 대량 회수가 그 자체로 부하가 되지 않게 자른다.
	reclaimLimit = 500
)

func affiliateRetryDelay(retryCount int) time.Duration {
	switch retryCount {
	case 0:
		return time.Minute
	case 1:
		return 10 * time.Minute
	default:
		return time.Hour
	}
}

func (w *WriteAfterWorker) handleAffiliateEvent(event gnudomain.WriteAfterEvent) error {
	if w.httpClient == nil || w.webBaseURL == "" {
		return fmt.Errorf("affiliate sync client not configured")
	}
	payload := map[string]interface{}{
		"boardId": event.BoardSlug,
	}

	switch event.EventType {
	case gnudomain.WriteAfterEventTypeAffiliatePostSync:
		payload["entity"] = "post"
		payload["action"] = "sync"
		payload["postId"] = event.WriteID
	case gnudomain.WriteAfterEventTypeAffiliateCommentSync:
		if event.PostID == nil {
			return fmt.Errorf("missing post_id for affiliate comment sync %s/%d", event.BoardSlug, event.WriteID)
		}
		payload["entity"] = "comment"
		payload["action"] = "sync"
		payload["postId"] = *event.PostID
		payload["commentId"] = event.WriteID
	case gnudomain.WriteAfterEventTypeAffiliatePostDelete:
		payload["entity"] = "post"
		payload["action"] = "delete"
		payload["postId"] = event.WriteID
	case gnudomain.WriteAfterEventTypeAffiliateCommentDelete:
		if event.PostID == nil {
			return fmt.Errorf("missing post_id for affiliate comment delete %s/%d", event.BoardSlug, event.WriteID)
		}
		payload["entity"] = "comment"
		payload["action"] = "delete"
		payload["postId"] = *event.PostID
		payload["commentId"] = event.WriteID
	default:
		return gnurepo.FormatUnknownWriteAfterEvent(event.EventType)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, w.webBaseURL+"/api/internal/affiliate/sync", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", w.internalSecret)
	req.Header.Set("User-Agent", "angple-backend-affiliate-worker/1.0")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("affiliate sync returned %d", resp.StatusCode)
	}

	return nil
}

// isPrivateBoard 는 일반 회원(기본 level 1)이 목록 또는 읽기를 할 수 없는 보드인지 판정한다.
// bo_list_level 또는 bo_read_level 이 1 을 초과하면(=운영/숨김 보드) 비공개로 본다.
// 조회 실패 시 false(기존 동작=알림 발송) 로 안전 폴백해 정상 보드 알림이 끊기지 않게 한다.
func (w *WriteAfterWorker) isPrivateBoard(boTable string) bool {
	var b struct {
		ListLevel int `gorm:"column:bo_list_level"`
		ReadLevel int `gorm:"column:bo_read_level"`
	}
	if err := w.db.Table("g5_board").
		Select("bo_list_level, bo_read_level").
		Where("bo_table = ?", boTable).
		Scan(&b).Error; err != nil {
		return false
	}
	return b.ListLevel > 1 || b.ReadLevel > 1
}

//nolint:gocyclo // Notification routing for post creation is branching-heavy business logic and stays explicit on purpose.
func (w *WriteAfterWorker) handlePostCreated(job PostCreatedJob) {
	if w.cacheService != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		w.logCacheError("InvalidatePosts", w.cacheService.InvalidatePosts(ctx, job.BoardSlug))
		cancel()
	}
	if w.clearPostCache != nil {
		w.clearPostCache(job.BoardSlug)
	}
	if w.activitySync != nil {
		if err := w.activitySync.SyncLegacyPost(job.BoardSlug, job.WriteID); err != nil {
			writeAfterActivitySyncFailuresTotal.WithLabelValues(gnudomain.WriteAfterEventTypePostCreated).Inc()
			log.Printf("[WriteAfterWorker] activity sync failed for post %s/%d: %v", job.BoardSlug, job.WriteID, err)
		}
	}
	// 소모임 개설 신청 안내 — newgroup 새 글에 "공감 100개" 안내 댓글 (멱등, newgroup/1193)
	if job.BoardSlug == "newgroup" && w.db != nil {
		if err := service.PostNewgroupGuideComment(w.db, job.WriteID); err != nil {
			log.Printf("[WriteAfterWorker] newgroup guide comment failed for %d: %v", job.WriteID, err)
		}
	}
	if w.db == nil || w.notiRepo == nil || w.notiPrefRepo == nil {
		return
	}

	authorName := job.Author
	if authorName == "" {
		authorName = job.MemberID
	}

	// 비공개/숨김 보드(일반 회원이 목록 또는 읽기 불가)는 팔로우·구독 알림을 보내지 않는다.
	// 접근 못 하는 회원에게 글 존재/링크가 노출되는 문제 방지(예: test, adm 등 운영 보드).
	// 기준은 permission_checker 의 CanList/CanRead 와 동일 — 기본 회원(level 1)이
	// list 또는 read 못 하면 비공개로 간주. 캐시 무효화·activity sync 는 위에서 이미 수행됨.
	if w.isPrivateBoard(job.BoardSlug) {
		return
	}

	// 작성자를 차단한 회원에게는 새 글 알림을 보내지 않는다 (bug/13252).
	//
	// 화면(목록·댓글)은 차단을 걸러 주는데 알림 발송은 거르지 않아, 차단한 회원의
	// 글이 알림으로 되살아났다 — "차단이 풀렸나 해서 찾아봤다"는 제보 그대로다.
	// 실증: 제보 계정에 차단 작성자의 write 알림 2건 존재.
	//
	// ⛔ block_scope='all' 만 대상이다. 'message'(쪽지 한정 차단, 33건)는 글 알림과
	//    무관하므로 거르면 안 된다.
	blockedBy := make(map[string]bool)
	{
		var ids []string
		w.db.Table("g5_member_block").Select("mb_id").
			Where("blocked_mb_id = ? AND block_scope = 'all'", job.MemberID).
			Pluck("mb_id", &ids)
		for _, id := range ids {
			blockedBy[id] = true
		}
	}

	var followerIDs []string
	w.db.Table("g5_member_follow").Select("mb_id").Where("target_id = ?", job.MemberID).Pluck("mb_id", &followerIDs)
	for _, fid := range followerIDs {
		if blockedBy[fid] {
			continue
		}
		pref, ok := w.mustGetNotiPreference(fid)
		if !ok || !pref.NotiFollow {
			continue
		}
		w.createNotification(&gnurepo.Notification{
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

	// level=1(전체)만 글 작성 시 알림. level=2(인기글만)는 추천 임계값 도달 시
	// 인기글 트리거 cron 이 1회 알림 (#12607 — 자유게시판 등 폭주 방지).
	var subscriberIDs []string
	w.db.Table("g5_board_subscribe").Select("mb_id").Where("bo_table = ? AND mb_id != ? AND level = 1", job.BoardSlug, job.MemberID).Pluck("mb_id", &subscriberIDs)
	followerSet := make(map[string]bool, len(followerIDs))
	for _, fid := range followerIDs {
		followerSet[fid] = true
	}
	for _, sid := range subscriberIDs {
		if followerSet[sid] {
			continue
		}
		// 차단 필터 — 위 blockedBy 주석 참조 (bug/13252)
		if blockedBy[sid] {
			continue
		}
		pref, ok := w.mustGetNotiPreference(sid)
		if !ok || !pref.NotiBoardSubscribe {
			continue
		}
		w.createNotification(&gnurepo.Notification{
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

//nolint:gocyclo // Notification routing for comments is branching-heavy business logic and stays explicit on purpose.
func (w *WriteAfterWorker) handleCommentCreated(job CommentCreatedJob) {
	if w.cacheService != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		w.logCacheError("InvalidateComments", w.cacheService.InvalidateComments(ctx, job.BoardSlug, job.PostID))
		w.logCacheError("InvalidatePosts", w.cacheService.InvalidatePosts(ctx, job.BoardSlug))
		cancel()
	}
	if w.clearPostCache != nil {
		w.clearPostCache(job.BoardSlug)
	}
	if w.activitySync != nil {
		if err := w.activitySync.SyncLegacyComment(job.BoardSlug, job.WriteID); err != nil {
			writeAfterActivitySyncFailuresTotal.WithLabelValues(gnudomain.WriteAfterEventTypeCommentCreated).Inc()
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

	// 답글 알림을 이미 받은 사람에게 글 알림을 또 보내지 않기 위한 표식.
	// ⛔ 원글 작성자가 자기 글에 댓글을 달고 거기 답글이 달리면, 한 번의 사건으로
	//    답글 알림 + 댓글 알림이 **같은 사람에게 두 개** 간다(bug/13206, 하루 200~300건).
	//    레거시 PHP 에는 이 가드가 있었는데(hook.lib.php:1186) Go 이식 때 빠졌다.
	repliedToPostAuthor := false

	if job.ParentID != nil && *job.ParentID > 0 {
		var parentAuthorMbID string
		if err := w.db.Table(tableName).Select("mb_id").Where("wr_id = ?", *job.ParentID).Scan(&parentAuthorMbID).Error; err == nil && parentAuthorMbID != "" && parentAuthorMbID != job.MemberID {
			if !w.isBlocked(parentAuthorMbID, job.MemberID) {
				if pref, ok := w.mustGetNotiPreference(parentAuthorMbID); ok && pref.NotiReply {
					if parentAuthorMbID == postAuthor.MbID {
						repliedToPostAuthor = true
					}
					w.createNotification(&gnurepo.Notification{
						PhToCase:      "comment_reply",
						PhFromCase:    "comment",
						BoTable:       job.BoardSlug,
						WrID:          job.WriteID,
						MbID:          parentAuthorMbID,
						RelMbID:       job.MemberID,
						RelMbNick:     job.Author,
						RelMsg:        fmt.Sprintf("%s님이 회원님의 댓글에 답글을 남겼습니다.", job.Author),
						RelURL:        fmt.Sprintf("/%s/%d#c_%d", job.BoardSlug, job.PostID, job.WriteID),
						PhReaded:      "N",
						PhDatetime:    job.CreatedAt,
						ParentSubject: postAuthor.WrSubject,
						WrParent:      job.PostID,
					})
				}
			}
		}
	}

	// 같은 사건으로 답글 알림을 이미 보냈으면 글 알림은 생략한다 — 더 구체적인 쪽(답글)이 남는다.
	if repliedToPostAuthor {
		return
	}
	if postAuthor.MbID == job.MemberID || w.isBlocked(postAuthor.MbID, job.MemberID) {
		return
	}
	if pref, ok := w.mustGetNotiPreference(postAuthor.MbID); ok && pref.NotiComment {
		w.createNotification(&gnurepo.Notification{
			PhToCase:      "comment",
			PhFromCase:    "comment",
			BoTable:       job.BoardSlug,
			WrID:          job.WriteID,
			MbID:          postAuthor.MbID,
			RelMbID:       job.MemberID,
			RelMbNick:     job.Author,
			RelMsg:        fmt.Sprintf("%s님이 회원님의 글에 댓글을 남겼습니다.", job.Author),
			RelURL:        fmt.Sprintf("/%s/%d#c_%d", job.BoardSlug, job.PostID, job.WriteID),
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
		w.logCacheError("InvalidatePost", w.cacheService.InvalidatePost(ctx, event.BoardSlug, event.WriteID))
		w.logCacheError("InvalidatePosts", w.cacheService.InvalidatePosts(ctx, event.BoardSlug))
		cancel()
	}
	if w.clearPostCache != nil {
		w.clearPostCache(event.BoardSlug)
	}
	if w.activitySync != nil {
		if err := w.activitySync.SyncLegacyPost(event.BoardSlug, event.WriteID); err != nil {
			writeAfterActivitySyncFailuresTotal.WithLabelValues(event.EventType).Inc()
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
		w.logCacheError("InvalidateComments", w.cacheService.InvalidateComments(ctx, event.BoardSlug, *event.PostID))
		w.logCacheError("InvalidatePosts", w.cacheService.InvalidatePosts(ctx, event.BoardSlug))
		cancel()
	}
	if w.clearPostCache != nil {
		w.clearPostCache(event.BoardSlug)
	}
	if w.activitySync != nil {
		if err := w.activitySync.SyncLegacyComment(event.BoardSlug, event.WriteID); err != nil {
			writeAfterActivitySyncFailuresTotal.WithLabelValues(event.EventType).Inc()
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
		postMemCache.Range(func(key, _ interface{}) bool {
			keyStr, ok := key.(string)
			if ok && strings.HasPrefix(keyStr, "posts:"+slug+":") {
				postMemCache.Delete(key)
			}
			return true
		})
	}
}

func (w *WriteAfterWorker) logCacheError(operation string, err error) {
	if err != nil {
		log.Printf("[WriteAfterWorker] cache %s failed: %v", operation, err)
	}
}

func (w *WriteAfterWorker) mustGetNotiPreference(mbID string) (*gnurepo.NotiPreference, bool) {
	pref, err := w.notiPrefRepo.Get(mbID)
	if err != nil {
		log.Printf("[WriteAfterWorker] notification preference lookup failed for %s: %v", mbID, err)
		return nil, false
	}
	return pref, true
}

func (w *WriteAfterWorker) createNotification(noti *gnurepo.Notification) {
	// 중복 가드: 알림 INSERT 후 MarkProcessed 전에 프로세스가 죽으면 이벤트가
	// 재클레임되어 팔로워·구독자 전원에게 같은 알림이 재발행된다.
	// idx_noti_dedup (bo_table, wr_id, rel_mb_id, ph_from_case) 로 싸게 걸러진다.
	if exists, err := w.notiRepo.Exists(noti.MbID, noti.BoTable, noti.WrID, noti.PhFromCase, noti.RelMbID); err == nil && exists {
		return
	}
	if err := w.notiRepo.Create(noti); err != nil {
		log.Printf("[WriteAfterWorker] notification create failed for %s/%d: %v", noti.BoTable, noti.WrID, err)
	}
}
