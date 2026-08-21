package main

import (
	"testing"
	"time"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ⛔ enrichWithDisciplineRelated 가 실패를 삼키면:
//
//	목록  — 마스킹 벗겨진 원문이 응답에 실리고, **캐시에 들어가 30초간 전원에게 재배포**된다
//	상세·댓글 — is_discipline_related 가 빠져 프런트 가림막(DisciplinedContent)이 안 그려진다
//
// 예전에는 반환 시그니처에 error 가 **아예 없어서** 호출부가 실패를 알 방법이 없었다.
// 이 테스트가 그 계약을 문다.
//
// ⚠️ 한계 둘 — 테스트가 못 덮는 부분을 명시한다.
//
//	① 호출부의 「실패 시 캐시에 쓰지 않는다」(disciplineOK) 배선과 상세의 중립 자리표시자는
//	   거대한 핸들러 안에 있어 여기서 못 돈다. 이 파일은 **error 가 올라온다**는 것까지만 보장한다.
//	② 2단 폴백(보드 전체 실패 → 요청된 id 만 조회)이 **성공하는** 경로도 못 만든다.
//	   두 쿼리가 같은 테이블(g5_na_singo)을 보므로 sqlite 에서는 둘 다 실패한다.
//	   여기 테스트가 덮는 것은 **둘 다 실패했을 때 error 가 나가는지**다.
//
// 둘 다 코드 리뷰가 맡아야 한다.

func newEnrichTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// ⭐ sqlite 에는 g5_na_singo 가 없어 조회가 반드시 실패한다. 검증 수단이다.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	return db
}

func TestEnrichWithDisciplineRelated_PropagatesError(t *testing.T) {
	gnurepo.ResetDisciplinedIDsCacheForTest()

	items := []map[string]any{{"id": 1, "title": "원제목-노출되면-안-됨"}}

	got, err := enrichWithDisciplineRelated(newEnrichTestDB(t), "free", items, true)

	// ⛔ 여기가 핵심이다. err == nil 이면 호출부가 정상으로 알고 **캐시에 쓴다.**
	if err == nil {
		t.Fatalf("error 를 기대했다. got=%v — 호출부가 캐시에 쓰게 되는 조합이다", got)
	}
	if got != nil {
		t.Errorf("실패 시에는 아이템을 돌려주면 안 된다(호출부가 원본을 유지하도록). got=%v", got)
	}
}

// TestEnrichWithDisciplineRelated_UsesStaleCache 는 만료 캐시가 있으면
// 조회가 실패해도 마스킹이 유지되는지 본다. 실제 실패의 대부분이 이 경로로 흡수된다.
func TestEnrichWithDisciplineRelated_UsesStaleCache(t *testing.T) {
	gnurepo.SeedDisciplinedIDsCacheForTest("free", map[int]bool{1: true}, time.Now().Add(-time.Minute))

	items := []map[string]any{
		{"id": 1, "title": "근거글-원제목"},
		{"id": 2, "title": "무고한 글"},
	}

	got, err := enrichWithDisciplineRelated(newEnrichTestDB(t), "free", items, true)
	if err != nil {
		t.Fatalf("만료 캐시가 있으면 error 가 아니어야 한다: %v", err)
	}
	if got[0]["title"] != "[이용제한 근거 글]" {
		t.Errorf("근거글은 가려야 한다. got=%v", got[0]["title"])
	}
	if got[0]["is_discipline_related"] != true {
		t.Errorf("가림막 플래그가 붙어야 한다. got=%v", got[0]["is_discipline_related"])
	}
	// ⛔ 무고한 글에 라벨이 붙으면 허위 낙인이다.
	if got[1]["title"] != "무고한 글" {
		t.Errorf("무고한 글은 그대로여야 한다. got=%v", got[1]["title"])
	}
	if _, ok := got[1]["is_discipline_related"]; ok {
		t.Errorf("무고한 글에 플래그가 붙었다. got=%v", got[1])
	}
}

// TestEnrichWithDisciplineRelated_EmptyInput 는 「조회할 것 없음」과 「실패」를 구분한다.
func TestEnrichWithDisciplineRelated_EmptyInput(t *testing.T) {
	gnurepo.ResetDisciplinedIDsCacheForTest()
	db := newEnrichTestDB(t)

	for _, tc := range []struct {
		name  string
		slug  string
		items []map[string]any
	}{
		{"아이템 없음", "free", nil},
		{"보드 없음", "", []map[string]any{{"id": 1}}},
		{"유효한 id 없음", "free", []map[string]any{{"id": 0}, {"id": "문자열"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := enrichWithDisciplineRelated(db, tc.slug, tc.items, true); err != nil {
				t.Fatalf("조회할 것이 없으면 error 가 아니어야 한다: %v", err)
			}
		})
	}
}
