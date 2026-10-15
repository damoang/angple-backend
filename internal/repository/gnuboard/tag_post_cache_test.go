package gnuboard

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ⛔ 이 파일이 지키는 계약 (글 상세 태그 조회 게이트):
//
//   - GetPostTagsForView 는 태그 있는 글 집합에 없는 글이면 행 조회 없이 nil 을 돌려준다(태그 없음).
//     집합은 프로세스에 캐시되어 TTL 안에서는 다시 읽지 않는다.
//   - 집합에 있으면 GetPostTags 와 같은 태그를 돌려준다.
//   - 집합 조회가 실패하면 캐시하지 않고 GetPostTags 로 대신한다(fail-open).
//   - SetPostTags·DeletePostTags 가 성공하면 집합을 비운다.
//   - bo_table 은 대소문자·끝 공백을 무시하고 맞춘다(게이트가 DB 비교보다 엄격하지 않게).

const taggedPostSetSQL = "SELECT DISTINCT bo_table, wr_id FROM g5_na_tag_log"
const taggedPostRowSQL = "WHERE bo_table = "

func newTagGateTestRepo(t *testing.T) (*tagRepository, *gorm.DB, *sqlCountingLogger) {
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
	if err := db.AutoMigrate(&gnuboard.G5NaTag{}, &gnuboard.G5NaTagLog{}); err != nil {
		t.Fatalf("테이블 생성 실패: %v", err)
	}

	// 태그 있는 글 집합 캐시는 패키지 전역이다 — 테스트마다 새 DB 를 보게 비운다.
	InvalidateTaggedPosts()
	t.Cleanup(InvalidateTaggedPosts)
	counter.reset()
	return &tagRepository{db: db}, db, counter
}

func tagLogRow(boTable string, wrID int, tag string) *gnuboard.G5NaTagLog {
	return &gnuboard.G5NaTagLog{BoTable: boTable, WrID: wrID, TagID: 1, Tag: tag, MbID: "tester", RegDate: time.Now()}
}

func TestGetPostTagsForView_NotInSetSkipsRowQuery(t *testing.T) {
	repo, db, counter := newTagGateTestRepo(t)
	if err := db.Create(tagLogRow("free", 11, "a")).Error; err != nil {
		t.Fatalf("삽입 실패: %v", err)
	}
	counter.reset()

	for i := 0; i < 3; i++ {
		tags, err := repo.GetPostTagsForView("free", 10)
		if err != nil || len(tags) != 0 {
			t.Fatalf("집합에 없는 글은 태그가 없어야 한다: tags=%v err=%v", tags, err)
		}
	}
	sqls := counter.snapshot()
	if got := countSQL(sqls, taggedPostSetSQL); got != 1 {
		t.Fatalf("집합 조회는 TTL 안에서 1번이어야 한다: %d (%v)", got, sqls)
	}
	if got := countSQL(sqls, taggedPostRowSQL); got != 0 {
		t.Fatalf("집합에 없으면 행 조회가 없어야 한다: %v", sqls)
	}
}

func TestGetPostTagsForView_InSetReturnsSameAsGetPostTags(t *testing.T) {
	repo, db, counter := newTagGateTestRepo(t)
	for _, tag := range []string{"x", "y"} {
		if err := db.Create(tagLogRow("free", 20, tag)).Error; err != nil {
			t.Fatalf("삽입 실패: %v", err)
		}
	}
	counter.reset()

	got, err := repo.GetPostTagsForView("free", 20)
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if got2 := countSQL(counter.snapshot(), taggedPostRowSQL); got2 != 1 {
		t.Fatalf("집합에 있으면 행을 1번 읽어야 한다: %d", got2)
	}
	want, err := repo.GetPostTags("free", 20)
	if err != nil {
		t.Fatalf("GetPostTags 실패: %v", err)
	}
	if !reflect.DeepEqual(got, want) || len(got) != 2 {
		t.Fatalf("GetPostTags 와 다르다: got=%v want=%v", got, want)
	}
	// 같은 wr_id 의 다른 게시판은 집합에 없다.
	if tags, _ := repo.GetPostTagsForView("qa", 20); len(tags) != 0 {
		t.Fatalf("다른 게시판 같은 wr_id 가 섞였다: %v", tags)
	}
}

func TestGetPostTagsForView_BoTableCaseInsensitive(t *testing.T) {
	repo, db, counter := newTagGateTestRepo(t)
	if err := db.Create(tagLogRow("Free", 30, "a")).Error; err != nil {
		t.Fatalf("삽입 실패: %v", err)
	}
	counter.reset()

	// 저장값과 요청값의 대소문자·끝 공백이 달라도 게이트는 통과시켜 행 조회로 넘긴다.
	for _, bo := range []string{"free", "FREE", "Free", "free "} {
		_, _ = repo.GetPostTagsForView(bo, 30)
	}
	if got := countSQL(counter.snapshot(), taggedPostRowSQL); got != 4 {
		t.Fatalf("대소문자·끝 공백만 다른 게시판은 게이트를 통과해야 한다: %d", got)
	}
	if normalizeTagBoTable("Free ") != "free" {
		t.Fatalf("정규화가 맞지 않다: %q", normalizeTagBoTable("Free "))
	}
}

