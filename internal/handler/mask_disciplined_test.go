package handler

import (
	"errors"
	"testing"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
)

// ⛔ 노출을 실제로 막는 코드는 저장소가 아니라 **여기**다.
// 「판별 실패 보드의 항목을 목록에서 뺀다」는 결정이 전부 maskDisciplined 안에 있다.
// 저장소 계약만 테스트하면 누가 `continue` 를 `disciplined[board] = nil` 로 바꿔도
// CI 가 못 잡는다.
//
// 지키는 것 넷:
//
//	① 실패 보드의 **원제목이 응답에 남지 않는다**  (이 수정의 존재 이유)
//	② 실패 보드의 **무고한 글에 라벨이 붙지 않는다** (과잉 마스킹 = 허위 낙인)
//	③ 다른 보드는 그대로 남는다
//	④ 정상 경로는 기존과 동일하다 (무회귀)

// fakeMyPageRepo 는 FindDisciplinedIDs 만 흉내 낸다.
// 나머지는 인터페이스를 임베드해 비워 둔다 — 호출되면 panic 이 나고, 그건
// "테스트가 의도보다 넓은 경로를 탔다"는 신호라 오히려 알아야 할 정보다.
type fakeMyPageRepo struct {
	gnurepo.MyPageRepository
	byBoard map[string]map[int]bool
	failed  map[string]bool
}

func (f *fakeMyPageRepo) FindDisciplinedIDs(boardID string, _ []int) (map[int]bool, error) {
	if f.failed[boardID] {
		return nil, errors.New("db down")
	}
	if set, ok := f.byBoard[boardID]; ok {
		return set, nil
	}
	return map[int]bool{}, nil
}

// 핸들러가 쓰는 마스킹 라벨. 테스트에서 여러 번 비교하므로 상수로 둔다.
const disciplinedLabel = "[이용제한 근거 글]"

func item(board string, wrID int, subject string) map[string]interface{} {
	return map[string]interface{}{"bo_table": board, "wr_id": wrID, "wr_subject": subject}
}

func subjects(items []map[string]interface{}) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		s, _ := it["wr_subject"].(string)
		out = append(out, s)
	}
	return out
}

func TestMaskDisciplined_NormalPath(t *testing.T) {
	h := &MyPageHandler{myPageRepo: &fakeMyPageRepo{
		byBoard: map[string]map[int]bool{"free": {10: true}},
	}}

	got := h.maskDisciplined([]map[string]interface{}{
		item("free", 10, "근거글-원제목"),
		item("free", 11, "평범한 글"),
	}, "wr_subject", disciplinedLabel)

	if len(got) != 2 {
		t.Fatalf("정상 경로에서는 항목이 빠지면 안 된다. got=%d", len(got))
	}
	if got[0]["wr_subject"] != disciplinedLabel {
		t.Errorf("근거글은 가려야 한다. got=%v", got[0]["wr_subject"])
	}
	if got[1]["wr_subject"] != "평범한 글" {
		t.Errorf("무고한 글은 그대로여야 한다. got=%v", got[1]["wr_subject"])
	}
}

func TestMaskDisciplined_FailedBoardIsDropped(t *testing.T) {
	h := &MyPageHandler{myPageRepo: &fakeMyPageRepo{
		failed:  map[string]bool{"free": true},
		byBoard: map[string]map[int]bool{"humor": {20: true}},
	}}

	got := h.maskDisciplined([]map[string]interface{}{
		item("free", 10, "근거글-원제목"),
		item("free", 11, "무고한 글"),
		item("humor", 20, "다른보드 근거글"),
		item("humor", 21, "다른보드 평범한 글"),
	}, "wr_subject", disciplinedLabel)

	for _, s := range subjects(got) {
		// ① 원제목이 남으면 안 된다
		if s == "근거글-원제목" {
			t.Errorf("⛔ 판별 실패 보드의 원제목이 응답에 남았다: %q", s)
		}
		// ② 무고한 글에 라벨이 붙으면 안 된다 (허위 낙인)
		if s == "무고한 글" {
			t.Errorf("⛔ 판별 실패 보드의 항목이 남았다(라벨 여부와 무관하게 빠져야 한다): %q", s)
		}
	}

	// ③ 다른 보드는 그대로
	if len(got) != 2 {
		t.Fatalf("humor 두 건만 남아야 한다. got=%d (%v)", len(got), subjects(got))
	}
	if got[0]["wr_subject"] != disciplinedLabel {
		t.Errorf("정상 보드의 근거글은 가려야 한다. got=%v", got[0]["wr_subject"])
	}
	if got[1]["wr_subject"] != "다른보드 평범한 글" {
		t.Errorf("정상 보드의 무고한 글은 그대로여야 한다. got=%v", got[1]["wr_subject"])
	}
}

// TestMaskDisciplined_NoFalseStigma 는 판별 실패 시 **라벨을 붙이지 않는지**를
// 따로 못 박는다. 「전부 가린다」로 구현이 바뀌면 이 테스트가 깨진다.
func TestMaskDisciplined_NoFalseStigma(t *testing.T) {
	h := &MyPageHandler{myPageRepo: &fakeMyPageRepo{failed: map[string]bool{"free": true}}}

	got := h.maskDisciplined([]map[string]interface{}{
		item("free", 11, "무고한 글"),
	}, "wr_subject", disciplinedLabel)

	for _, s := range subjects(got) {
		if s == disciplinedLabel {
			t.Error("⛔ 판별 못 한 글에 「이용제한 근거 글」을 붙였다 — 허위 낙인이다")
		}
	}
}

func TestMaskDisciplined_EmptyInput(t *testing.T) {
	h := &MyPageHandler{myPageRepo: &fakeMyPageRepo{}}
	if got := h.maskDisciplined(nil, "wr_subject", "x"); got != nil {
		t.Errorf("빈 입력은 그대로 돌려줘야 한다. got=%v", got)
	}
}
