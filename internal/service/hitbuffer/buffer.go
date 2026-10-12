// Package hitbuffer 는 게시글 조회수(wr_hit) 증가를 Redis 에 모아 두었다가
// 주기적으로 한 번에 DB 에 반영한다.
//
// 조회 1회마다 UPDATE 1회를 보내던 방식을, 파드가 공유하는 Redis 해시
// (hits:{bo_table}, 필드=wr_id)에 HINCRBY 로 쌓고 일정 주기마다 한 파드가
// 글별로 `wr_hit = wr_hit + N` 묶음 UPDATE 를 보내는 방식으로 바꿔 DB 쓰기 부하를 줄인다.
//
// 합계 보존 규칙:
//   - Redis 쓰기가 실패하면 즉시 DB UPDATE 로 폴백한다(조회수 누락 방지).
//   - flush 는 살아있는 해시를 처리 중 키로 원자적으로 RENAME 한 뒤 읽는다.
//     RENAME 이후 들어온 조회는 새 해시에 쌓이므로 섞이지 않는다.
//   - DB 반영이 실패하면 처리 중 키에 남은 값을 살아있는 해시로 원자적으로 되돌린다.
//   - 처리 도중 파드가 죽어 남은 처리 중 키는 staleAfter 가 지난 뒤 다음 flush 가
//     원자적으로 다시 가져가 반영한다(두 파드가 동시에 가져가지 못한다).
//
// 전달 보장은 「최소 한 번」이다. 아래 좁은 창에서는 같은 몫이 두 번 반영될 수 있다:
//   - 묶음 UPDATE 가 커밋된 뒤 그 필드를 지우는 HDEL(Ack)이 실패한 경우 — 필드가 처리 중 키에
//     남아 staleAfter 뒤 회수되며 다시 더해진다(그 묶음만).
//   - DB 가 커밋한 뒤 오류를 돌려준 경우(응답 유실·타임아웃) — 되돌린 값이 다음 flush 에 다시 더해진다.
//   - Incr 의 Redis 호출이 시간 초과로 실패했지만 서버에서는 HINCRBY 가 실행된 경우 — 폴백
//     즉시 UPDATE 와 버퍼 값이 둘 다 반영되어 그 조회 1건이 두 번 세어진다.
//
// Redis 쪽에서 아직 반영 안 된 값이 사라질 수 있는 경우:
//   - maxmemory 정책이 allkeys-* 이면 이 키들도 내쫓길 수 있다(volatile-* 은 TTL 없는 키를 내쫓지 않는다).
//   - Redis 가 재시작하면 마지막 RDB/AOF 저장 이후 쌓인 값은 사라진다(최대 한 주기분 + 저장 간격).
//
// 화면에 보이는 조회수의 집계 방식(중복 제거 여부)은 바꾸지 않는다 — 반영 시점만 늦어진다.
package hitbuffer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultInterval 는 flush 주기다. 조회수는 최대 이 시간만큼 늦게 DB 에 반영된다.
	DefaultInterval = 3 * time.Minute

	// DefaultChunkSize 는 UPDATE 한 문장에 묶는 글 수의 상한이다.
	DefaultChunkSize = 500

	// DefaultStaleAfter 는 처리 중 키를 「주인이 죽었다」고 보고 회수하기까지의 대기 시간이다.
	// flush 제한 시간(flushTimeout)보다 충분히 길어야 살아있는 flush 의 키를 가로채지 않는다.
	DefaultStaleAfter = 10 * time.Minute

	// StopTimeout 은 Stop 이 기다리는 총 상한이다(진행 중 flush 중단 + 종료 flush).
	// 취소 뒤에도 끝까지 시도하는 정리 호출(bookkeepingTimeout, 많아야 두어 번)만큼 넘칠 수 있다.
	StopTimeout = 3 * time.Second

	incrTimeout  = time.Second
	flushTimeout = time.Minute

	// DB 반영 뒤의 정리 호출(Ack·Done·Restore) 상한. flush 가 취소돼도 이 호출들은 끝까지 시도한다.
	bookkeepingTimeout = time.Second

	// 리더 락 TTL 은 주기보다 조금 짧게 둔다 — 락을 쥔 파드가 다음 틱에 다시 잡을 수 있게.
	lockMargin = 10 * time.Second

	// 테이블이 없는 게시판은 이 시간 동안 flush 를 건너뛰고(값은 버퍼에 남는다) 로그도 이 간격으로만 남긴다.
	missingTableBackoff = time.Hour

	liveKeyPrefix   = "hits:"
	flushingSegment = "flushing"
)

