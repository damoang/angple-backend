package worker

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/redis/go-redis/v9"
)

const (
	// keywordAlertToCase 는 키워드 알림의 ph_to_case 값이다(ph_from_case 는 'write').
	keywordAlertToCase = "keyword"
	// keywordAlertFromCase 는 새 글 알림(팔로우·구독·키워드)이 공유하는 ph_from_case 값이다.
	keywordAlertFromCase = "write"
	// keywordAlertHourlyLimit 는 회원 한 명이 한 시간(UTC 시각 단위)에 받는 키워드 알림 상한이다.
	keywordAlertHourlyLimit = 20
	// keywordAlertRateKeyTTL 은 시간당 카운터 키의 만료 시간이다(한 시간 + 여유).
	keywordAlertRateKeyTTL = 3700 * time.Second
	// keywordAlertMaxRecipients 는 글 하나가 보내는 키워드 알림 수신자 상한이다.
	keywordAlertMaxRecipients = 1000
	// keywordAlertRateTimeout 은 시간당 카운터 한 번 올리는 데 쓰는 시간 상한이다.
	keywordAlertRateTimeout = 500 * time.Millisecond
)

// keywordAlertBoardRe 는 동적 테이블명(g5_write_{board})에 넣어도 안전한 게시판 ID 다.
var keywordAlertBoardRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// KeywordAlertLimiter 는 회원별 시간당 키워드 알림 상한을 판정한다.
type KeywordAlertLimiter interface {
	// Allow 는 이번 알림을 보내도 되는지 돌려준다. 호출하면 카운터가 1 오른다.
	Allow(ctx context.Context, mbID string, now time.Time) bool
}

// RedisKeywordAlertLimiter 는 Redis INCR 카운터로 시간당 상한을 센다.
//
// 키: kwalert:{mb_id}:{UTC YYYYMMDDHH}, 만료 3700초.
// Redis 오류(장애·시간 초과)면 **보낸다** — 상한은 폭주 방지용 보조 장치이고,
// Redis 가 잠깐 죽었다고 키워드 알림 전체가 멈추면 안 된다.
type RedisKeywordAlertLimiter struct {
	limit int64
	incr  func(ctx context.Context, key string) (int64, error)
}

// NewRedisKeywordAlertLimiter 는 Redis 기반 시간당 상한을 만든다. client 가 nil 이면 nil 을 돌려준다
// (상한 없이 보낸다 — Redis 오류 시와 같은 동작).
func NewRedisKeywordAlertLimiter(client *redis.Client) *RedisKeywordAlertLimiter {
	if client == nil {
		return nil
	}
	incr := func(ctx context.Context, key string) (int64, error) {
		pipe := client.TxPipeline()
		cnt := pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, keywordAlertRateKeyTTL)
		if _, err := pipe.Exec(ctx); err != nil {
			return 0, err
		}
		return cnt.Val(), nil
	}
	return &RedisKeywordAlertLimiter{limit: keywordAlertHourlyLimit, incr: incr}
}

// keywordAlertRateKey 는 회원·UTC 시각(시 단위)별 카운터 키다.
func keywordAlertRateKey(mbID string, now time.Time) string {
	return "kwalert:" + mbID + ":" + now.UTC().Format("2006010215")
}

// Allow 는 카운터를 올리고 상한 이하이면 true 를 돌려준다. Redis 오류 시 true.
func (l *RedisKeywordAlertLimiter) Allow(ctx context.Context, mbID string, now time.Time) bool {
	if l == nil || l.incr == nil {
		return true
	}
	cctx, cancel := context.WithTimeout(ctx, keywordAlertRateTimeout)
	defer cancel()
	n, err := l.incr(cctx, keywordAlertRateKey(mbID, now))
	if err != nil {
		log.Printf("[KeywordAlert] rate counter failed for %s, sending anyway: %v", mbID, err)
		return true
	}
	return n <= l.limit
}

// SetKeywordAlert 는 키워드 알림 단계를 켠다. matcher 가 nil 이면 키워드 알림을 보내지 않는다.
// limiter 가 nil 이면 시간당 상한 없이 보낸다.
func (w *WriteAfterWorker) SetKeywordAlert(matcher *KeywordAlertMatcher, limiter KeywordAlertLimiter) {
	w.keywordMatcher = matcher
	// typed nil(*RedisKeywordAlertLimiter)(nil) 이 인터페이스에 담겨도 Allow 가 true 를 돌려주므로 안전하다.
	w.keywordLimiter = limiter
	w.keywordMaxRecipients = keywordAlertMaxRecipients
}

// isSecretPostOption 은 wr_option 에 secret 이 들어 있는지 본다(예: "html1,secret").
func isSecretPostOption(wrOption string) bool {
	for _, opt := range strings.Split(wrOption, ",") {
		if strings.TrimSpace(opt) == "secret" {
			return true
		}
	}
	return false
}

