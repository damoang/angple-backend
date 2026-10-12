package hitbuffer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testBoard = "free"

var errBoom = errors.New("boom")

// fakeBackend 는 RedisBackend 의 의미(원자적 RENAME·되돌리기)를 메모리로 흉내 낸다.
type fakeBackend struct {
	mu      sync.Mutex
	hashes  map[string]map[string]int64
	tables  map[string]bool
	pending map[string]bool
	lock    string
	lockTTL time.Duration

	incrErr error
	ackErr  error
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		hashes:  map[string]map[string]int64{},
		tables:  map[string]bool{},
		pending: map[string]bool{},
	}
}

func (f *fakeBackend) Incr(_ context.Context, boardID string, wrID int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.incrErr != nil {
		return f.incrErr
	}
	f.hincr(LiveKey(boardID), strconv.Itoa(wrID), 1)
	f.tables[boardID] = true
	return nil
}

func (f *fakeBackend) hincr(key, field string, n int64) {
	h, ok := f.hashes[key]
	if !ok {
		h = map[string]int64{}
		f.hashes[key] = h
	}
	h[field] += n
}

func (f *fakeBackend) AcquireLock(_ context.Context, token string, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lock != "" {
		return false, nil
	}
	f.lock = token
	f.lockTTL = ttl
	return true, nil
}

func (f *fakeBackend) expireLock() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lock = ""
}

func (f *fakeBackend) Tables(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.tables))
	for t := range f.tables {
		out = append(out, t)
	}
	return out, nil
}

func (f *fakeBackend) RemoveTable(_ context.Context, boardID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.tables, boardID)
	return nil
}

func (f *fakeBackend) Pending(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.pending))
	for k := range f.pending {
		out = append(out, k)
	}
	return out, nil
}

func (f *fakeBackend) Claim(_ context.Context, boardID, src, dst string, srcPending bool) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h, ok := f.hashes[src]
	if !ok || len(h) == 0 {
		if srcPending {
			delete(f.pending, src)
		} else {
			delete(f.tables, boardID)
		}
		return false, nil
	}
	delete(f.hashes, src)
	f.hashes[dst] = h
	f.pending[dst] = true
	if srcPending {
		delete(f.pending, src)
	}
	return true, nil
}

func (f *fakeBackend) Read(_ context.Context, key string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	for k, v := range f.hashes[key] {
		out[k] = strconv.FormatInt(v, 10)
	}
	return out, nil
}

func (f *fakeBackend) Ack(_ context.Context, key string, fields []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ackErr != nil {
		return f.ackErr
	}
	for _, field := range fields {
		delete(f.hashes[key], field)
	}
	return nil
}

func (f *fakeBackend) Restore(_ context.Context, boardID, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	h := f.hashes[key]
	for field, n := range h {
		f.hincr(LiveKey(boardID), field, n)
	}
	if len(h) > 0 {
		f.tables[boardID] = true
	}
	delete(f.hashes, key)
	delete(f.pending, key)
	return nil
}

func (f *fakeBackend) Done(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.hashes, key)
	delete(f.pending, key)
	return nil
}

// buffered 는 아직 DB 에 반영되지 않은 모든 키의 합계를 글별로 돌려준다.
func (f *fakeBackend) buffered() map[string]int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int64{}
	for _, h := range f.hashes {
		for field, n := range h {
			out[field] += n
		}
	}
	return out
}

// fakeStore 는 DB 대신 글별 조회수를 메모리에 쌓는다.
type fakeStore struct {
	mu        sync.Mutex
	hits      map[string]map[int]int64
	direct    int
	addCalls  int
	failCalls map[int]bool // n 번째(1부터) AddHits 호출을 실패시킨다
	missing   map[string]bool

	// entered 가 있으면 첫 AddHits 가 이를 닫고 ctx 가 끝날 때까지 막힌다(진행 중 flush 흉내).
	entered chan struct{}
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		hits:      map[string]map[int]int64{},
		failCalls: map[int]bool{},
		missing:   map[string]bool{},
	}
}

