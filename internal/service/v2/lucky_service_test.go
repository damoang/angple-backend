package v2

import (
	"testing"

	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// TestRollLucky_WinRateAndRange rolls 1,000,000 times and asserts the win rate is
// close to 1/PointDice and every winning amount lands in [1, PointMax], losses = 0.
func TestRollLucky_WinRateAndRange(t *testing.T) {
	svc := NewLuckyService()
	cfg := v2repo.LuckyConfig{Enabled: true, PointDice: 20, PointMax: 500}

	const iterations = 1_000_000
	wins := 0
	for i := 0; i < iterations; i++ {
		won, amount := svc.RollLucky(cfg)
		if won {
			wins++
			if amount < 1 || amount > cfg.PointMax {
				t.Fatalf("winning amount out of range: got %d, want 1..%d", amount, cfg.PointMax)
			}
		} else if amount != 0 {
			t.Fatalf("losing roll must have amount 0, got %d", amount)
		}
	}

	rate := float64(wins) / float64(iterations)
	expected := 1.0 / float64(cfg.PointDice)
	// ±10% 상대허용치 (기대 0.05, 1e6 표본에서 표준편차 ~0.0002 → 매우 넉넉함).
	lower := expected * 0.9
	upper := expected * 1.1
	if rate < lower || rate > upper {
		t.Fatalf("win rate %.5f out of tolerance [%.5f, %.5f] (expected ~%.5f)", rate, lower, upper, expected)
	}
}

// TestRollLucky_DiceOne makes every roll a guaranteed win (1-sided dice) and checks
// the amount always falls within [1, PointMax].
func TestRollLucky_DiceOne(t *testing.T) {
	svc := NewLuckyService()
	cfg := v2repo.LuckyConfig{Enabled: true, PointDice: 1, PointMax: 10}

	for i := 0; i < 10000; i++ {
		won, amount := svc.RollLucky(cfg)
		if !won {
			t.Fatalf("PointDice=1 must always win")
		}
		if amount < 1 || amount > cfg.PointMax {
			t.Fatalf("amount out of range: got %d, want 1..%d", amount, cfg.PointMax)
		}
	}
}