func TestGetPostTagsForView_StaleSetUntilInvalidated(t *testing.T) {
	repo, db, _ := newTagGateTestRepo(t)

	// 집합을 먼저 적재한다(비어 있음).
	if tags, _ := repo.GetPostTagsForView("free", 40); len(tags) != 0 {
		t.Fatalf("아직 태그가 없다: %v", tags)
	}
	// 저장소를 거치지 않고 넣으면 무효화가 없다 — TTL 안에서는 집합이 그대로다.
	if err := db.Create(tagLogRow("free", 40, "a")).Error; err != nil {
		t.Fatalf("직접 삽입 실패: %v", err)
	}
	if tags, _ := repo.GetPostTagsForView("free", 40); len(tags) != 0 {
		t.Fatalf("캐시된 집합을 써야 한다(무효화 전): %v", tags)
	}
	// GetPostTags 는 집합과 무관하게 직접 읽는다.
	if tags, err := repo.GetPostTags("free", 40); err != nil || len(tags) != 1 {
		t.Fatalf("GetPostTags 는 직접 조회여야 한다: tags=%v err=%v", tags, err)
	}

	InvalidateTaggedPosts()
	if tags, err := repo.GetPostTagsForView("free", 40); err != nil || len(tags) != 1 {
		t.Fatalf("무효화 뒤에는 보여야 한다: tags=%v err=%v", tags, err)
	}
}

func TestTagWrites_InvalidateTaggedPostSet(t *testing.T) {
	repo, _, _ := newTagGateTestRepo(t)

	// SetPostTags — 적재된 빈 집합이 비워져 바로 보인다.
	_, _ = repo.GetPostTagsForView("free", 50)
	if err := repo.SetPostTags("free", 50, []string{"a", "b"}, "tester"); err != nil {
		t.Fatalf("SetPostTags 실패: %v", err)
	}
	tags, err := repo.GetPostTagsForView("free", 50)
	if err != nil || len(tags) != 2 {
		t.Fatalf("SetPostTags 뒤 바로 보여야 한다: tags=%v err=%v", tags, err)
	}

	// SetPostTags(빈 목록) — 집합에서 빠진다.
	if err := repo.SetPostTags("free", 50, nil, "tester"); err != nil {
		t.Fatalf("SetPostTags(빈) 실패: %v", err)
	}
	if got, _ := repo.GetPostTagsForView("free", 50); len(got) != 0 {
		t.Fatalf("태그를 비운 뒤에는 없어야 한다: %v", got)
	}

	// DeletePostTags — 집합을 비운다.
	if err := repo.SetPostTags("free", 51, []string{"c"}, "tester"); err != nil {
		t.Fatalf("SetPostTags 실패: %v", err)
	}
	if got, _ := repo.GetPostTagsForView("free", 51); len(got) != 1 {
		t.Fatalf("SetPostTags 뒤 보여야 한다: %v", got)
	}
	if err := repo.DeletePostTags("free", 51); err != nil {
		t.Fatalf("DeletePostTags 실패: %v", err)
	}
	if got, _ := repo.GetPostTagsForView("free", 51); len(got) != 0 {
		t.Fatalf("DeletePostTags 뒤에는 없어야 한다: %v", got)
	}
}

func TestGetPostTagsForView_LoadFailureFallsBackToGetPostTags(t *testing.T) {
	repo, db, _ := newTagGateTestRepo(t)
	if err := db.Create(tagLogRow("free", 60, "a")).Error; err != nil {
		t.Fatalf("삽입 실패: %v", err)
	}

	calls := 0
	prev := taggedPostLoader
	taggedPostLoader = func(*gorm.DB) (map[taggedPostKey]struct{}, error) {
		calls++
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { taggedPostLoader = prev })

	for i := 0; i < 2; i++ {
		tags, err := repo.GetPostTagsForView("free", 60)
		if err != nil || len(tags) != 1 || tags[0] != "a" {
			t.Fatalf("집합 조회 실패면 GetPostTags 로 대신해야 한다: tags=%v err=%v", tags, err)
		}
	}
	if tags, err := repo.GetPostTagsForView("free", 61); err != nil || len(tags) != 0 {
		t.Fatalf("태그 없는 글은 GetPostTags 처럼 빈 결과: tags=%v err=%v", tags, err)
	}
	if calls != 3 {
		t.Fatalf("실패는 캐시하지 않아야 한다(매번 재시도): calls=%d", calls)
	}
}

func TestTaggedPostCache_TTLAndGeneration(t *testing.T) {
	c := &taggedPostCache{}
	now := time.Now()
	set := map[taggedPostKey]struct{}{{boTable: "free", wrID: 1}: {}}

	_, gen, ok := c.get(now)
	if ok {
		t.Fatal("빈 캐시가 적중했다")
	}
	c.put(set, gen, now)
	if _, _, ok := c.get(now.Add(taggedPostTTL - time.Second)); !ok {
		t.Fatal("TTL 안에서는 적중해야 한다")
	}
	if _, _, ok := c.get(now.Add(taggedPostTTL)); ok {
		t.Fatal("TTL 이 지나면 미스여야 한다")
	}

	// 조회 도중 무효화가 끼어들면 그 결과는 넣지 않는다.
	_, gen, _ = c.get(now.Add(taggedPostTTL))
	c.invalidate()
	c.put(set, gen, now)
	if _, _, ok := c.get(now); ok {
		t.Fatal("무효화 전 세대의 조회 결과가 적재됐다")
	}
}
