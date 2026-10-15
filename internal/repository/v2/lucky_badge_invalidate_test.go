package v2

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
)

// ⛔ 이 파일이 지키는 계약: 행운 지급이 실제로 쓰였을 때만(커밋 뒤) 그 글·댓글의 글 상세 배지 캐시 키를 지운다.
// 중복·상한으로 아무것도 쓰지 않았으면 지우지 않는다.

// delRecorder 는 DEL 된 키만 기록하는 LuckyBadgeStore 다.
type delRecorder struct {
	mu   sync.Mutex
	dels []string
}

func (d *delRecorder) Get(context.Context, string) ([]byte, error) { return nil, errors.New("miss") }
func (d *delRecorder) Set(context.Context, string, []byte, time.Duration) error {
	return nil
}
func (d *delRecorder) Del(_ context.Context, keys ...string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dels = append(d.dels, keys...)
	return nil
}

func (d *delRecorder) take() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.dels
	d.dels = nil
	return out
}

func TestGrant_InvalidatesBadgeCacheAfterCommit(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	addLuckyMember(t, db, "member_a")
	rec := &delRecorder{}
	restore := gnurepo.SetLuckyBadgeStoreForTest(rec)
	defer restore()

	now := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	out, err := r.GrantWithOptions("member_a", "free", "41", LuckyKindPost, 5, GrantOptions{Now: now})
	if err != nil || out != GrantOutcomeGranted {
		t.Fatalf("글 지급: out=%s err=%v", out, err)
	}
	if got := rec.take(); len(got) != 1 || got[0] != "lucky:badge:free:41" {
		t.Errorf("글 지급 뒤 그 글 키를 지워야 한다: %v", got)
	}

	out, err = r.GrantWithOptions("member_a", "free", "42", LuckyKindComment, 3, GrantOptions{Now: now})
	if err != nil || out != GrantOutcomeGranted {
		t.Fatalf("댓글 지급: out=%s err=%v", out, err)
	}
	if got := rec.take(); len(got) != 1 || got[0] != "lucky:badge:free:42" {
		t.Errorf("댓글 지급 뒤 그 댓글 키를 지워야 한다: %v", got)
	}

	// 같은 글 두 번째(중복) — 아무것도 쓰지 않았으니 지우지 않는다.
	_, _ = r.GrantWithOptions("member_a", "free", "41", LuckyKindPost, 5, GrantOptions{Now: now})
	if got := rec.take(); len(got) != 0 {
		t.Errorf("중복 지급은 키를 지우지 않는다: %v", got)
	}

	// 회원 하루 상한에 닿음 — 쓰지 않았으니 지우지 않는다.
	out, err = r.GrantWithOptions("member_a", "free", "43", LuckyKindPost, 5, GrantOptions{Now: now, MemberDailyCap: 1})
	if err != nil || out != GrantOutcomeCappedMember {
		t.Fatalf("상한: out=%s err=%v", out, err)
	}
	if got := rec.take(); len(got) != 0 {
		t.Errorf("상한으로 멈춘 지급은 키를 지우지 않는다: %v", got)
	}
}