// 테이블명(g5_write_{bo_table})에 끼워 넣을 게시판 ID 검증.
// Redis 에서 읽어 온 값도 그대로 SQL 에 넣지 않는다.
var boardIDRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

var (
	// ErrInvalidBoardID 는 게시판 ID 가 테이블명으로 쓸 수 없는 값일 때 반환된다.
	ErrInvalidBoardID = errors.New("hitbuffer: invalid board id")

	// ErrInvalidPostID 는 글 ID 가 0 이하일 때 반환된다.
	ErrInvalidPostID = errors.New("hitbuffer: invalid post id")

	// ErrTableMissing 은 Store 가 게시판 테이블이 없다고 판단했을 때 반환한다.
	ErrTableMissing = errors.New("hitbuffer: board table missing")
)

// ValidBoardID 는 게시판 ID 가 동적 테이블명에 넣어도 안전한 값인지 확인한다.
func ValidBoardID(boardID string) bool {
	return boardIDRe.MatchString(boardID)
}

// Store 는 조회수를 실제로 DB 에 반영하는 쪽이다.
type Store interface {
	// IncrementHit 는 글 하나의 조회수를 즉시 1 올린다(Redis 장애 시 폴백).
	IncrementHit(boardID string, wrID int) error
	// AddHits 는 한 게시판의 여러 글 조회수를 글별 증가분만큼 한 번에 올린다.
	AddHits(ctx context.Context, boardID string, hits map[int]int64) error
}

// Backend 는 조회수 버퍼를 담는 공유 저장소(Redis)다. 각 메서드는 원자적이어야 한다.
type Backend interface {
	// Incr 는 hits:{boardID} 해시의 wrID 필드를 1 올리고 게시판을 목록에 등록한다.
	Incr(ctx context.Context, boardID string, wrID int) error
	// AcquireLock 은 flush 리더 락을 ttl 동안 잡는다. 이미 누가 잡고 있으면 false.
	AcquireLock(ctx context.Context, token string, ttl time.Duration) (bool, error)
	// Tables 는 버퍼에 값이 있을 수 있는 게시판 ID 목록을 돌려준다.
	Tables(ctx context.Context) ([]string, error)
	// RemoveTable 은 게시판 ID 를 목록에서 뺀다.
	RemoveTable(ctx context.Context, boardID string) error
	// Pending 은 아직 정리되지 않은 처리 중 키 목록을 돌려준다.
	Pending(ctx context.Context) ([]string, error)
	// Claim 은 src 키를 처리 중 키 dst 로 원자적으로 옮기고 dst 를 처리 중 목록에 올린다.
	// src 가 없으면 false 를 돌려주고 src 를 목록에서 정리한다
	// (srcPending 이면 처리 중 목록에서, 아니면 게시판 목록에서 boardID 를).
	Claim(ctx context.Context, boardID, src, dst string, srcPending bool) (bool, error)
	// Read 는 처리 중 키의 필드(wr_id)와 값(증가분)을 모두 읽는다.
	Read(ctx context.Context, key string) (map[string]string, error)
	// Ack 는 DB 반영이 끝난 필드를 처리 중 키에서 지운다.
	Ack(ctx context.Context, key string, fields []string) error
	// Restore 는 처리 중 키에 남은 값을 hits:{boardID} 로 더해 되돌리고 처리 중 키를 지운다.
	Restore(ctx context.Context, boardID, key string) error
	// Done 은 처리가 끝난 처리 중 키를 지우고 목록에서 뺀다.
	Done(ctx context.Context, key string) error
}

// Buffer 는 조회수 증가를 Backend 에 모으고 주기적으로 Store 에 반영한다.
type Buffer struct {
	backend Backend
	store   Store

	interval   time.Duration
	chunkSize  int
	staleAfter time.Duration
	now        func() time.Time

	token string
	seq   atomic.Uint64

	loopCtx    context.Context
	loopCancel context.CancelFunc
	doneCh     chan struct{}
	started    atomic.Bool
	stopOnce   sync.Once

	missingMu sync.Mutex
	missing   map[string]time.Time
}

// Option 은 Buffer 설정을 바꾼다.
type Option func(*Buffer)

// WithInterval 은 flush 주기를 바꾼다(리더 락 TTL 도 이 값에서 정한다).
func WithInterval(d time.Duration) Option {
	return func(b *Buffer) {
		if d > 0 {
			b.interval = d
		}
	}
}

// WithChunkSize 는 UPDATE 한 문장에 묶는 글 수 상한을 바꾼다.
func WithChunkSize(n int) Option {
	return func(b *Buffer) {
		if n > 0 {
			b.chunkSize = n
		}
	}
}

