package gnuboard

import (
	"errors"
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ⛔ 이 파일이 지키는 계약 (글 상세 예약삭제 조회 게이트):
//
//   - FindByPostForView 는 pending 집합에 없는 글이면 행 조회 없이 not found 를 돌려준다. 집합은 프로세스에
//     캐시되어 TTL 안에서는 다시 읽지 않는다.
//   - 집합에 있으면 FindByPost 와 같은 행을 돌려준다.
//   - 집합 조회가 실패하면 캐시하지 않고 FindByPost 로 대신한다(fail-open).
//   - Create·Cancel·MarkExecuted·DeleteByPost 가 성공하면 집합을 비운다.
//   - 쓰기·권한 경로가 쓰는 FindByPost 는 집합과 무관하게 항상 직접 조회한다.

func newScheduledDeleteTestRepo(t *testing.T) (*scheduledDeleteRepository, *gorm.DB, *sqlCountingLogger) {
	t.Helper()
	counter := &sqlCountingLogger{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: counter})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB 얻기 실패: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&gnuboard.ScheduledDelete{}); err != nil {
		t.Fatalf("테이블 생성 실패: %v", err)
	}

	// pending 집합 캐시는 패키지 전역이다 — 테스트마다 새 DB 를 보게 비운다.
	InvalidateScheduledPending()
	t.Cleanup(InvalidateScheduledPending)
	counter.reset()
	return &scheduledDeleteRepository{db: db}, db, counter
}

func pendingRow(boTable string, wrID int) *gnuboard.ScheduledDelete {
	now := time.Now()
	return &gnuboard.ScheduledDelete{
		BoTable:      boTable,
		WrID:         wrID,
		DelayMinutes: 5,
		ScheduledAt:  now.Add(5 * time.Minute),
		RequestedBy:  "tester",
		RequestedAt:  now,
		Status:       "pending",
	}
}

func TestFindByPostForView_NotInSetSkipsRowQuery(t *testing.T) {
	repo, _, counter := newScheduledDeleteTestRepo(t)

	for i := 0; i < 3; i++ {
		sd, err := repo.FindByPostForView("free", 10)
		if sd != nil || !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("집합에 없는 글은 not found 여야 한다: sd=%v err=%v", sd, err)
		}
	}
	sqls := counter.snapshot()
	if got := countSQL(sqls, "WHERE status = 'pending'"); got != 1 {
		t.Fatalf("집합 조회는 TTL 안에서 1번이어야 한다: %d (%v)", got, sqls)
	}
	if got := countSQL(sqls, "bo_table = "); got != 0 {
		t.Fatalf("집합에 없으면 행 조회가 없어야 한다: %v", sqls)
	}
}

func TestFindByPostForView_InSetReturnsSameRowAsFindByPost(t *testing.T) {
	repo, _, _ := newScheduledDeleteTestRepo(t)
	if err := repo.Create(pendingRow("free", 20)); err != nil {
		t.Fatalf("Create 실패: %v", err)
	}

	got, err := repo.FindByPostForView("free", 20)
	if err != nil || got == nil {
		t.Fatalf("집합에 있는 글은 행이 나와야 한다: %v", err)
	}
	want, err := repo.FindByPost("free", 20)
	if err != nil {
		t.Fatalf("FindByPost 실패: %v", err)
	}
	if got.ID != want.ID || got.DelayMinutes != want.DelayMinutes || !got.ScheduledAt.Equal(want.ScheduledAt) ||
		!got.RequestedAt.Equal(want.RequestedAt) || got.RequestedBy != want.RequestedBy {
		t.Fatalf("FindByPost 와 다르다: got=%+v want=%+v", got, want)
	}
	// 같은 wr_id 의 다른 게시판은 집합에 없다.
	if sd, _ := repo.FindByPostForView("qa", 20); sd != nil {
		t.Fatalf("다른 게시판 같은 wr_id 가 섞였다: %+v", sd)
	}
}

func TestFindByPostForView_StaleSetUntilInvalidated(t *testing.T) {
	repo, db, _ := newScheduledDeleteTestRepo(t)

	// 집합을 먼저 적재한다(비어 있음).
	if sd, _ := repo.FindByPostForView("free", 30); sd != nil {
		t.Fatalf("아직 행이 없다: %+v", sd)
	}
	// 저장소를 거치지 않고 넣으면 무효화가 없다 — TTL 안에서는 집합이 그대로다.
	if err := db.Create(pendingRow("free", 30)).Error; err != nil {
		t.Fatalf("직접 삽입 실패: %v", err)
	}
	if sd, _ := repo.FindByPostForView("free", 30); sd != nil {
		t.Fatalf("캐시된 집합을 써야 한다(무효화 전): %+v", sd)
	}
	// 쓰기·권한 경로의 FindByPost 는 집합과 무관하게 직접 읽는다.
	if sd, err := repo.FindByPost("free", 30); err != nil || sd == nil {
		t.Fatalf("FindByPost 는 직접 조회여야 한다: %v", err)
	}

	InvalidateScheduledPending()
	if sd, err := repo.FindByPostForView("free", 30); err != nil || sd == nil {
		t.Fatalf("무효화 뒤에는 보여야 한다: %v", err)
	}
}

