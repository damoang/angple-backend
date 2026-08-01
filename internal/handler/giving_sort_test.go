package handler

import (
	"sort"
	"testing"
)

// 실장애 재현 케이스: 일정 없는 옛 글(GivingEnd="")이 urgent 정렬에서 진행중 글보다
// 앞에 와 위젯 limit 밖으로 진행중이 밀리던 문제 (2026-08-01).
func TestGivingUrgentSortActiveFirst(t *testing.T) {
	items := []GivingListItem{
		{ID: 2390, Title: "옛 (마감)", GivingStatus: "no_giving", GivingEnd: ""},
		{ID: 9002, Title: "진행중 B", GivingStatus: "active", GivingEnd: "2026-08-03 12:00:00"},
		{ID: 9001, Title: "진행중 A", GivingStatus: "active", GivingEnd: "2026-08-02 12:00:00"},
		{ID: 9100, Title: "대기", GivingStatus: "waiting", GivingStart: "2026-08-05 00:00:00"},
		{ID: 8000, Title: "일정미정 최근", GivingStatus: "no_giving", GivingEnd: ""},
	}
	sort.SliceStable(items, func(i, j int) bool { return givingListLess(items[i], items[j], "urgent") })

	got := []int{items[0].ID, items[1].ID, items[2].ID, items[3].ID, items[4].ID}
	want := []int{9001, 9002, 9100, 8000, 2390} // 진행중 임박순 → 대기 → 일정미정 최신순
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("urgent 순서 = %v, want %v", got, want)
		}
	}
}

func TestGivingNewestSort(t *testing.T) {
	items := []GivingListItem{
		{ID: 1, GivingStatus: "active"},
		{ID: 3, GivingStatus: "no_giving"},
		{ID: 2, GivingStatus: "ended"},
	}
	sort.SliceStable(items, func(i, j int) bool { return givingListLess(items[i], items[j], "newest") })
	if items[0].ID != 3 || items[2].ID != 1 {
		t.Fatalf("newest 는 상태 무관 ID 내림차순이어야: %+v", items)
	}
}

func TestGivingPausedRanksWithWaiting(t *testing.T) {
	if givingUrgentRank("paused") != 1 || givingUrgentRank("waiting") != 1 {
		t.Fatal("paused/waiting 은 같은 서열(1)")
	}
	if givingUrgentRank("active") != 0 || givingUrgentRank("ended") != 2 {
		t.Fatal("active=0, ended=2")
	}
}