func (s *fakeStore) IncrementHit(boardID string, wrID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.direct++
	s.add(boardID, wrID, 1)
	return nil
}

func (s *fakeStore) AddHits(ctx context.Context, boardID string, hits map[int]int64) error {
	s.mu.Lock()
	s.addCalls++
	call := s.addCalls
	entered := s.entered
	s.mu.Unlock()
	if entered != nil && call == 1 {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.missing[boardID] {
		return fmt.Errorf("%w: %s", ErrTableMissing, boardID)
	}
	if s.failCalls[call] {
		return errBoom
	}
	for id, n := range hits {
		s.add(boardID, id, n)
	}
	return nil
}

func (s *fakeStore) add(boardID string, wrID int, n int64) {
	h, ok := s.hits[boardID]
	if !ok {
		h = map[int]int64{}
		s.hits[boardID] = h
	}
	h[wrID] += n
}

func (s *fakeStore) get(boardID string, wrID int) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[boardID][wrID]
}

func TestIncrAndFlushPreservesCounts(t *testing.T) {
	be := newFakeBackend()
	st := newFakeStore()
	b := New(be, st, WithChunkSize(2))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			assert.NoError(t, b.Incr(testBoard, i%5+1))
		}(i)
	}
	wg.Wait()
	require.NoError(t, b.Incr("qa", 7))

	// flush 전에는 DB 에 쓰지 않는다.
	assert.Equal(t, 0, st.addCalls)
	assert.Equal(t, int64(0), st.get(testBoard, 1))

	require.NoError(t, b.Flush(context.Background()))
	for id := 1; id <= 5; id++ {
		assert.Equal(t, int64(10), st.get(testBoard, id), "wr_id=%d", id)
	}
	assert.Equal(t, int64(1), st.get("qa", 7))
	assert.Equal(t, 0, st.direct)
	assert.Empty(t, be.buffered())
	assert.Empty(t, be.pending)

	// 두 번째 flush 는 같은 값을 다시 반영하지 않는다.
	require.NoError(t, b.Flush(context.Background()))
	assert.Equal(t, int64(10), st.get(testBoard, 1))
	assert.Empty(t, be.tables, "빈 게시판은 목록에서 정리된다")
}

func TestFlushDBFailureRestoresCounts(t *testing.T) {
	be := newFakeBackend()
	st := newFakeStore()
	st.failCalls[1] = true
	b := New(be, st)

	for i := 0; i < 3; i++ {
		require.NoError(t, b.Incr(testBoard, 1))
	}
	require.Error(t, b.Flush(context.Background()))
	assert.Equal(t, int64(0), st.get(testBoard, 1))
	assert.Equal(t, map[string]int64{"1": 3}, be.buffered(), "실패한 몫은 버퍼로 되돌아간다")
	assert.Empty(t, be.pending)
	assert.Equal(t, int64(3), be.hashes[LiveKey(testBoard)]["1"])

	// 되돌린 뒤 쌓인 조회와 합쳐 다음 flush 에 반영된다.
	require.NoError(t, b.Incr(testBoard, 1))
	require.NoError(t, b.Flush(context.Background()))
	assert.Equal(t, int64(4), st.get(testBoard, 1))
	assert.Empty(t, be.buffered())
}

func TestFlushPartialChunkFailureRestoresOnlyUnapplied(t *testing.T) {
	be := newFakeBackend()
	st := newFakeStore()
	st.failCalls[2] = true
	b := New(be, st, WithChunkSize(2))

	for id := 1; id <= 4; id++ {
		require.NoError(t, b.Incr(testBoard, id))
	}
	require.Error(t, b.Flush(context.Background()))
	// 첫 묶음(1,2)은 반영되고, 실패한 묶음(3,4)만 되돌아간다 — 이중 반영 없음.
	assert.Equal(t, int64(1), st.get(testBoard, 1))
	assert.Equal(t, int64(1), st.get(testBoard, 2))
	assert.Equal(t, map[string]int64{"3": 1, "4": 1}, be.buffered())

	require.NoError(t, b.Flush(context.Background()))
	for id := 1; id <= 4; id++ {
		assert.Equal(t, int64(1), st.get(testBoard, id), "wr_id=%d", id)
	}
}

