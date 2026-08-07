package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"github.com/damoang/angple-backend/internal/handler"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"gorm.io/gorm"
)

// PushNotifyWorker turns in-app notifications (g5_na_noti) into device push
// notifications via the Expo Push API. The notification table acts as an
// outbox: this worker polls rows past a persisted cursor (v2_push_cursors),
// resolves recipients' registered devices (v2_devices) and sends batched
// pushes. It never touches the request path — failures are logged and the
// in-app notification remains the source of truth.
//
// Enabled only when PUSH_ENABLED=true (or 1). Optional EXPO_ACCESS_TOKEN is
// sent as a bearer token (Expo "enhanced push security").
type PushNotifyWorker struct {
	db           *gorm.DB
	pollInterval time.Duration
	batchSize    int
	// maxPerCycle caps how many push messages one poll cycle may send.
	// Guards against cron-generated notification floods and Expo rate limits.
	maxPerCycle int
	httpClient  *http.Client
	accessToken string
	stop        chan struct{}
	wg          sync.WaitGroup
}

const expoPushURL = "https://exp.host/--/api/v2/push/send"

// pushFromCases is the MVP whitelist: person-to-person events only.
// System/cron notifications (digest, point expiry, promote 등) are excluded
// on purpose — they can be generated thousands at a time.
var pushFromCases = map[string]bool{
	"board":   true, // 내 글에 댓글
	"comment": true, // 내 댓글에 답글
	"reply":   true, // 답글
	"mention": true, // 멘션
	"good":    true, // 추천
	"memo":    true, // 쪽지
}

func NewPushNotifyWorker(db *gorm.DB) *PushNotifyWorker {
	return &PushNotifyWorker{
		db:           db,
		pollInterval: 3 * time.Second,
		batchSize:    500,
		maxPerCycle:  1000,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		accessToken:  os.Getenv("EXPO_ACCESS_TOKEN"),
		stop:         make(chan struct{}),
	}
}

// Start launches the single poller goroutine. A single worker is intentional:
// the cursor is a global watermark and one poller avoids double-send races.
func (w *PushNotifyWorker) Start() {
	// 커서 테이블은 워커가 직접 보장한다 — AUTO_MIGRATE_ON_BOOT=false 인
	// 환경(canary/prod)에서 전체 스키마 마이그레이션 없이도 동작하도록.
	// AutoMigrate 는 멱등이고 이 한 테이블만 만진다.
	if err := w.db.AutoMigrate(&v2domain.V2PushCursor{}); err != nil {
		log.Printf("[PushNotifyWorker] cursor table migrate failed: %v", err)
	}
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
	log.Printf("[PushNotifyWorker] Started (interval=%s batch=%d cap=%d)", w.pollInterval, w.batchSize, w.maxPerCycle)
}

func (w *PushNotifyWorker) Stop() {
	close(w.stop)
	w.wg.Wait()
	log.Printf("[PushNotifyWorker] Stopped")
}

// loadCursor returns the watermark, initializing it to MAX(ph_id) on first run
// so a fresh deploy never blasts historical notifications.
func (w *PushNotifyWorker) loadCursor() (int, bool) {
	var cur v2domain.V2PushCursor
	err := w.db.First(&cur, "id = 1").Error
	if err == nil {
		return cur.LastPhID, true
	}
	if err != gorm.ErrRecordNotFound {
		log.Printf("[PushNotifyWorker] cursor load failed: %v", err)
		return 0, false
	}
	var maxID int
	if err := w.db.Model(&gnurepo.Notification{}).Select("COALESCE(MAX(ph_id), 0)").Scan(&maxID).Error; err != nil {
		log.Printf("[PushNotifyWorker] cursor init (max ph_id) failed: %v", err)
		return 0, false
	}
	cur = v2domain.V2PushCursor{ID: 1, LastPhID: maxID}
	if err := w.db.Create(&cur).Error; err != nil {
		log.Printf("[PushNotifyWorker] cursor init insert failed: %v", err)
		return 0, false
	}
	log.Printf("[PushNotifyWorker] cursor initialized at ph_id=%d", maxID)
	return maxID, true
}

// claimCursor advances the watermark old→new with an optimistic guard.
// Returns false when another poller instance already advanced it (multi-replica
// safety: the loser skips sending, so notifications are never double-pushed).
// Claiming BEFORE send also gives crash semantics of "miss over duplicate".
func (w *PushNotifyWorker) claimCursor(oldPhID, newPhID int) bool {
	res := w.db.Model(&v2domain.V2PushCursor{}).
		Where("id = 1 AND last_ph_id = ?", oldPhID).
		Update("last_ph_id", newPhID)
	if res.Error != nil {
		log.Printf("[PushNotifyWorker] cursor claim failed at ph_id=%d: %v", newPhID, res.Error)
		return false
	}
	if res.RowsAffected == 0 {
		log.Printf("[PushNotifyWorker] cursor claim lost (another instance active?) old=%d", oldPhID)
		return false
	}
	return true
}

