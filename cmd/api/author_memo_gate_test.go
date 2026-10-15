package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// enrichWithAuthorMemo 의 메모 대상 게이트 계약:
//
//   - 집합을 알고 작성자와 겹치지 않으면 DB 조회 없이 모든 작성자에 author_memo: nil
//   - 겹치면 겹친 작성자만 조회한다
//   - 집합을 모르면(ok=false) 페이지 작성자 전체로 조회한다(fail-open)
//   - 게이트에는 보는 사람 ID 가 그대로 넘어간다(키는 보는 사람별)

func stubAuthorMemoTargetSet(t *testing.T, fn func(context.Context, *gorm.DB, string) (map[string]struct{}, bool)) {
	t.Helper()
	prev := authorMemoTargetSet
	authorMemoTargetSet = fn
	t.Cleanup(func() { authorMemoTargetSet = prev })
}

// newAuthorMemoGateNoTableDB 는 g5_member_memo 테이블이 없는 DB 다 — 조회하면 오류가 난다.
func newAuthorMemoGateNoTableDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func assertAuthorMemoNil(t *testing.T, item map[string]any, label string) {
	t.Helper()
	v, ok := item["author_memo"]
	if !ok {
		t.Fatalf("%s: author_memo key must be present", label)
	}
	if m, isH := v.(gin.H); !isH || m != nil {
		t.Fatalf("%s: author_memo must be nil gin.H, got %#v", label, v)
	}
}

func TestEnrichWithAuthorMemo_GateEmptySetSkipsDB(t *testing.T) {
	db := newAuthorMemoGateNoTableDB(t)
	var gotViewer string
	stubAuthorMemoTargetSet(t, func(_ context.Context, _ *gorm.DB, viewer string) (map[string]struct{}, bool) {
		gotViewer = viewer
		return map[string]struct{}{}, true
	})
	stubAuthorMemoWithdrawnIDs(t, func(context.Context, []string) (map[string]bool, error) {
		t.Fatal("withdrawn lookup must not run without memos")
		return nil, nil
	})

	out := enrichWithAuthorMemo(db, "me", authorMemoItems())
	if gotViewer != "me" {
		t.Fatalf("gate viewer = %q, want me", gotViewer)
	}
	if len(out) != 3 {
		t.Fatalf("len = %d", len(out))
	}
	for _, item := range out {
		assertAuthorMemoNil(t, item, "item")
	}
}

func TestEnrichWithAuthorMemo_GateDisjointSetSkipsDB(t *testing.T) {
	db := newAuthorMemoGateNoTableDB(t)
	stubAuthorMemoTargetSet(t, func(context.Context, *gorm.DB, string) (map[string]struct{}, bool) {
		return map[string]struct{}{"someone-else": {}}, true
	})

	out := enrichWithAuthorMemo(db, "me", authorMemoItems())
	for _, item := range out {
		assertAuthorMemoNil(t, item, "item")
	}
}

func TestEnrichWithAuthorMemo_GateQueriesIntersectionOnly(t *testing.T) {
	db := newAuthorMemoTestDB(t) // me → left, normal 메모
	stubAuthorMemoTargetSet(t, func(context.Context, *gorm.DB, string) (map[string]struct{}, bool) {
		return map[string]struct{}{"normal": {}}, true
	})
	var checked []string
	stubAuthorMemoWithdrawnIDs(t, func(_ context.Context, ids []string) (map[string]bool, error) {
		checked = append(checked, ids...)
		return map[string]bool{}, nil
	})

	out := enrichWithAuthorMemo(db, "me", authorMemoItems())
	// left 는 집합에 없으므로 조회하지 않는다 — DB 에 행이 있어도 nil.
	assertAuthorMemoNil(t, out[0], "left")
	assertAuthorMemoNil(t, out[2], "nomemo")
	m, ok := out[1]["author_memo"].(gin.H)
	if !ok || m["content"] != "memo" {
		t.Fatalf("normal author memo must be attached, got %#v", out[1]["author_memo"])
	}
	if !reflect.DeepEqual(checked, []string{"normal"}) {
		t.Fatalf("withdrawn check ids = %v, want [normal]", checked)
	}
}

func TestEnrichWithAuthorMemo_GateUnknownFallsBackToFullLookup(t *testing.T) {
	db := newAuthorMemoTestDB(t)
	stubAuthorMemoTargetSet(t, func(context.Context, *gorm.DB, string) (map[string]struct{}, bool) {
		return nil, false // Redis 오류·시간 초과·truncated 등
	})
	stubAuthorMemoWithdrawnIDs(t, func(context.Context, []string) (map[string]bool, error) {
		return map[string]bool{}, nil
	})

	out := enrichWithAuthorMemo(db, "me", authorMemoItems())
	for _, i := range []int{0, 1} {
		m, ok := out[i]["author_memo"].(gin.H)
		if !ok || m["content"] != "memo" {
			t.Fatalf("item %d: full lookup must attach memo, got %#v", i, out[i]["author_memo"])
		}
	}
	assertAuthorMemoNil(t, out[2], "nomemo")
}

func TestEnrichWithAuthorMemo_GateNotCalledForAnonymous(t *testing.T) {
	stubAuthorMemoTargetSet(t, func(context.Context, *gorm.DB, string) (map[string]struct{}, bool) {
		t.Fatal("gate must not run for anonymous viewer")
		return nil, false
	})
	items := authorMemoItems()
	out := enrichWithAuthorMemo(newAuthorMemoGateNoTableDB(t), "", items)
	for _, item := range out {
		if _, ok := item["author_memo"]; ok {
			t.Fatalf("anonymous viewer must get no author_memo key, got %#v", item)
		}
	}
}

func TestEnrichWithAuthorMemo_GateDoesNotMutateInput(t *testing.T) {
	stubAuthorMemoTargetSet(t, func(context.Context, *gorm.DB, string) (map[string]struct{}, bool) {
		return map[string]struct{}{}, true
	})
	items := authorMemoItems()
	_ = enrichWithAuthorMemo(newAuthorMemoGateNoTableDB(t), "me", items)
	for _, item := range items {
		if _, ok := item["author_memo"]; ok {
			t.Fatalf("shared input item must not be mutated, got %#v", item)
		}
	}
}

func TestIntersectMemoTargetIDs(t *testing.T) {
	got := intersectMemoTargetIDs([]string{"a", "b", "c"}, map[string]struct{}{"c": {}, "a": {}, "z": {}})
	if !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Fatalf("got %v", got)
	}
	if got := intersectMemoTargetIDs([]string{"a"}, nil); len(got) != 0 {
		t.Fatalf("nil set must yield empty, got %v", got)
	}
}