func TestFlushRecoversStaleFlushingKey(t *testing.T) {
	be := newFakeBackend()
	st := newFakeStore()
	now := time.Unix(1_800_000_000, 0)
	clock := func() time.Time { return now }

	// 죽은 파드가 RENAME 까지만 하고 남긴 처리 중 키.
	crashed := New(be, st, WithClock(clock))
	staleKey := crashed.newFlushingKey(testBoard)
	be.hashes[staleKey] = map[string]int64{"9": 5}
	be.pending[staleKey] = true

	// 아직 오래되지 않은 처리 중 키(다른 파드가 처리 중일 수 있음)는 건드리지 않는다.
	now = now.Add(time.Minute)
	freshKey := crashed.newFlushingKey("qa")
	be.hashes[freshKey] = map[string]int64{"2": 1}
	be.pending[freshKey] = true

	b := New(be, st, WithClock(clock), WithStaleAfter(5*time.Minute))
	now = now.Add(4 * time.Minute)
	require.NoError(t, b.Flush(context.Background()))
	assert.Equal(t, int64(5), st.get(testBoard, 9))
	assert.Equal(t, int64(0), st.get("qa", 2))
	assert.NotContains(t, be.pending, staleKey)
	assert.Contains(t, be.pending, freshKey)

	now = now.Add(time.Minute)
	require.NoError(t, b.Flush(context.Background()))
	assert.Equal(t, int64(1), st.get("qa", 2))
	assert.Empty(t, be.pending)
	assert.Empty(t, be.buffered())
}

func TestParseFlushingKey(t *testing.T) {
	b := New(nil, newFakeStore(), WithClock(func() time.Time { return time.Unix(0, 42) }))
	board, ts, ok := parseFlushingKey(b.newFlushingKey(testBoard))
	require.True(t, ok)
	assert.Equal(t, testBoard, board)
	assert.Equal(t, int64(42), ts.UnixNano())

	for _, k := range []string{"hits:free", "x:free:flushing:1:t", "hits:free:flushing:nan:t", "hits:free:other:1:t"} {
		_, _, ok := parseFlushingKey(k)
		assert.False(t, ok, k)
	}
}

func TestFlushIfLeaderPreventsDoubleFlush(t *testing.T) {
	be := newFakeBackend()
	st := newFakeStore()
	podA := New(be, st, WithInterval(time.Minute))
	podB := New(be, st, WithInterval(time.Minute))

	require.NoError(t, podA.Incr(testBoard, 1))
	ranA, err := podA.FlushIfLeader(context.Background())
	require.NoError(t, err)
	assert.True(t, ranA)
	assert.Equal(t, 50*time.Second, be.lockTTL, "락 TTL 은 주기보다 조금 짧다")

	require.NoError(t, podB.Incr(testBoard, 1))
	ranB, err := podB.FlushIfLeader(context.Background())
	require.NoError(t, err)
	assert.False(t, ranB, "락이 살아 있으면 다른 파드는 flush 하지 않는다")
	assert.Equal(t, 1, st.addCalls)
	assert.Equal(t, int64(1), st.get(testBoard, 1))

	be.expireLock()
	ranB, err = podB.FlushIfLeader(context.Background())
	require.NoError(t, err)
	assert.True(t, ranB)
	assert.Equal(t, int64(2), st.get(testBoard, 1))
}

