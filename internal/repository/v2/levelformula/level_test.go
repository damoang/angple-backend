package levelformula

import "testing"

// levelExp is a copy of the nariya-compatible formula from exp_repo.go
// Nariya formula: 1000 * (n-1)² (xp_base=1000, xp_rate=2)
func levelExp(level int) int {
	if level <= 1 {
		return 0
	}
	n := level - 1
	return 1000 * n * n
}

func calculateLevel(totalExp int) int {
	lo, hi := 1, 5000
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if levelExp(mid) <= totalExp {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// TestLevelExpNariyaCompatibility verifies levelExp matches nariya PHP formula
func TestLevelExpNariyaCompatibility(t *testing.T) {
	tests := []struct {
		level    int
		expected int
	}{
		{1, 0},
		{2, 1000},
		{3, 4000},
		{4, 9000},
		{5, 16000},
		{10, 81000},
		{20, 361000},
		{40, 1521000},
		{50, 2401000},
		{100, 9801000},
	}

	for _, tt := range tests {
		got := levelExp(tt.level)
		if got != tt.expected {
			t.Errorf("levelExp(%d) = %d, want %d", tt.level, got, tt.expected)
		}
	}
}

// TestCalculateLevelFromXP verifies level calculation from XP
func TestCalculateLevelFromXP(t *testing.T) {
	tests := []struct {
		totalExp      int
		expectedLevel int
	}{
		{0, 1},
		{500, 1},
		{999, 1},
		{1000, 2},
		{3999, 2},
		{4000, 3},
		{9000, 4},
		{81000, 10},
		{81001, 10},
		{99999, 10},
		{100000, 11},
		{361000, 20},
		{1521000, 40},
	}

	for _, tt := range tests {
		level := calculateLevel(tt.totalExp)
		if level != tt.expectedLevel {
			t.Errorf("calculateLevel(%d) = %d, want %d", tt.totalExp, level, tt.expectedLevel)
		}
	}
}