func TestScheduledDeleteWrites_InvalidatePendingSet(t *testing.T) {
	repo, _, _ := newScheduledDeleteTestRepo(t)

	// Create — 적재된 빈 집합이 비워져 바로 보인다.
	_, _ = repo.FindByPostForView("free", 40)
	if err := repo.Create(pendingRow("free", 40)); err != nil {
		t.Fatalf("Create 실패: %v", err)
	}
	sd, err := repo.FindByPostForView("free", 40)
	if err != nil || sd == nil {
		t.Fatalf("Create 뒤 바로 보여야 한다: %v", err)
	}

	// Cancel — 집합에서 빠진다.
	if err := repo.Cancel(sd.ID); err != nil {
		t.Fatalf("Cancel 실패: %v", err)
	}
	if got, _ := repo.FindByPostForView("free", 40); got != nil {
		t.Fatalf("Cancel 뒤에는 없어야 한다: %+v", got)
	}

	// 다시 Create(취소 행 정리 후 새 행) → MarkExecuted — 집합에서 빠진다.
	if err := repo.Create(pendingRow("free", 40)); err != nil {
		t.Fatalf("재 Create 실패: %v", err)
	}
	sd, err = repo.FindByPostForView("free", 40)
	if err != nil || sd == nil {
		t.Fatalf("재 Create 뒤 보여야 한다: %v", err)
	}
	if err := repo.MarkExecuted(sd.ID); err != nil {
		t.Fatalf("MarkExecuted 실패: %v", err)
	}
	if got, _ := repo.FindByPostForView("free", 40); got != nil {
		t.Fatalf("MarkExecuted 뒤에는 없어야 한다: %+v", got)
	}

	// DeleteByPost — 집합을 비운다.
	if err := repo.Create(pendingRow("free", 41)); err != nil {
		t.Fatalf("Create 실패: %v", err)
	}
	if got, _ := repo.FindByPostForView("free", 41); got == nil {
		t.Fatal("Create 뒤 보여야 한다")
	}
	if err := repo.DeleteByPost("free", 41); err != nil {
		t.Fatalf("DeleteByPost 실패: %v", err)
	}
	if got, _ := repo.FindByPostForView("free", 41); got != nil {
		t.Fatalf("DeleteByPost 뒤에는 없어야 한다: %+v", got)
	}
}

func TestFindByPostForView_LoadFailureFallsBackToFindByPost(t *testing.T) {
	repo, _, _ := newScheduledDeleteTestRepo(t)
	if err := repo.Create(pendingRow("free", 50)); err != nil {
		t.Fatalf("Create 실패: %v", err)
	}

	calls := 0
	prev := scheduledPendingLoader
	scheduledPendingLoader = func(*gorm.DB) (map[scheduledPendingKey]struct{}, error) {
		calls++
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { scheduledPendingLoader = prev })

	for i := 0; i < 2; i++ {
		sd, err := repo.FindByPostForView("free", 50)
		if err != nil || sd == nil || sd.WrID != 50 {
			t.Fatalf("집합 조회 실패면 FindByPost 로 대신해야 한다: sd=%v err=%v", sd, err)
		}
	}
	if sd, err := repo.FindByPostForView("free", 51); sd != nil || err == nil {
		t.Fatalf("행 없는 글은 FindByPost 처럼 not found: sd=%v err=%v", sd, err)
	}
	if calls != 3 {
		t.Fatalf("실패는 캐시하지 않아야 한다(매번 재시도): calls=%d", calls)
	}
}

func TestScheduledPendingCache_TTLAndGeneration(t *testing.T) {
	c := &scheduledPendingCache{}
	now := time.Now()
	set := map[scheduledPendingKey]struct{}{{boTable: "free", wrID: 1}: {}}

	_, gen, ok := c.get(now)
	if ok {
		t.Fatal("빈 캐시가 적중했다")
	}
	c.put(set, gen, now)
	if _, _, ok := c.get(now.Add(scheduledPendingTTL - time.Second)); !ok {
		t.Fatal("TTL 안에서는 적중해야 한다")
	}
	if _, _, ok := c.get(now.Add(scheduledPendingTTL)); ok {
		t.Fatal("TTL 이 지나면 미스여야 한다")
	}

	// 조회 도중 무효화가 끼어들면 그 결과는 넣지 않는다.
	_, gen, _ = c.get(now.Add(scheduledPendingTTL))
	c.invalidate()
	c.put(set, gen, now)
	if _, _, ok := c.get(now); ok {
		t.Fatal("무효화 전 세대의 조회 결과가 적재됐다")
	}
}