func TestConcurrentFlushNoDoubleCount(t *testing.T) {
	be := newFakeBackend()
	st := newFakeStore()
	podA := New(be, st)
	podB := New(be, st)

	for i := 0; i < 100; i++ {
		require.NoError(t, podA.Incr(testBoard, i%10+1))
	}
	var wg sync.WaitGroup
	for _, p := range []*Buffer{podA, podB, podA, podB} {
		wg.Add(1)
		go func(p *Buffer) {
			defer wg.Done()
			assert.NoError(t, p.Flush(context.Background()))
		}(p)
	}
	wg.Wait()
	for id := 1; id <= 10; id++ {
		assert.Equal(t, int64(10), st.get(testBoard, id), "wr_id=%d", id)
	}
}

func TestIncrFallsBackOnBackendError(t *testing.T) {
	be := newFakeBackend()
	be.incrErr = errBoom
	st := newFakeStore()
	b := New(be, st)

	require.NoError(t, b.Incr(testBoard, 3))
	assert.Equal(t, 1, st.direct)
	assert.Equal(t, int64(1), st.get(testBoard, 3))
	assert.Empty(t, be.buffered())
}

func TestIncrWithoutBackendIsDirect(t *testing.T) {
	st := newFakeStore()
	b := New(nil, st)
	require.NoError(t, b.Incr(testBoard, 3))
	assert.Equal(t, 1, st.direct)
	b.Start()
	b.Stop() // 백엔드 없이도 멈춘다
}

func TestInvalidBoardRejected(t *testing.T) {
	be := newFakeBackend()
	st := newFakeStore()
	b := New(be, st)

	for _, bad := range []string{"", "free;DROP TABLE x", "free`", "a b", "../x", "hits:x"} {
		assert.ErrorIs(t, b.Incr(bad, 1), ErrInvalidBoardID, bad)
	}
	assert.ErrorIs(t, b.Incr(testBoard, 0), ErrInvalidPostID)
	assert.Empty(t, be.buffered())
	assert.Equal(t, 0, st.direct)

	// Redis 에 섞여 들어온 잘못된 게시판 ID 는 DB 로 가지 않고 목록에서 빠진다.
	be.tables["x`; DROP"] = true
	be.hashes[LiveKey("x`; DROP")] = map[string]int64{"1": 1}
	require.NoError(t, b.Flush(context.Background()))
	assert.Equal(t, 0, st.addCalls)
	assert.NotContains(t, be.tables, "x`; DROP")
}

func TestFlushDropsMalformedFields(t *testing.T) {
	be := newFakeBackend()
	st := newFakeStore()
	b := New(be, st)

	be.tables[testBoard] = true
	be.hashes[LiveKey(testBoard)] = map[string]int64{"5": 2, "abc": 1, "-1": 1, "6": 0}
	require.NoError(t, b.Flush(context.Background()))
	assert.Equal(t, int64(2), st.get(testBoard, 5))
	assert.Len(t, st.hits[testBoard], 1)
	assert.Empty(t, be.buffered())
}

func TestStopFlushesBuffered(t *testing.T) {
	be := newFakeBackend()
	st := newFakeStore()
	b := New(be, st, WithInterval(time.Hour))
	b.Start()

	require.NoError(t, b.Incr(testBoard, 1))
	require.NoError(t, b.Incr(testBoard, 1))
	// 다른 파드가 리더 락을 쥐고 있어도 종료 flush 는 돈다.
	be.lock = "other"
	b.Stop()
	assert.Equal(t, int64(2), st.get(testBoard, 1))
	assert.Empty(t, be.buffered())
	b.Stop() // 두 번 불러도 안전
}

func TestLockTTLShorterThanInterval(t *testing.T) {
	st := newFakeStore()
	assert.Equal(t, DefaultInterval-10*time.Second, New(nil, st).lockTTL())
	assert.Equal(t, 15*time.Second, New(nil, st, WithInterval(15*time.Second)).lockTTL())
}