type expoPushMessage struct {
	To        string            `json:"to"`
	Title     string            `json:"title"`
	Body      string            `json:"body,omitempty"`
	Data      map[string]string `json:"data,omitempty"`
	Sound     string            `json:"sound,omitempty"`
	ChannelID string            `json:"channelId,omitempty"`
	Priority  string            `json:"priority,omitempty"`
}

type expoPushTicket struct {
	Status  string `json:"status"` // "ok" | "error"
	Message string `json:"message,omitempty"`
	Details struct {
		Error string `json:"error,omitempty"`
	} `json:"details,omitempty"`
}

func (w *PushNotifyWorker) processBatch() {
	cursor, ok := w.loadCursor()
	if !ok {
		return
	}

	// Settle window: auto-increment ids can commit out of order — a row with a
	// smaller ph_id may become visible after we've already advanced past it.
	// Only process rows older than 2s so late commits aren't skipped forever.
	//
	// ph_readed: 이미 읽은 알림(웹/앱에서 먼저 확인)은 푸시하지 않는다 — 읽음 동기화.
	// 읽힌 행도 커서는 정상 전진한다(발송만 스킵이 아니라 조회에서 제외돼도,
	// 커서는 배치의 마지막 ph_id 가 아닌 "조회된" 행 기준이므로 아래 주의:
	// 조회에서 빼면 그 행 위로 커서가 못 넘어가 무한 재조회된다. 그래서
	// WHERE 가 아니라 발송 단계에서 스킵한다.
	settled := time.Now().Add(-2 * time.Second)
	var notis []gnurepo.Notification
	if err := w.db.Where("ph_id > ? AND ph_datetime <= ?", cursor, settled).
		Order("ph_id ASC").Limit(w.batchSize).Find(&notis).Error; err != nil {
		log.Printf("[PushNotifyWorker] outbox query failed: %v", err)
		return
	}
	if len(notis) == 0 {
		return
	}

	// Resolve recipient mb_id → v2 user id → device tokens (2 queries, no N+1).
	mbSet := make(map[string]struct{})
	for _, n := range notis {
		if pushFromCases[n.PhFromCase] && n.MbID != "" {
			mbSet[n.MbID] = struct{}{}
		}
	}
	tokensByMb := w.resolveDeviceTokens(mbSet)

	// Build messages in ph_id order, respecting the per-cycle cap. If the cap
	// would be crossed mid-notification, stop there and leave the rest for the
	// next cycle (cursor only advances past fully processed rows).
	var msgs []expoPushMessage
	lastProcessed := cursor
	for _, n := range notis {
		var candidate []expoPushMessage
		// 화이트리스트 + 미읽음만 발송. 읽힌 행도 커서는 전진(스킵만).
		if pushFromCases[n.PhFromCase] && n.PhReaded != "Y" {
			for _, tok := range tokensByMb[n.MbID] {
				candidate = append(candidate, w.buildMessage(n, tok))
			}
		}
		if len(msgs)+len(candidate) > w.maxPerCycle && lastProcessed > cursor {
			break
		}
		msgs = append(msgs, candidate...)
		lastProcessed = n.PhID
	}

	if lastProcessed == cursor {
		return
	}
	// Claim first (multi-instance guard + crash semantics "miss over duplicate"),
	// send only if this instance won the claim.
	if !w.claimCursor(cursor, lastProcessed) {
		return
	}
	if len(msgs) > 0 {
		w.sendAll(msgs)
	}
}

// resolveDeviceTokens maps recipient mb_ids to their Expo push tokens.
func (w *PushNotifyWorker) resolveDeviceTokens(mbSet map[string]struct{}) map[string][]string {
	out := make(map[string][]string)
	if len(mbSet) == 0 {
		return out
	}
	mbIDs := make([]string, 0, len(mbSet))
	for mb := range mbSet {
		mbIDs = append(mbIDs, mb)
	}

	var users []v2domain.V2User
	if err := w.db.Select("id", "username").Where("username IN ?", mbIDs).Find(&users).Error; err != nil {
		log.Printf("[PushNotifyWorker] user lookup failed: %v", err)
		return out
	}
	if len(users) == 0 {
		return out
	}
	idToMb := make(map[uint64]string, len(users))
	userIDs := make([]uint64, 0, len(users))
	for _, u := range users {
		idToMb[u.ID] = u.Username
		userIDs = append(userIDs, u.ID)
	}

	var devices []v2domain.V2Device
	if err := w.db.Where("user_id IN ?", userIDs).Find(&devices).Error; err != nil {
		log.Printf("[PushNotifyWorker] device lookup failed: %v", err)
		return out
	}
	for _, d := range devices {
		// Only Expo tokens are routable via exp.host; skip anything else.
		if !strings.HasPrefix(d.Token, "ExponentPushToken") && !strings.HasPrefix(d.Token, "ExpoPushToken") {
			continue
		}
		mb := idToMb[d.UserID]
		out[mb] = append(out[mb], d.Token)
	}
	return out
}

