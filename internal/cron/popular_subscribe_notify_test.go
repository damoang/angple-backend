package cron

import (
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openPopularTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err != nil {
			return
		}
		if err := sqlDB.Close(); err != nil {
			t.Logf("close sqlite: %v", err)
		}
	})
	if err := db.Exec(`CREATE TABLE g5_write_free (
		wr_id INTEGER PRIMARY KEY,
		wr_is_comment INTEGER NOT NULL DEFAULT 0,
		wr_good INTEGER NOT NULL DEFAULT 0,
		wr_datetime DATETIME NOT NULL,
		wr_subject TEXT NOT NULL DEFAULT '',
		mb_id TEXT NOT NULL DEFAULT '',
		wr_name TEXT NOT NULL DEFAULT ''
	)`).Error; err != nil {
		t.Fatalf("create g5_write_free: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_board_subscribe_notified (
		bo_table TEXT NOT NULL,
		wr_id INTEGER NOT NULL,
		PRIMARY KEY (bo_table, wr_id)
	)`).Error; err != nil {
		t.Fatalf("create g5_board_subscribe_notified: %v", err)
	}
	return db
}

func seedPopularPost(t *testing.T, db *gorm.DB, id, isComment, good int, at time.Time) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO g5_write_free (wr_id, wr_is_comment, wr_good, wr_datetime, wr_subject, mb_id, wr_name) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, isComment, good, at, "subject", "author", "name",
	).Error; err != nil {
		t.Fatalf("seed post %d: %v", id, err)
	}
}

// seedPopularWindow seeds old posts before cutoff and a mix of posts inside the window.
func seedPopularWindow(t *testing.T, db *gorm.DB, cutoff time.Time) {
	t.Helper()
	old := cutoff.Add(-48 * time.Hour)
	recent := cutoff.Add(time.Hour)
	seedPopularPost(t, db, 1, 0, 50, old)    // 기간 밖 추천글 — 대상 아님
	seedPopularPost(t, db, 2, 0, 3, old)     // 기간 밖 일반글
	seedPopularPost(t, db, 3, 0, 1, recent)  // 기간 안 첫 글(하한) — 추천 미달
	seedPopularPost(t, db, 4, 0, 20, recent) // 기간 안 추천글 — 대상
	seedPopularPost(t, db, 5, 0, 30, recent) // 기간 안 추천글 — 이미 통지됨
	seedPopularPost(t, db, 6, 1, 40, recent) // 댓글 — 대상 아님
	seedPopularPost(t, db, 7, 0, 10, recent) // 기간 안 추천글(임계값 경계) — 대상
	if err := db.Exec(`INSERT INTO g5_board_subscribe_notified (bo_table, wr_id) VALUES ('free', 5)`).Error; err != nil {
		t.Fatalf("seed notified: %v", err)
	}
}

func popularIDs(rows []popularPostRow) []int {
	ids := make([]int, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.WrID)
	}
	return ids
}

func assertPopularIDs(t *testing.T, got []popularPostRow, want []int) {
	t.Helper()
	ids := popularIDs(got)
	if len(ids) != len(want) {
		t.Fatalf("candidates = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("candidates = %v, want %v", ids, want)
		}
	}
}

func TestSelectPopularCandidatesUsesWindowFloor(t *testing.T) {
	db := openPopularTestDB(t, "popular_floor")
	cutoff := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	seedPopularWindow(t, db, cutoff)

	floor, found, err := popularPostFloor(db, "free", cutoff)
	if err != nil {
		t.Fatalf("popularPostFloor: %v", err)
	}
	if !found || floor != 3 {
		t.Fatalf("floor = %d (found=%v), want 3", floor, found)
	}

	posts, err := selectPopularCandidates(db, "free", 10, cutoff, 200, true)
	if err != nil {
		t.Fatalf("selectPopularCandidates: %v", err)
	}
	assertPopularIDs(t, posts, []int{4, 7})
	if posts[0].Subject != "subject" || posts[0].MbID != "author" || posts[0].WrName != "name" {
		t.Fatalf("unexpected row fields: %+v", posts[0])
	}

	limited, err := selectPopularCandidates(db, "free", 10, cutoff, 1, true)
	if err != nil {
		t.Fatalf("selectPopularCandidates limit: %v", err)
	}
	assertPopularIDs(t, limited, []int{4})
}

func TestSelectPopularCandidatesSkipsBoardWithoutRecentPosts(t *testing.T) {
	db := openPopularTestDB(t, "popular_empty")
	cutoff := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	seedPopularPost(t, db, 1, 0, 50, cutoff.Add(-48*time.Hour))

	floor, found, err := popularPostFloor(db, "free", cutoff)
	if err != nil {
		t.Fatalf("popularPostFloor: %v", err)
	}
	if found {
		t.Fatalf("expected no floor, got %d", floor)
	}

	posts, err := selectPopularCandidates(db, "free", 10, cutoff, 200, true)
	if err != nil {
		t.Fatalf("selectPopularCandidates: %v", err)
	}
	if len(posts) != 0 {
		t.Fatalf("expected no candidates, got %v", popularIDs(posts))
	}
}

func TestSelectPopularCandidatesFallsBackWhenFloorFails(t *testing.T) {
	db := openPopularTestDB(t, "popular_fallback")
	cutoff := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	seedPopularWindow(t, db, cutoff)

	orig := popularPostFloor
	popularPostFloor = func(*gorm.DB, string, time.Time) (int64, bool, error) {
		return 0, false, errors.New("floor lookup failed")
	}
	t.Cleanup(func() { popularPostFloor = orig })

	posts, err := selectPopularCandidates(db, "free", 10, cutoff, 200, true)
	if err != nil {
		t.Fatalf("selectPopularCandidates: %v", err)
	}
	assertPopularIDs(t, posts, []int{4, 7})
}

func TestSelectPopularCandidatesMissingTable(t *testing.T) {
	db := openPopularTestDB(t, "popular_missing")
	cutoff := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)

	if _, err := selectPopularCandidates(db, "nosuchboard", 10, cutoff, 200, true); err == nil {
		t.Fatal("expected error for missing board table")
	}
}

// stubPopularFloor replaces popularPostFloor and counts how often it is called.
func stubPopularFloor(t *testing.T) *int {
	t.Helper()
	calls := 0
	orig := popularPostFloor
	popularPostFloor = func(db *gorm.DB, board string, cutoff time.Time) (int64, bool, error) {
		calls++
		return orig(db, board, cutoff)
	}
	t.Cleanup(func() { popularPostFloor = orig })
	return &calls
}

// stubIndexLookup replaces popularDatetimeIndexedTables with a fixed answer.
func stubIndexLookup(t *testing.T, names []string, err error) *[]string {
	t.Helper()
	var asked []string
	orig := popularDatetimeIndexedTables
	popularDatetimeIndexedTables = func(_ *gorm.DB, tables []string) ([]string, error) {
		asked = append(asked, tables...)
		return names, err
	}
	t.Cleanup(func() { popularDatetimeIndexedTables = orig })
	return &asked
}

func TestPopularFloorBoardsOnlyIndexedBoards(t *testing.T) {
	db := openPopularTestDB(t, "popular_index_boards")
	asked := stubIndexLookup(t, []string{"g5_write_free"}, nil)

	got := popularFloorBoards(db, []string{"free", "car", "bad-slug"})
	if !got["free"] {
		t.Fatalf("expected free to use floor, got %v", got)
	}
	if got["car"] {
		t.Fatalf("expected car without floor, got %v", got)
	}
	if len(*asked) != 2 || (*asked)[0] != "g5_write_free" || (*asked)[1] != "g5_write_car" {
		t.Fatalf("lookup tables = %v, want [g5_write_free g5_write_car]", *asked)
	}
}

func TestPopularFloorBoardsLookupFailure(t *testing.T) {
	db := openPopularTestDB(t, "popular_index_fail")
	stubIndexLookup(t, []string{"g5_write_free"}, errors.New("lookup failed"))

	got := popularFloorBoards(db, []string{"free", "car"})
	if len(got) != 0 {
		t.Fatalf("expected no floor boards on lookup failure, got %v", got)
	}
}

func TestSelectPopularCandidatesIndexedBoardUsesFloor(t *testing.T) {
	db := openPopularTestDB(t, "popular_indexed")
	cutoff := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	seedPopularWindow(t, db, cutoff)
	stubIndexLookup(t, []string{"g5_write_free"}, nil)
	calls := stubPopularFloor(t)

	floorBoards := popularFloorBoards(db, []string{"free"})
	posts, err := selectPopularCandidates(db, "free", 10, cutoff, 200, floorBoards["free"])
	if err != nil {
		t.Fatalf("selectPopularCandidates: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("floor lookup calls = %d, want 1", *calls)
	}
	assertPopularIDs(t, posts, []int{4, 7})
}

func TestSelectPopularCandidatesNonIndexedBoardSkipsFloor(t *testing.T) {
	db := openPopularTestDB(t, "popular_nonindexed")
	cutoff := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	seedPopularWindow(t, db, cutoff)
	stubIndexLookup(t, nil, nil)
	calls := stubPopularFloor(t)

	floorBoards := popularFloorBoards(db, []string{"free"})
	posts, err := selectPopularCandidates(db, "free", 10, cutoff, 200, floorBoards["free"])
	if err != nil {
		t.Fatalf("selectPopularCandidates: %v", err)
	}
	if *calls != 0 {
		t.Fatalf("floor lookup calls = %d, want 0", *calls)
	}
	assertPopularIDs(t, posts, []int{4, 7})
}

func TestSelectPopularCandidatesIndexLookupFailureSkipsFloor(t *testing.T) {
	db := openPopularTestDB(t, "popular_index_lookup_fail")
	cutoff := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	seedPopularWindow(t, db, cutoff)
	stubIndexLookup(t, []string{"g5_write_free"}, errors.New("lookup failed"))
	calls := stubPopularFloor(t)

	floorBoards := popularFloorBoards(db, []string{"free"})
	posts, err := selectPopularCandidates(db, "free", 10, cutoff, 200, floorBoards["free"])
	if err != nil {
		t.Fatalf("selectPopularCandidates: %v", err)
	}
	if *calls != 0 {
		t.Fatalf("floor lookup calls = %d, want 0", *calls)
	}
	assertPopularIDs(t, posts, []int{4, 7})
}
