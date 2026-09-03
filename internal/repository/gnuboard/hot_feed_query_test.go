package gnuboard

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// hot_feed_query_test.go — 크로스보드 핫 피드 쿼리 빌더의 계약 테스트.
//
// 지키는 계약:
//  1. hours 는 화이트리스트(6/12/24/72)로 정규화된다 — 임의 값이 SQL 창으로 새지 않는다.
//  2. 보드별 분기는 반드시 2단 바운딩이다 — 내부 PK 역순 LIMIT 2000 이 보드당 스캔
//     상한이고, 시간 창 필터는 그 캡 안에서만 적용된다(wr_datetime 인덱스 비의존).
//  3. 병합 정렬은 wr_good DESC, wr_id DESC + LIMIT ? OFFSET ? 다.
//  4. 검증 안 된 slug 는 SQL 에 들어가지 않는다(인젝션 방지).

func TestNormalizeHotHours(t *testing.T) {
	cases := []struct{ in, want int }{
		{6, 6}, {12, 12}, {24, 24}, {72, 72}, // 허용값은 그대로
		{0, 24}, {-5, 24}, {1, 24}, {48, 24}, {100, 24}, {73, 24}, // 밖은 기본 24
	}
	for _, tc := range cases {
		if got := NormalizeHotHours(tc.in); got != tc.want {
			t.Errorf("NormalizeHotHours(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestBuildHotFeedQuery_Snapshot 은 단일 보드 케이스의 SQL 전문을 고정한다.
// 구조(2단 바운딩·필터·정렬)가 바뀌면 여기서 잡힌다 — 의도한 변경이면 스냅샷을 갱신하라.
func TestBuildHotFeedQuery_Snapshot(t *testing.T) {
	sql, args := buildHotFeedQuery([]string{"free"}, 24, 21, 0, nil)

	want := "SELECT * FROM (" +
		"(SELECT * FROM (" +
		"SELECT wr_id, wr_subject, LEFT(wr_content, 1000) AS wr_content, wr_datetime, wr_10," +
		" wr_hit, wr_good, wr_comment, mb_id, wr_name, wr_option, 'free' AS board_id" +
		" FROM `g5_write_free`" +
		" WHERE wr_is_comment = 0" +
		" AND (wr_option NOT LIKE '%secret%' OR wr_option IS NULL)" +
		" AND (wr_7 IS NULL OR wr_7 != 'lock')" +
		" AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')" +
		" ORDER BY wr_id DESC LIMIT 2000" +
		") t0 WHERE t0.wr_datetime >= DATE_SUB(NOW(), INTERVAL ? HOUR) AND t0.wr_good >= 1)" +
		") AS hot ORDER BY wr_good DESC, wr_id DESC LIMIT ? OFFSET ?"

	if sql != want {
		t.Errorf("SQL 스냅샷 불일치.\n got: %s\nwant: %s", sql, want)
	}
	wantArgs := []interface{}{24, 21, 0}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("args = %v, want %v", args, wantArgs)
	}
}

func TestBuildHotFeedQuery_BoundingPerBoard(t *testing.T) {
	boards := []string{"free", "qa", "new"}
	sql, args := buildHotFeedQuery(boards, 6, 21, 20, nil)

	// 보드 수만큼 UNION ALL 분기, 분기마다 내부 캡(PK 역순 LIMIT 2000)이 정확히 하나.
	if got := strings.Count(sql, " UNION ALL "); got != len(boards)-1 {
		t.Errorf("UNION ALL 수 = %d, want %d", got, len(boards)-1)
	}
	if got := strings.Count(sql, "ORDER BY wr_id DESC LIMIT 2000"); got != len(boards) {
		t.Errorf("내부 바운딩(ORDER BY wr_id DESC LIMIT 2000) 수 = %d, want %d — 캡 없는 분기는 풀스캔이 된다", got, len(boards))
	}
	// 시간 창 필터는 분기마다 캡 밖(서브쿼리 바깥)에서 적용된다 — wr_datetime 인덱스 비의존.
	if got := strings.Count(sql, "wr_datetime >= DATE_SUB(NOW(), INTERVAL ? HOUR)"); got != len(boards) {
		t.Errorf("시간 창 필터 수 = %d, want %d", got, len(boards))
	}
	if got := strings.Count(sql, "wr_good >= 1"); got != len(boards) {
		t.Errorf("wr_good >= 1 필터 수 = %d, want %d", got, len(boards))
	}
	// ⛔ 내부 서브쿼리에 시간 정렬이 있으면 안 된다(wr_datetime 정렬은 어디에도 없어야 한다).
	if strings.Contains(sql, "ORDER BY wr_datetime") {
		t.Error("wr_datetime 정렬이 존재한다 — 인덱스 없는 보드에서 filesort 가 난다")
	}
	// args: 분기마다 hours 하나 + 마지막에 limit, offset.
	wantArgs := []interface{}{6, 6, 6, 21, 20}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("args = %v, want %v", args, wantArgs)
	}
}

func TestBuildHotFeedQuery_OuterOrdering(t *testing.T) {
	sql, _ := buildHotFeedQuery([]string{"free", "qa"}, 24, 21, 0, nil)
	// 병합 정렬은 공감순(동률은 wr_id 역순) + limit/offset 플레이스홀더로 끝나야 한다.
	if !strings.HasSuffix(sql, ") AS hot ORDER BY wr_good DESC, wr_id DESC LIMIT ? OFFSET ?") {
		t.Errorf("외부 정렬/페이지네이션이 계약과 다르다: ...%s", sql[len(sql)-80:])
	}
}

func TestBuildHotFeedQuery_ExcludeMbIDs(t *testing.T) {
	blocked := []string{"baduser1", "baduser2"}
	sql, args := buildHotFeedQuery([]string{"free", "qa"}, 24, 21, 0, blocked)

	// 차단 필터는 분기마다 붙는다(어느 보드의 글이든 차단 대상은 빠져야 한다).
	if got := strings.Count(sql, "mb_id NOT IN ?"); got != 2 {
		t.Errorf("mb_id NOT IN 수 = %d, want 2", got)
	}
	// args 순서: (hours, blocked) × 보드 수, 마지막 limit, offset.
	wantArgs := []interface{}{24, blocked, 24, blocked, 21, 0}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("args = %v, want %v", args, wantArgs)
	}
}

func TestBuildHotFeedQuery_HoursNormalized(t *testing.T) {
	// 허용 밖 hours(48)는 빌더 안에서도 24 로 정규화돼 바인딩된다.
	_, args := buildHotFeedQuery([]string{"free"}, 48, 21, 0, nil)
	if len(args) == 0 || args[0] != 24 {
		t.Errorf("args[0] = %v, want 24 (허용 밖 hours 는 기본값으로)", args)
	}
}

func TestBuildHotFeedQuery_RejectsInvalidSlug(t *testing.T) {
	// GetSearchableBoards 를 거쳤어야 할 slug 지만, 방어적으로 한 번 더 거른다.
	sql, _ := buildHotFeedQuery([]string{"free", "bad-slug", "x; DROP TABLE g5_member", ""}, 24, 21, 0, nil)
	if strings.Contains(sql, "bad-slug") || strings.Contains(sql, "DROP TABLE") {
		t.Errorf("검증 안 된 slug 가 SQL 에 들어갔다: %s", sql)
	}
	if got := strings.Count(sql, "g5_write_"); got != 1 {
		t.Errorf("유효 보드는 free 하나여야 한다. g5_write_ 등장 수 = %d", got)
	}

	// 전부 무효면 빈 쿼리 — 호출부가 빈 결과로 처리한다.
	sql, args := buildHotFeedQuery([]string{"bad-slug", ""}, 24, 21, 0, nil)
	if sql != "" || args != nil {
		t.Errorf("무효 slug 만 있으면 (\"\", nil) 이어야 한다. got sql=%q args=%v", sql, args)
	}
}

// TestFindHotAcrossBoards_FailClosed — 보드 허용 목록을 못 얻으면 error 로 나간다.
// (nil, false, nil) 로 뭉개면 「핫 글 없음」과 「조회 실패」가 같은 값이 된다.
func TestFindHotAcrossBoards_FailClosed(t *testing.T) {
	resetSearchableBoardsCache()
	r := newTestRepo(t, &fakeBoardRepo{err: errors.New("db down")})

	rows, hasMore, err := r.FindHotAcrossBoards(24, 20, 0, nil)
	if err == nil {
		t.Fatalf("error 를 기대했다. rows=%v hasMore=%v", rows, hasMore)
	}
	if rows != nil {
		t.Errorf("실패 시에는 행을 주면 안 된다. rows=%v", rows)
	}
}

func TestFindHotAcrossBoards_NonPositiveLimit(t *testing.T) {
	resetSearchableBoardsCache()
	r := newTestRepo(t, &fakeBoardRepo{err: errors.New("db down")})

	// limit<=0 은 보드 조회 전에 빈 결과로 끝난다(쿼리를 만들 이유가 없다).
	rows, hasMore, err := r.FindHotAcrossBoards(24, 0, 0, nil)
	if err != nil || rows != nil || hasMore {
		t.Errorf("limit=0: got rows=%v hasMore=%v err=%v, want (nil, false, nil)", rows, hasMore, err)
	}
}