func (w *PushNotifyWorker) buildMessage(n gnurepo.Notification, token string) expoPushMessage {
	body := strings.TrimSpace(n.RelMsg)
	if body == "" {
		body = strings.TrimSpace(n.ParentSubject)
	}
	if r := []rune(body); len(r) > 120 {
		body = string(r[:120]) + "…"
	}
	return expoPushMessage{
		To: token,
		// 제목은 handler.NotificationTitle 단일 소스 사용 — 복제 금지(bug#13242).
		// comment 계열의 80%는 "내 글에 댓글"이라 toCase/wrParent 구분이 필수다.
		Title:     handler.NotificationTitle(n.PhFromCase, n.PhToCase, n.RelMbNick, n.WrID, n.WrParent),
		Body:      body,
		Data:      map[string]string{"url": pushAppURL(n)},
		Sound:     "default",
		ChannelID: "default",
		Priority:  "high",
	}
}

// pushAppURL builds the app deep-link path consumed by the app's notification
// tap handler (router.push(data.url)).
func pushAppURL(n gnurepo.Notification) string {
	if n.PhFromCase == "memo" {
		return "/messages"
	}
	if n.BoTable != "" {
		postID := n.WrID
		if n.WrParent > 0 {
			postID = n.WrParent
		}
		return fmt.Sprintf("/post/%s/%d", n.BoTable, postID)
	}
	return ""
}

// sendAll pushes messages to Expo in chunks of 100 and cleans up dead tokens.
// Ticket order matches message order within a chunk (Expo semantics), so the
// dead token is read straight from msgs — no parallel slice to misalign.
func (w *PushNotifyWorker) sendAll(msgs []expoPushMessage) {
	const chunkSize = 100
	sent, failed := 0, 0
	for start := 0; start < len(msgs); start += chunkSize {
		end := start + chunkSize
		if end > len(msgs) {
			end = len(msgs)
		}
		tickets, err := w.sendChunk(msgs[start:end])
		if err != nil {
			failed += end - start
			log.Printf("[PushNotifyWorker] chunk send failed (%d msgs): %v", end-start, err)
			continue
		}
		for i, t := range tickets {
			if t.Status == "ok" {
				sent++
				continue
			}
			failed++
			if t.Details.Error == "DeviceNotRegistered" && start+i < len(msgs) {
				w.deleteDeadToken(msgs[start+i].To)
			}
		}
	}
	log.Printf("[PushNotifyWorker] cycle done: sent=%d failed=%d", sent, failed)
}

func (w *PushNotifyWorker) sendChunk(msgs []expoPushMessage) ([]expoPushTicket, error) {
	payload, err := json.Marshal(msgs)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}
		req, err := http.NewRequest(http.MethodPost, expoPushURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if w.accessToken != "" {
			req.Header.Set("Authorization", "Bearer "+w.accessToken)
		}

		resp, err := w.httpClient.Do(req)
		if err != nil {
			// 전송 계층 오류는 재시도하지 않는다 — Expo 가 이미 배치를 수신했을 수
			// 있어(응답 유실) 재전송하면 중복 푸시가 된다. 미발송(인앱엔 남음)을 택함.
			return nil, err
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("expo push http %d: %s", resp.StatusCode, truncateForLog(body))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("expo push http %d: %s", resp.StatusCode, truncateForLog(body))
		}
		var parsed struct {
			Data []expoPushTicket `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("expo push response parse: %w", err)
		}
		return parsed.Data, nil
	}
	return nil, lastErr
}

func (w *PushNotifyWorker) deleteDeadToken(token string) {
	if err := w.db.Where("token = ?", token).Delete(&v2domain.V2Device{}).Error; err != nil {
		log.Printf("[PushNotifyWorker] dead token delete failed: %v", err)
		return
	}
	log.Printf("[PushNotifyWorker] removed DeviceNotRegistered token")
}

func truncateForLog(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
