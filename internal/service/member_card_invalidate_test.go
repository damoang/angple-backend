package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/damoang/angple-backend/internal/webcache"
)

func recordMemberCardDeletes(t *testing.T) *[]string {
	t.Helper()
	t.Setenv("AUTH_CACHE_NAMESPACES", "")
	var keys []string
	restore := webcache.SetDeleterForTest(func(_ context.Context, ks ...string) error {
		keys = append(keys, ks...)
		return nil
	})
	t.Cleanup(restore)
	return &keys
}

var wantAbcdCardKeys = []string{"mbcard:abcd", "canary:mbcard:abcd"}

// 사진 저장 성공 뒤 회원 표시 캐시 키를 지운다.
func TestUploadMemberImage_InvalidatesMemberCard(t *testing.T) {
	deleted := recordMemberCardDeletes(t)
	store := newFakeMemberImageStore()
	store.existing["data/member_image/Ab/AbCd_1700000000.webp"] = 1
	svc := newTestMemberService(store, &fakeMemberRepo{})

	if _, err := svc.uploadMemberImage(context.Background(), "AbCd", "photo.png", testPNG); err != nil {
		t.Fatalf("업로드 실패: %v", err)
	}
	if !reflect.DeepEqual(*deleted, wantAbcdCardKeys) {
		t.Fatalf("deleted = %v, want %v", *deleted, wantAbcdCardKeys)
	}
}

// DB 갱신이 실패하면 캐시 키를 지우지 않는다.
func TestUploadMemberImage_DBFailureDoesNotInvalidate(t *testing.T) {
	deleted := recordMemberCardDeletes(t)
	store := newFakeMemberImageStore()
	store.existing["data/member_image/ab/abcd_1700000000.webp"] = 1
	svc := newTestMemberService(store, &fakeMemberRepo{err: errors.New("db down")})

	if _, err := svc.uploadMemberImage(context.Background(), "abcd", "a.jpg", testJPEG); err == nil {
		t.Fatal("DB 실패인데 에러가 없다")
	}
	if len(*deleted) != 0 {
		t.Fatalf("DB 실패인데 캐시 키를 지웠다: %v", *deleted)
	}
}

// 사진 삭제 성공 뒤 회원 표시 캐시 키를 지운다.
func TestDeleteMemberImage_InvalidatesMemberCard(t *testing.T) {
	deleted := recordMemberCardDeletes(t)
	svc := newTestMemberService(newFakeMemberImageStore(), &fakeMemberRepo{})

	if err := svc.DeleteMemberImage(context.Background(), "abcd"); err != nil {
		t.Fatalf("삭제 실패: %v", err)
	}
	if !reflect.DeepEqual(*deleted, wantAbcdCardKeys) {
		t.Fatalf("deleted = %v, want %v", *deleted, wantAbcdCardKeys)
	}
}

// 사진 삭제의 DB 갱신이 실패하면 캐시 키를 지우지 않는다.
func TestDeleteMemberImage_DBFailureDoesNotInvalidate(t *testing.T) {
	deleted := recordMemberCardDeletes(t)
	svc := newTestMemberService(newFakeMemberImageStore(), &fakeMemberRepo{err: errors.New("db down")})

	if err := svc.DeleteMemberImage(context.Background(), "abcd"); err == nil {
		t.Fatal("DB 실패인데 에러가 없다")
	}
	if len(*deleted) != 0 {
		t.Fatalf("DB 실패인데 캐시 키를 지웠다: %v", *deleted)
	}
}
