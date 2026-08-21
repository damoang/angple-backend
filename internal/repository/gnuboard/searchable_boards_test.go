package gnuboard

import (
	"errors"
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ⛔ 이 파일이 지키는 계약은 하나다:
//
//	GetSearchableBoards 와 searchableBoardSet 은 **절대 (nil, nil) 을 돌려주지 않는다.**
//
// 반환값이 「검색 가능한 게시판 = 허용 목록」이라, nil 은 호출부에서 「제한 없음」으로 읽힌다.
// 2026-08-21 에 이 실수를 한 파일에서 세 곳 찾았고, 그때마다 주석으로만 막았다.
// 주석은 다음 사람이 지운다. 테스트는 안 지워진다.
//
// 무엇이 걸려 있나 — member_activity_feed 151개 보드 중 검색 대상이 아닌 것이 29개다:
//
//	adm(읽기레벨 10) · advertiser(10) · temp(10) · archive(10)
//	disciplinelog(징계기록) · truthroom(신고누적) · claim(소명) · angreport(신고)
//
// 필터가 사라지면 이것들이 앱 피드와 회원 공개 프로필에 뜬다.

// fakeBoardRepo 는 FindAll 만 흉내 낸다.
// 나머지 메서드는 인터페이스를 임베드해 비워 둔다 — 테스트가 부르면 panic 이 나고,
// 그건 "테스트가 의도보다 넓은 경로를 탔다"는 신호라 오히려 알아야 할 정보다.
type fakeBoardRepo struct {
	BoardRepository
	boards []*gnuboard.G5Board
	err    error
}

func (f *fakeBoardRepo) FindAll() ([]*gnuboard.G5Board, error) { return f.boards, f.err }

// resetSearchableBoardsCache 는 패키지 전역 캐시를 비운다.
// ⛔ 이걸 안 하면 앞 테스트의 결과가 뒤 테스트로 새어 판정이 뒤집힌다.
func resetSearchableBoardsCache() {
	searchableBoardsCache.Lock()
	searchableBoardsCache.boards = nil
	searchableBoardsCache.expiresAt = time.Time{}
	searchableBoardsCache.Unlock()
}

// newTestRepo 는 sqlite in-memory 로 저장소를 만든다.
//
// ⭐ sqlite 에는 information_schema 가 없다. 그래서 GetSearchableBoards 의
// 테이블 실재 확인 쿼리가 **반드시 실패**한다 — 이 파일에서는 그게 버그가 아니라
// 검증 수단이다. 그 실패가 조용히 (nil, nil) 로 나가지 않는지를 본다.
func newTestRepo(t *testing.T, br BoardRepository) *myPageRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	return &myPageRepository{db: db, boardRepo: br}
}

func TestGetSearchableBoards_NeverReturnsNilNil(t *testing.T) {
	board := &gnuboard.G5Board{BoTable: "free", BoSubject: "자유게시판", BoUseSearch: 1}

	cases := []struct {
		name string
		repo *fakeBoardRepo
		why  string
	}{
		{
			name: "FindAll 이 에러",
			repo: &fakeBoardRepo{err: errors.New("db down")},
			why:  "조회 실패는 error 로 나가야 한다",
		},
		{
			name: "FindAll 이 빈 목록",
			repo: &fakeBoardRepo{boards: nil},
			why:  "게시판이 정상적으로 0개일 수는 없다 — 결과가 아니라 조회 이상이다",
		},
		{
			name: "테이블 실재 확인 쿼리가 실패 (information_schema 없음)",
			repo: &fakeBoardRepo{boards: []*gnuboard.G5Board{board}},
			why:  "예전에는 이 에러를 버려서 result 가 nil 이 되고 (nil, nil) 로 나갔다",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetSearchableBoardsCache()
			r := newTestRepo(t, tc.repo)

			got, err := r.GetSearchableBoards()

			if err == nil {
				t.Fatalf("error 를 기대했다 (%s). got=%v", tc.why, got)
			}
			if got != nil {
				t.Errorf("실패 시에는 목록을 주면 안 된다. got=%v", got)
			}
		})
	}
}

func TestSearchableBoardSet_PropagatesError(t *testing.T) {
	cases := []struct {
		name string
		repo *fakeBoardRepo
	}{
		{"FindAll 이 에러", &fakeBoardRepo{err: errors.New("db down")}},
		{"FindAll 이 빈 목록", &fakeBoardRepo{boards: nil}},
		{"테이블 실재 확인 실패", &fakeBoardRepo{
			boards: []*gnuboard.G5Board{{BoTable: "free", BoUseSearch: 1}},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetSearchableBoardsCache()
			r := newTestRepo(t, tc.repo)

			set, err := r.searchableBoardSet()

			// ⛔ 여기가 핵심이다. err == nil && set == nil 이면 호출부가
			//    `if set != nil && !set[b]` 같은 가드로 **필터를 통째로 건너뛴다.**
			if err == nil {
				t.Fatalf("error 를 기대했다. set=%v — 이 조합이 fail-open 을 만든다", set)
			}
			if set != nil {
				t.Errorf("실패 시에는 집합을 주면 안 된다. set=%v", set)
			}
		})
	}
}

// TestSearchableBoardsCache_DoesNotServeEmpty 는 실패 결과가 캐시에 남아
// 5분간 되풀이 서빙되지 않는지 본다.
func TestSearchableBoardsCache_DoesNotServeEmpty(t *testing.T) {
	resetSearchableBoardsCache()
	r := newTestRepo(t, &fakeBoardRepo{
		boards: []*gnuboard.G5Board{{BoTable: "free", BoUseSearch: 1}},
	})

	if _, err := r.GetSearchableBoards(); err == nil {
		t.Fatal("첫 호출에서 error 를 기대했다")
	}

	// 두 번째 호출도 같아야 한다. 빈 결과가 캐시됐다면 여기서 (nil, nil) 이 나온다.
	got, err := r.GetSearchableBoards()
	if err == nil {
		t.Fatalf("두 번째 호출도 error 여야 한다 — 빈 결과가 캐시된 것으로 보인다. got=%v", got)
	}
}