// WithStaleAfter 는 남은 처리 중 키를 회수하기까지의 대기 시간을 바꾼다.
func WithStaleAfter(d time.Duration) Option {
	return func(b *Buffer) {
		if d >= 0 {
			b.staleAfter = d
		}
	}
}

// WithClock 은 현재 시각 함수를 바꾼다(테스트용).
func WithClock(now func() time.Time) Option {
	return func(b *Buffer) {
		if now != nil {
			b.now = now
		}
	}
}

// New 는 Buffer 를 만든다. backend 가 nil 이면 모든 조회를 즉시 DB 에 반영한다(기존 동작).
func New(backend Backend, store Store, opts ...Option) *Buffer {
	loopCtx, loopCancel := context.WithCancel(context.Background())
	b := &Buffer{
		backend:    backend,
		store:      store,
		interval:   DefaultInterval,
		chunkSize:  DefaultChunkSize,
		staleAfter: DefaultStaleAfter,
		now:        time.Now,
		token:      uuid.NewString(),
		loopCtx:    loopCtx,
		loopCancel: loopCancel,
		doneCh:     make(chan struct{}),
		missing:    map[string]time.Time{},
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Incr 는 글 하나의 조회수를 1 올린다. Redis 에 쌓지 못하면 즉시 DB 에 반영한다.
func (b *Buffer) Incr(boardID string, wrID int) error {
	if !ValidBoardID(boardID) {
		return ErrInvalidBoardID
	}
	if wrID <= 0 {
		return ErrInvalidPostID
	}
	if b.backend != nil {
		ctx, cancel := context.WithTimeout(context.Background(), incrTimeout)
		err := b.backend.Incr(ctx, boardID, wrID)
		cancel()
		if err == nil {
			return nil
		}
		log.Printf("[hitbuffer] buffer write failed, direct update fallback: %v", err)
	}
	return b.store.IncrementHit(boardID, wrID)
}

// Start 는 주기적 flush 루프를 띄운다. backend 가 없으면 아무것도 하지 않는다.
func (b *Buffer) Start() {
	if b.backend == nil || !b.started.CompareAndSwap(false, true) {
		return
	}
	go b.loop()
}

func (b *Buffer) loop() {
	defer close(b.doneCh)
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	for {
		select {
		case <-b.loopCtx.Done():
			return
		case <-ticker.C:
			// Stop 이 loopCtx 를 취소하면 진행 중인 Redis·DB 호출이 바로 끝난다.
			ctx, cancel := context.WithTimeout(b.loopCtx, flushTimeout)
			if _, err := b.FlushIfLeader(ctx); err != nil {
				log.Printf("[hitbuffer] flush error: %v", err)
			}
			cancel()
		}
	}
}

// Stop 은 flush 루프를 멈추고 마지막으로 한 번 flush 한다(종료 시 호출).
// 진행 중인 flush 는 취소되고(못 끝낸 묶음은 버퍼로 되돌아간다), 전체가 StopTimeout 안에 끝난다.
// 종료 flush 는 리더 락 없이 돈다 — RENAME 으로 가져간 몫만 처리하므로
// 다른 파드의 flush 와 겹쳐도 같은 값을 두 번 반영하지 않는다.
func (b *Buffer) Stop() {
	b.stopOnce.Do(b.stop)
}

func (b *Buffer) stop() {
	ctx, cancel := context.WithTimeout(context.Background(), StopTimeout)
	defer cancel()
	b.loopCancel()
	if b.started.Load() {
		select {
		case <-b.doneCh:
		case <-ctx.Done():
			// 루프가 아직 Redis 를 쓰고 있을 수 있으므로 백엔드를 닫지 않는다.
			log.Printf("[hitbuffer] stop timed out waiting for in-flight flush")
			return
		}
	}
	if b.backend == nil {
		return
	}
	if err := b.Flush(ctx); err != nil {
		log.Printf("[hitbuffer] shutdown flush error: %v", err)
	}
	if c, ok := b.backend.(io.Closer); ok {
		if err := c.Close(); err != nil {
			log.Printf("[hitbuffer] backend close error: %v", err)
		}
	}
}

// lockTTL 은 리더 락 TTL 이다. 주기보다 lockMargin 만큼 짧아 락을 쥔 파드가 다음 틱에 다시 잡는다.
func (b *Buffer) lockTTL() time.Duration {
	if b.interval > 2*lockMargin {
		return b.interval - lockMargin
	}
	return b.interval
}

// FlushIfLeader 는 리더 락을 잡은 경우에만 flush 한다. 락은 주기만큼 유지되어
// 여러 파드가 있어도 전체 flush 가 주기당 한 번 정도로 제한된다.
func (b *Buffer) FlushIfLeader(ctx context.Context) (bool, error) {
	if b.backend == nil {
		return false, nil
	}
	ok, err := b.backend.AcquireLock(ctx, b.token, b.lockTTL())
	if err != nil {
		return false, fmt.Errorf("hitbuffer: acquire lock: %w", err)
	}
	if !ok {
		return false, nil
	}
	return true, b.Flush(ctx)
}

// Flush 는 남은 처리 중 키를 회수한 뒤 모든 게시판의 버퍼를 DB 에 반영한다.
func (b *Buffer) Flush(ctx context.Context) error {
	if b.backend == nil {
		return nil
	}
	errs := make([]error, 0)
	if err := b.recoverStale(ctx); err != nil {
		errs = append(errs, err)
	}
	tables, err := b.backend.Tables(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("hitbuffer: list tables: %w", err))
		return errors.Join(errs...)
	}
	sort.Strings(tables)
	for _, t := range tables {
		if err := b.flushTable(ctx, t); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (b *Buffer) flushTable(ctx context.Context, boardID string) error {
	if !ValidBoardID(boardID) {
		log.Printf("[hitbuffer] invalid board id in buffer, dropped: %q", boardID)
		return b.backend.RemoveTable(ctx, boardID)
	}
	if b.recentlyMissing(boardID) {
		// 값은 hits:{boardID} 에 그대로 남아 나중에 반영될 수 있다.
		return nil
	}
	dst := b.newFlushingKey(boardID)
	ok, err := b.backend.Claim(ctx, boardID, LiveKey(boardID), dst, false)
	if err != nil {
		return fmt.Errorf("hitbuffer: claim %s: %w", boardID, err)
	}
	if !ok {
		return nil
	}
	return b.drain(ctx, boardID, dst)
}

// recoverStale 은 주인이 죽어 남은 처리 중 키를 다시 가져가 반영한다.
func (b *Buffer) recoverStale(ctx context.Context) error {
	keys, err := b.backend.Pending(ctx)
	if err != nil {
		return fmt.Errorf("hitbuffer: list pending: %w", err)
	}
	sort.Strings(keys)
	errs := make([]error, 0)
	for _, k := range keys {
		boardID, ts, ok := parseFlushingKey(k)
		if !ok || !ValidBoardID(boardID) {
			log.Printf("[hitbuffer] unrecognized pending key skipped: %q", k)
			continue
		}
		if b.now().Sub(ts) < b.staleAfter {
			continue
		}
		dst := b.newFlushingKey(boardID)
		claimed, err := b.backend.Claim(ctx, boardID, k, dst, true)
		if err != nil {
			errs = append(errs, fmt.Errorf("hitbuffer: reclaim %s: %w", k, err))
			continue
		}
		if !claimed {
			continue
		}
		if err := b.drain(ctx, boardID, dst); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type hitEntry struct {
	field string
	wrID  int
	count int64
}

// drain 은 처리 중 키를 읽어 묶음 단위로 DB 에 반영한다.
// 묶음이 반영될 때마다 그 필드를 지우므로, 중간에 실패해도 반영 안 된 몫만 되돌린다.
func (b *Buffer) drain(ctx context.Context, boardID, key string) error {
	raw, err := b.backend.Read(ctx, key)
	if err != nil {
		// 키는 처리 중 목록에 남아 있으므로 staleAfter 뒤 회수된다.
		return fmt.Errorf("hitbuffer: read %s: %w", key, err)
	}
	entries, bad := parseHits(raw)
	if len(bad) > 0 {
		log.Printf("[hitbuffer] %d malformed fields dropped from %s", len(bad), key)
		if err := b.ack(ctx, key, bad); err != nil {
			return fmt.Errorf("hitbuffer: drop malformed %s: %w", key, err)
		}
	}
	for start := 0; start < len(entries); start += b.chunkSize {
		end := min(start+b.chunkSize, len(entries))
		chunk := entries[start:end]
		hits := make(map[int]int64, len(chunk))
		fields := make([]string, 0, len(chunk))
		for _, e := range chunk {
			hits[e.wrID] = e.count
			fields = append(fields, e.field)
		}
		if err := b.store.AddHits(ctx, boardID, hits); err != nil {
			return b.restoreAfterFailure(ctx, boardID, key, err)
		}
		if err := b.ack(ctx, key, fields); err != nil {
			return fmt.Errorf("hitbuffer: ack %s: %w", key, err)
		}
	}
	dctx, cancel := bookkeepingContext(ctx)
	defer cancel()
	if err := b.backend.Done(dctx, key); err != nil {
		return fmt.Errorf("hitbuffer: done %s: %w", key, err)
	}
	return nil
}

// restoreAfterFailure 는 DB 반영 실패 시 남은 값을 버퍼로 되돌린다.
// flush 가 취소된 경우(종료)에도 되돌릴 수 있게 취소와 분리된 짧은 컨텍스트를 쓴다.
// 되돌리기마저 실패하면 키는 처리 중 목록에 남아 staleAfter 뒤 회수된다.
func (b *Buffer) restoreAfterFailure(ctx context.Context, boardID, key string, cause error) error {
	rctx, cancel := bookkeepingContext(ctx)
	defer cancel()
	if rErr := b.backend.Restore(rctx, boardID, key); rErr != nil {
		return fmt.Errorf("hitbuffer: update %s: %w (restore failed: %w)", boardID, cause, rErr)
	}
	if errors.Is(cause, ErrTableMissing) {
		// 같은 게시판으로 매 주기 로그가 쌓이지 않도록 한 시간 동안 건너뛴다(그동안 로그도 없음).
		b.markMissing(boardID)
		log.Printf("[hitbuffer] table for board %q missing; counts kept in buffer, retry in %s", boardID, missingTableBackoff)
		return nil
	}
	return fmt.Errorf("hitbuffer: update %s (restored to buffer): %w", boardID, cause)
}

// ack 는 반영된 필드를 지운다. DB 에 이미 반영된 뒤라 flush 가 취소돼도 끝까지 시도한다
// (여기서 빠지면 그 묶음이 회수 때 한 번 더 더해진다).
func (b *Buffer) ack(ctx context.Context, key string, fields []string) error {
	actx, cancel := bookkeepingContext(ctx)
	defer cancel()
	return b.backend.Ack(actx, key, fields)
}

// bookkeepingContext 는 부모의 취소와 분리되고 bookkeepingTimeout 으로 제한된 컨텍스트다.
func bookkeepingContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), bookkeepingTimeout)
}

func (b *Buffer) markMissing(boardID string) {
	b.missingMu.Lock()
	defer b.missingMu.Unlock()
	b.missing[boardID] = b.now()
}

func (b *Buffer) recentlyMissing(boardID string) bool {
	b.missingMu.Lock()
	defer b.missingMu.Unlock()
	at, ok := b.missing[boardID]
	if !ok {
		return false
	}
	if b.now().Sub(at) >= missingTableBackoff {
		delete(b.missing, boardID)
		return false
	}
	return true
}

// parseHits 는 해시 필드/값을 글 ID/증가분으로 바꾼다. 해석할 수 없는 필드는 bad 로 돌려준다.
func parseHits(raw map[string]string) ([]hitEntry, []string) {
	entries := make([]hitEntry, 0, len(raw))
	bad := make([]string, 0)
	for field, value := range raw {
		wrID, err := strconv.Atoi(field)
		if err != nil || wrID <= 0 {
			bad = append(bad, field)
			continue
		}
		count, err := strconv.ParseInt(value, 10, 64)
		if err != nil || count <= 0 {
			bad = append(bad, field)
			continue
		}
		entries = append(entries, hitEntry{field: field, wrID: wrID, count: count})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].wrID < entries[j].wrID })
	return entries, bad
}

// LiveKey 는 게시판의 조회수 버퍼 해시 키(hits:{bo_table})다.
func LiveKey(boardID string) string {
	return liveKeyPrefix + boardID
}

// newFlushingKey 는 hits:{bo_table}:flushing:{unixnano}:{token}-{seq} 형식의 처리 중 키를 만든다.
func (b *Buffer) newFlushingKey(boardID string) string {
	return fmt.Sprintf("%s%s:%s:%d:%s-%d",
		liveKeyPrefix, boardID, flushingSegment, b.now().UnixNano(), b.token, b.seq.Add(1))
}

// parseFlushingKey 는 처리 중 키에서 게시판 ID 와 만든 시각을 꺼낸다.
func parseFlushingKey(key string) (string, time.Time, bool) {
	rest, ok := strings.CutPrefix(key, liveKeyPrefix)
	if !ok {
		return "", time.Time{}, false
	}
	parts := strings.Split(rest, ":")
	if len(parts) != 4 || parts[1] != flushingSegment {
		return "", time.Time{}, false
	}
	nanos, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", time.Time{}, false
	}
	return parts[0], time.Unix(0, nanos), true
}
