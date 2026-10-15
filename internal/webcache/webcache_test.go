package webcache

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestMemberCardCacheKeys_LowercaseAndNamespaces(t *testing.T) {
	t.Setenv("AUTH_CACHE_NAMESPACES", "")
	got := MemberCardCacheKeys("AbCd")
	want := []string{"mbcard:abcd", "canary:mbcard:abcd"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
}

func TestMemberCardCacheKeys_EnvAddsNamespaces(t *testing.T) {
	t.Setenv("AUTH_CACHE_NAMESPACES", " canary, blue ,,canary")
	got := MemberCardCacheKeys("User1")
	want := []string{"mbcard:user1", "canary:mbcard:user1", "blue:mbcard:user1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
}

func TestMemberCardCacheKeys_Empty(t *testing.T) {
	if got := MemberCardCacheKeys(""); got != nil {
		t.Fatalf("empty mb_id should give nil, got %v", got)
	}
}

func TestNamespacePrefixes_AlwaysIncludesBare(t *testing.T) {
	t.Setenv("AUTH_CACHE_NAMESPACES", "blue")
	got := NamespacePrefixes()
	want := []string{"", "blue:"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prefixes = %v, want %v", got, want)
	}
}

func TestInvalidateMemberCard_NoRedisIsNoop(t *testing.T) {
	restore := SetDeleterForTest(nil)
	defer restore()
	InvalidateMemberCard("abcd") // panic 나 블로킹 없이 돌아와야 한다
}

func TestInvalidateMemberCard_DeletesKeysWithDeadline(t *testing.T) {
	t.Setenv("AUTH_CACHE_NAMESPACES", "")
	var gotKeys []string
	var hadDeadline bool
	restore := SetDeleterForTest(func(ctx context.Context, keys ...string) error {
		_, hadDeadline = ctx.Deadline()
		gotKeys = append(gotKeys, keys...)
		return nil
	})
	defer restore()

	InvalidateMemberCard("AbCd")
	want := []string{"mbcard:abcd", "canary:mbcard:abcd"}
	if !reflect.DeepEqual(gotKeys, want) {
		t.Fatalf("deleted = %v, want %v", gotKeys, want)
	}
	if !hadDeadline {
		t.Error("DEL 은 타임아웃이 걸린 context 로 불려야 한다")
	}
}

func TestInvalidateMemberCard_ErrorIsSwallowed(t *testing.T) {
	calls := 0
	restore := SetDeleterForTest(func(context.Context, ...string) error {
		calls++
		return errors.New("redis down")
	})
	defer restore()

	InvalidateMemberCard("abcd")
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestInvalidateMemberCard_EmptyIDSkips(t *testing.T) {
	calls := 0
	restore := SetDeleterForTest(func(context.Context, ...string) error {
		calls++
		return nil
	})
	defer restore()

	InvalidateMemberCard("")
	if calls != 0 {
		t.Fatalf("empty mb_id should not call DEL, calls = %d", calls)
	}
}
