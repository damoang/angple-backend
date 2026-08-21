package gnuboard

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ⛔ DisciplinedIDs 는 **네 표면이 공유하는 계약**이다:
//
//	회원 공개 프로필 · 게시판 글 목록 · 글 상세 · 댓글 목록
//
// 넷 다 이용제한 근거글 마스킹(또는 프런트 가림막 플래그)을 이 값으로 결정한다.
// 여기가 조용히 빈 집합을 돌려주면 **네 곳이 동시에 뚫린다.**
//
// 2026-08-21 에 이 계열 fail-open 을 다섯 곳에서 찾았고, 그때마다 각자 조회하고
// 각자 nil 로 뭉갰다. 그래서 조회를 한 곳으로 모으고 그 함수가 계약을 강제한다.
//
// ⭐ 판별 기준은 `!= nil` 이 아니라 **`err == nil` 인데 값이 비어 있을 수 있는가** 다.

func newSharedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// ⭐ sqlite 에는 g5_na_singo 가 없다. 조회가 **반드시 실패**한다 —
	//    여기서는 그게 버그가 아니라 검증 수단이다.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	return db
}

func TestDisciplinedIDs_ErrorsWhenNoCache(t *testing.T) {
	resetDisciplinedIDsCache()

	set, err := DisciplinedIDs(newSharedTestDB(t), "free")

	if err == nil {
		t.Fatalf("error 를 기대했다. set=%v — 이 조합(err==nil + 빈 집합)이 네 표면을 동시에 뚫는다", set)
	}
	if set != nil {
		t.Errorf("실패 시에는 집합을 주면 안 된다. set=%v", set)
	}
}

func TestDisciplinedIDs_FallsBackToStaleCache(t *testing.T) {
	resetDisciplinedIDsCache()
	disciplinedIDsCache.Lock()
	disciplinedIDsCache.byBoard = map[string]map[int]bool{"free": {7: true}}
	disciplinedIDsCache.expires = map[string]time.Time{"free": time.Now().Add(-time.Minute)}
	disciplinedIDsCache.Unlock()

	set, err := DisciplinedIDs(newSharedTestDB(t), "free")

	if err != nil {
		t.Fatalf("만료 캐시가 있으면 그 값을 써야 한다: %v", err)
	}
	if !set[7] {
		t.Errorf("만료된 캐시 내용이 유지돼야 한다. set=%v", set)
	}
}

// TestLoadDisciplinedIDs_DelegatesToShared 는 저장소 메서드가 공유 함수로
// 위임하는지 본다. 위임이 끊기면 계약이 다시 두 겹이 되고, 한쪽만 고쳐지는
// 상황이 재현된다 — 이 파일이 그걸 막는다.
func TestLoadDisciplinedIDs_DelegatesToShared(t *testing.T) {
	resetDisciplinedIDsCache()
	disciplinedIDsCache.Lock()
	disciplinedIDsCache.byBoard = map[string]map[int]bool{"free": {9: true}}
	disciplinedIDsCache.expires = map[string]time.Time{"free": time.Now().Add(time.Minute)}
	disciplinedIDsCache.Unlock()

	r := &myPageRepository{db: newSharedTestDB(t)}
	set, err := r.loadDisciplinedIDs("free")
	if err != nil {
		t.Fatalf("캐시 히트인데 error: %v", err)
	}
	if !set[9] {
		t.Errorf("공유 캐시를 봐야 한다(위임이 끊겼을 수 있다). set=%v", set)
	}
}
