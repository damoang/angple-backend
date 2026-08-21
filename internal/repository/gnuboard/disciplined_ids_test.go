package gnuboard

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ⛔ 이 파일이 지키는 계약:
//
//	loadDisciplinedIDs / FindDisciplinedIDs 는 조회 실패를 **삼키지 않는다.**
//
// 반환값이 「가려야 할 글 목록」이라, 실패가 빈 집합으로 나가면 호출부에서
// 「가릴 게 없다」로 읽힌다. 그러면 이용제한 근거글의 **원제목이 공개 프로필에 그대로 나간다.**
//
// 2026-08-21 실측: free 한 곳에서만 692글 · 341명이 이 마스킹만을 보호막으로 삼는다
// (나머지는 삭제·잠금·비밀글이라 어차피 안 보인다).
//
// 예전 코드는 FindDisciplinedIDs 의 return 경로가 둘 다 (result, nil) 이라
// **error 를 낼 수가 없었고**, 소비자의 `if err == nil` 가드가 구조적으로 무의미했다.
// ⭐ 판별 기준은 `!= nil` 이 아니라 **`err == nil` 인데 값이 비어 있을 수 있는가** 다.

func resetDisciplinedIDsCache() {
	disciplinedIDsCache.Lock()
	disciplinedIDsCache.byBoard = nil
	disciplinedIDsCache.expires = nil
	disciplinedIDsCache.Unlock()
}

// newDisciplinedTestRepo 는 sqlite in-memory 로 저장소를 만든다.
//
// ⭐ sqlite 에는 g5_na_singo 테이블이 없다. 그래서 근거글 조회가 **반드시 실패**한다 —
// 여기서는 그게 버그가 아니라 검증 수단이다. 그 실패가 조용히 빈 집합으로
// 나가지 않는지를 본다.
func newDisciplinedTestRepo(t *testing.T) *myPageRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	return &myPageRepository{db: db}
}

func TestLoadDisciplinedIDs_ErrorsWhenNoCache(t *testing.T) {
	resetDisciplinedIDsCache()
	r := newDisciplinedTestRepo(t)

	set, err := r.loadDisciplinedIDs("free")

	if err == nil {
		t.Fatalf("error 를 기대했다. set=%v — 이 조합(err==nil + 빈 집합)이 원제목 노출을 만든다", set)
	}
	if set != nil {
		t.Errorf("실패 시에는 집합을 주면 안 된다. set=%v", set)
	}
}

func TestFindDisciplinedIDs_PropagatesError(t *testing.T) {
	resetDisciplinedIDsCache()
	r := newDisciplinedTestRepo(t)

	set, err := r.FindDisciplinedIDs("free", []int{1, 2, 3})

	// ⛔ 여기가 핵심이다. 예전에는 (빈 map, nil) 이 나와서 호출부가
	//    "가릴 게 없다"로 읽고 원제목을 그대로 내보냈다.
	if err == nil {
		t.Fatalf("error 를 기대했다. set=%v — 마스킹이 조용히 사라지는 조합이다", set)
	}
	if len(set) != 0 {
		t.Errorf("실패 시에는 집합을 주면 안 된다. set=%v", set)
	}
}

// TestFindDisciplinedIDs_EmptyInputIsNotAnError 는 정상적으로 비는 경우와
// 실패를 구분한다. 입력이 없으면 조회할 것도 없으므로 error 가 아니다.
func TestFindDisciplinedIDs_EmptyInputIsNotAnError(t *testing.T) {
	resetDisciplinedIDsCache()
	r := newDisciplinedTestRepo(t)

	for _, tc := range []struct {
		name  string
		board string
		ids   []int
	}{
		{"보드 없음", "", []int{1}},
		{"id 없음", "free", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set, err := r.FindDisciplinedIDs(tc.board, tc.ids)
			if err != nil {
				t.Fatalf("입력이 비면 error 가 아니어야 한다: %v", err)
			}
			if set == nil {
				t.Error("빈 map 을 돌려줘야 한다 (nil 이 아니라)")
			}
		})
	}
}

// TestLoadDisciplinedIDs_FallsBackToStaleCache 는 조회가 실패해도
// **만료된 캐시가 있으면 그걸 쓰는지** 본다.
//
// ⭐ 「몇 분 전의 근거글 목록」은 「근거글 없음」보다 압도적으로 정확하다.
// 제재는 하루 몇 건 수준이라 그 사이 새로 생긴 근거글은 사실상 없다.
func TestLoadDisciplinedIDs_FallsBackToStaleCache(t *testing.T) {
	resetDisciplinedIDsCache()
	r := newDisciplinedTestRepo(t)

	// 만료된 캐시를 심는다.
	disciplinedIDsCache.Lock()
	disciplinedIDsCache.byBoard = map[string]map[int]bool{"free": {42: true}}
	disciplinedIDsCache.expires = map[string]time.Time{"free": time.Now().Add(-time.Minute)}
	disciplinedIDsCache.Unlock()

	set, err := r.loadDisciplinedIDs("free")

	if err != nil {
		t.Fatalf("만료 캐시가 있으면 error 가 아니라 그 값을 써야 한다: %v", err)
	}
	if !set[42] {
		t.Errorf("만료된 캐시 내용이 유지돼야 한다. set=%v", set)
	}

	// 그리고 그 값이 FindDisciplinedIDs 까지 전달돼야 한다.
	got, err := r.FindDisciplinedIDs("free", []int{42, 43})
	if err != nil {
		t.Fatalf("만료 캐시 경로에서는 error 가 아니어야 한다: %v", err)
	}
	if !got[42] || got[43] {
		t.Errorf("요청한 id 만 걸러 돌려줘야 한다. got=%v", got)
	}
}