// isSecretPost 는 글이 비밀글인지 DB 에서 확인한다.
// 조회에 실패하면 비밀글로 본다 — 타인이 못 읽는 글의 제목이 알림으로 새는 것보다
// 키워드 알림 한 건을 놓치는 쪽이 안전하다.
func (w *WriteAfterWorker) isSecretPost(boTable string, wrID int) bool {
	if !keywordAlertBoardRe.MatchString(boTable) {
		return true
	}
	var rows []string
	if err := w.db.Table("g5_write_"+boTable).
		Where("wr_id = ?", wrID).
		Limit(1).
		Pluck("wr_option", &rows).Error; err != nil {
		log.Printf("[KeywordAlert] wr_option lookup failed for %s/%d: %v", boTable, wrID, err)
		return true
	}
	if len(rows) == 0 {
		return true
	}
	return isSecretPostOption(rows[0])
}

// sendKeywordAlerts 는 새 원글 제목에 회원 키워드가 들어가면 그 회원에게 알림을 보낸다.
//
// handlePostCreated 의 팔로우·구독 알림 뒤에 실행된다. 비공개 보드는 호출 전에 이미 걸러진다.
// 제외: 작성자 본인, 작성자를 차단한 회원(blockedBy), 팔로워(followerSet),
// 이 글로 이미 구독 알림을 받은 회원(subscribed), 비밀글, 키워드 알림 설정 OFF.
// 한 글 = 한 회원 최대 1건 — 같은 글·같은 작성자의 'write' 알림이 이미 있으면 건너뛴다
// (이벤트 재시도 시에도 중복 발행되지 않는다).
// 이 단계의 실패는 로그만 남기고 이벤트 처리 결과에는 영향을 주지 않는다.
func (w *WriteAfterWorker) sendKeywordAlerts(job PostCreatedJob, authorName string, blockedBy, followerSet, subscribed map[string]bool) {
	if w.keywordMatcher == nil {
		return
	}
	ctx := context.Background()
	matches := w.keywordMatcher.Match(ctx, job.Subject)
	if len(matches) == 0 {
		return
	}
	if w.isSecretPost(job.BoardSlug, job.WriteID) {
		return
	}

	maxRecipients := w.keywordMaxRecipients
	if maxRecipients <= 0 {
		maxRecipients = keywordAlertMaxRecipients
	}
	now := time.Now()
	sent := 0
	for i, m := range matches {
		if m.MbID == job.MemberID || blockedBy[m.MbID] || followerSet[m.MbID] || subscribed[m.MbID] {
			continue
		}
		if sent >= maxRecipients {
			log.Printf("[KeywordAlert] recipient cap %d reached for %s/%d, dropped up to %d candidates",
				maxRecipients, job.BoardSlug, job.WriteID, len(matches)-i)
			return
		}
		if w.sendKeywordAlert(ctx, job, authorName, m, now) {
			sent++
		}
	}
}

// sendKeywordAlert 는 한 회원에게 키워드 알림 1건을 보낸다. 실제로 만들었으면 true.
func (w *WriteAfterWorker) sendKeywordAlert(ctx context.Context, job PostCreatedJob, authorName string, m KeywordAlertMatch, now time.Time) bool {
	pref, ok := w.mustGetNotiPreference(m.MbID)
	if !ok || !pref.NotiKeyword {
		return false
	}
	// 같은 글로 이미 'write' 알림(팔로우·구독·이전 시도의 키워드)을 받았으면 보내지 않는다.
	// 시간당 카운터를 올리기 전에 확인해, 재시도가 상한을 갉아먹지 않게 한다.
	if exists, err := w.notiRepo.Exists(m.MbID, job.BoardSlug, job.WriteID, keywordAlertFromCase, job.MemberID); err != nil || exists {
		return false
	}
	if w.keywordLimiter != nil && !w.keywordLimiter.Allow(ctx, m.MbID, now) {
		return false
	}
	noti := &gnurepo.Notification{
		PhToCase: keywordAlertToCase, PhFromCase: keywordAlertFromCase, BoTable: job.BoardSlug,
		WrID: job.WriteID, MbID: m.MbID, RelMbID: job.MemberID,
		RelMbNick:  authorName,
		RelMsg:     fmt.Sprintf("키워드 '%s' 새 글: %s", m.Keyword, job.Subject),
		RelURL:     fmt.Sprintf("/%s/%d", job.BoardSlug, job.WriteID),
		PhReaded:   "N",
		PhDatetime: job.CreatedAt,
		WrParent:   job.WriteID,
	}
	if err := w.notiRepo.Create(noti); err != nil {
		log.Printf("[KeywordAlert] notification create failed for %s/%d: %v", job.BoardSlug, job.WriteID, err)
		return false
	}
	return true
}
