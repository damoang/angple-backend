package service

import (
	"errors"
	"testing"

	"github.com/damoang/angple-backend/internal/domain"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"github.com/damoang/angple-backend/internal/repository"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPostRatingTest(t *testing.T) *PostRatingService {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := db.AutoMigrate(&domain.AnglePostRating{}, &v2domain.V2BoardExtendedSettings{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	settingsRepo := v2repo.NewBoardExtendedSettingsRepository(db)
	if err := settingsRepo.Upsert(&v2domain.V2BoardExtendedSettings{
		BoardID:  "angtt",
		Settings: `{"features":{"rating":true}}`,
	}); err != nil {
		t.Fatalf("seed angtt settings: %v", err)
	}
	// rating 토글이 꺼진 보드 (features 는 있으나 rating=false)
	if err := settingsRepo.Upsert(&v2domain.V2BoardExtendedSettings{
		BoardID:  "free",
		Settings: `{"features":{"rating":false}}`,
	}); err != nil {
		t.Fatalf("seed free settings: %v", err)
	}

	return NewPostRatingService(repository.NewPostRatingRepository(db), settingsRepo)
}

func TestPostRatingUpsertAndRevote(t *testing.T) {
	svc := setupPostRatingTest(t)

	summary, err := svc.Rate("angtt", 100, "member1", 3, 4)
	if err != nil {
		t.Fatalf("first vote: %v", err)
	}
	if summary.Avg != 4.0 || summary.Count != 1 || summary.My != 4 {
		t.Errorf("first vote: got avg=%v count=%d my=%d, want avg=4 count=1 my=4",
			summary.Avg, summary.Count, summary.My)
	}

	// 재투표 = UPDATE (count 는 그대로, rating 만 갱신)
	summary, err = svc.Rate("angtt", 100, "member1", 3, 5)
	if err != nil {
		t.Fatalf("revote: %v", err)
	}
	if summary.Avg != 5.0 || summary.Count != 1 || summary.My != 5 {
		t.Errorf("revote: got avg=%v count=%d my=%d, want avg=5 count=1 my=5",
			summary.Avg, summary.Count, summary.My)
	}
}

func TestPostRatingRangeValidation(t *testing.T) {
	svc := setupPostRatingTest(t)

	for _, invalid := range []int{0, 6, -1, 100} {
		if _, err := svc.Rate("angtt", 100, "member1", 3, invalid); !errors.Is(err, ErrRatingOutOfRange) {
			t.Errorf("rating=%d: got err=%v, want ErrRatingOutOfRange", invalid, err)
		}
	}
}

func TestPostRatingLevelGuard(t *testing.T) {
	svc := setupPostRatingTest(t)

	// mb_level < 3 → 거부
	if _, err := svc.Rate("angtt", 100, "newbie", 2, 4); !errors.Is(err, ErrRatingLevelTooLow) {
		t.Errorf("level 2: got err=%v, want ErrRatingLevelTooLow", err)
	}
	// mb_level 3 (앙님) → 허용
	if _, err := svc.Rate("angtt", 100, "angnim", 3, 4); err != nil {
		t.Errorf("level 3: unexpected err=%v", err)
	}
}

func TestPostRatingDisabledBoard(t *testing.T) {
	svc := setupPostRatingTest(t)

	// features.rating=false 보드
	if _, err := svc.Rate("free", 100, "member1", 3, 4); !errors.Is(err, ErrRatingDisabled) {
		t.Errorf("free board: got err=%v, want ErrRatingDisabled", err)
	}
	// extended settings 자체가 없는 보드
	if _, err := svc.Rate("unknown", 100, "member1", 3, 4); !errors.Is(err, ErrRatingDisabled) {
		t.Errorf("unknown board: got err=%v, want ErrRatingDisabled", err)
	}
	if svc.Enabled("angtt") != true {
		t.Error("angtt should be rating-enabled")
	}
	if svc.Enabled("free") != false {
		t.Error("free should be rating-disabled")
	}
}

func TestPostRatingAggregate(t *testing.T) {
	svc := setupPostRatingTest(t)

	// 2명이 4, 5점 → avg 4.5, count 2
	if _, err := svc.Rate("angtt", 200, "member1", 3, 4); err != nil {
		t.Fatalf("member1 vote: %v", err)
	}
	summary, err := svc.Rate("angtt", 200, "member2", 3, 5)
	if err != nil {
		t.Fatalf("member2 vote: %v", err)
	}
	if summary.Avg != 4.5 || summary.Count != 2 || summary.My != 5 {
		t.Errorf("got avg=%v count=%d my=%d, want avg=4.5 count=2 my=5",
			summary.Avg, summary.Count, summary.My)
	}

	// 비로그인 조회: my=0
	guest, err := svc.Summary("angtt", 200, "")
	if err != nil {
		t.Fatalf("guest summary: %v", err)
	}
	if guest.Avg != 4.5 || guest.Count != 2 || guest.My != 0 {
		t.Errorf("guest: got avg=%v count=%d my=%d, want avg=4.5 count=2 my=0",
			guest.Avg, guest.Count, guest.My)
	}
}

func TestPostRatingAvgRounding(t *testing.T) {
	svc := setupPostRatingTest(t)

	// 4, 4, 5 → 13/3 = 4.333... → 소수 1자리 반올림 = 4.3
	for i, r := range map[string]int{"m1": 4, "m2": 4, "m3": 5} {
		if _, err := svc.Rate("angtt", 300, i, 3, r); err != nil {
			t.Fatalf("vote %s: %v", i, err)
		}
	}
	summary, err := svc.Summary("angtt", 300, "")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Avg != 4.3 || summary.Count != 3 {
		t.Errorf("got avg=%v count=%d, want avg=4.3 count=3", summary.Avg, summary.Count)
	}
}