func TestStopCancelsInFlightFlush(t *testing.T) {
	be := newFakeBackend()
	st := newFakeStore()
	st.entered = make(chan struct{})
	b := New(be, st, WithInterval(5*time.Millisecond))

	require.NoError(t, b.Incr(testBoard, 1))
	b.Start()
	select {
	case <-st.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("flush did not start")
	}

	// 진행 중 flush 가 컨텍스트 취소로 바로 끝나야 Stop 이 상한 안에 돌아온다.
	started := time.Now()
	b.Stop()
	assert.Less(t, time.Since(started), StopTimeout)

	// 취소된 묶음은 버퍼로 되돌아갔다가 종료 flush 로 반영된다 — 누락·이중 없음.
	assert.Equal(t, int64(1), st.get(testBoard, 1))
	assert.Empty(t, be.buffered())
	assert.Empty(t, be.pending)
}

func TestAckFailureLeavesKeyForRecovery(t *testing.T) {
	be := newFakeBackend()
	be.ackErr = errBoom
	st := newFakeStore()
	now := time.Unix(1_800_000_000, 0)
	b := New(be, st, WithChunkSize(2), WithClock(func() time.Time { return now }))

	for id := 1; id <= 4; id++ {
		require.NoError(t, b.Incr(testBoard, id))
	}
	require.Error(t, b.Flush(context.Background()))
	// 첫 묶음은 DB 에 반영됐지만 HDEL 이 실패해 처리 중 키에 그대로 남는다.
	assert.Equal(t, int64(1), st.get(testBoard, 1))
	assert.Equal(t, int64(0), st.get(testBoard, 3))
	assert.Len(t, be.pending, 1)
	assert.Equal(t, map[string]int64{"1": 1, "2": 1, "3": 1, "4": 1}, be.buffered())

	// staleAfter 뒤 회수되어 남은 몫이 반영된다. 이미 반영된 첫 묶음은 한 번 더 더해진다
	// (패키지 문서의 「최소 한 번」 창) — 조회수가 사라지지는 않는다.
	be.ackErr = nil
	now = now.Add(DefaultStaleAfter)
	require.NoError(t, b.Flush(context.Background()))
	assert.Equal(t, int64(2), st.get(testBoard, 1))
	assert.Equal(t, int64(2), st.get(testBoard, 2))
	assert.Equal(t, int64(1), st.get(testBoard, 3))
	assert.Equal(t, int64(1), st.get(testBoard, 4))
	assert.Empty(t, be.pending)
	assert.Empty(t, be.buffered())
}

func TestMissingTableKeepsCountsAndBacksOff(t *testing.T) {
	be := newFakeBackend()
	st := newFakeStore()
	st.missing[testBoard] = true
	now := time.Unix(1_800_000_000, 0)
	b := New(be, st, WithClock(func() time.Time { return now }))

	require.NoError(t, b.Incr(testBoard, 1))
	require.NoError(t, b.Incr(testBoard, 1))
	require.NoError(t, b.Flush(context.Background()), "테이블 없음은 flush 오류로 올리지 않는다")
	assert.Equal(t, 1, st.addCalls)
	assert.Equal(t, map[string]int64{"1": 2}, be.buffered(), "값은 버퍼에 남는다")
	assert.Empty(t, be.pending)

	// 한 시간 동안은 DB 에 다시 묻지 않는다.
	require.NoError(t, b.Incr(testBoard, 1))
	now = now.Add(30 * time.Minute)
	require.NoError(t, b.Flush(context.Background()))
	assert.Equal(t, 1, st.addCalls)

	// 테이블이 돌아오면 쌓인 값이 모두 반영된다.
	st.missing[testBoard] = false
	now = now.Add(31 * time.Minute)
	require.NoError(t, b.Flush(context.Background()))
	assert.Equal(t, int64(3), st.get(testBoard, 1))
	assert.Empty(t, be.buffered())
}
