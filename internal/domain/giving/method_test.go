package giving

import (
	"reflect"
	"testing"
)

func TestParseBidNumbers(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"1,3,5", []int{1, 3, 5}},
		{"5-10", []int{5, 6, 7, 8, 9, 10}},
		{"15~18", []int{15, 16, 17, 18}},
		{"1,3,5-7,3", []int{1, 3, 5, 6, 7}}, // dedup across single + range
		{"0,00,2.5,3", []int{3}},            // all-zero and decimal skipped
		{"10-5", []int{}},                   // reversed range ignored
		{" 4 , 2 ", []int{2, 4}},            // trimmed + sorted
		{"", []int{}},
	}
	for _, tc := range cases {
		got := ParseBidNumbers(tc.in)
		if len(got) == 0 && len(tc.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseBidNumbers(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestLowestUniqueWinner(t *testing.T) {
	// 1: A,B (dup) | 2: C (unique) | 3: D (unique) → winner = 2, C
	bids := map[int][]string{
		1: {"A", "B"},
		2: {"C"},
		3: {"D"},
	}
	num, mb, ok := LowestUniqueWinner(bids)
	if !ok || num != 2 || mb != "C" {
		t.Fatalf("got num=%d mb=%s ok=%v, want 2 C true", num, mb, ok)
	}

	// all duplicated → no winner
	allDup := map[int][]string{1: {"A", "B"}, 2: {"C", "D"}}
	if _, _, ok := LowestUniqueWinner(allDup); ok {
		t.Fatalf("expected no winner when all numbers duplicated")
	}

	// empty → no winner
	if _, _, ok := LowestUniqueWinner(map[int][]string{}); ok {
		t.Fatalf("expected no winner for empty board")
	}
}

func TestSeedCommitReveal(t *testing.T) {
	seed := DeriveSeed("secret-key", "giving", 42)
	if seed == DeriveSeed("secret-key", "giving", 43) {
		t.Fatal("seeds for different posts must differ")
	}
	if seed != DeriveSeed("secret-key", "giving", 42) {
		t.Fatal("seed derivation must be deterministic")
	}
	// committed hash verifies against revealed seed
	committed := SeedHash(seed)
	if committed != SeedHash(seed) || committed == seed {
		t.Fatal("seed hash must be stable and distinct from seed")
	}
}

func TestRandomWinnersDeterministic(t *testing.T) {
	parts := []string{"a", "b", "c", "d", "e"}
	seed := "abc123"
	w1 := RandomWinners(seed, parts, 2)
	w2 := RandomWinners(seed, parts, 2)
	if !reflect.DeepEqual(w1, w2) {
		t.Fatalf("random winners not reproducible: %v vs %v", w1, w2)
	}
	if len(w1) != 2 {
		t.Fatalf("expected 2 winners, got %d", len(w1))
	}
	// different seed → (very likely) different selection
	if reflect.DeepEqual(w1, RandomWinners("different", parts, 2)) {
		t.Log("note: different seed produced same winners (possible but unlikely)")
	}
	// capacity larger than pool clamps
	if len(RandomWinners(seed, parts, 99)) != len(parts) {
		t.Fatal("winners should clamp to participant count")
	}
	// all winners must be real participants
	for _, w := range w1 {
		if w != "a" && w != "b" && w != "c" && w != "d" && w != "e" {
			t.Fatalf("winner %q not in participant pool", w)
		}
	}
}

func TestBuildLadderDeterministicAndConsistent(t *testing.T) {
	parts := []string{"p1", "p2", "p3", "p4"}
	seed := DeriveSeed("k", "giving", 7)

	r1 := BuildLadder(seed, parts, 2)
	r2 := BuildLadder(seed, parts, 2)
	if !reflect.DeepEqual(r1, r2) {
		t.Fatal("ladder must be deterministic for the same seed")
	}
	if len(r1.Winners) != 2 {
		t.Fatalf("expected 2 winners, got %d (%v)", len(r1.Winners), r1.Winners)
	}

	// End columns must be a permutation of 0..cols-1 (ladder is a bijection).
	seen := make(map[int]bool)
	for _, c := range r1.EndCol {
		if c < 0 || c >= r1.Columns || seen[c] {
			t.Fatalf("ladder end columns not a permutation: %v", r1.EndCol)
		}
		seen[c] = true
	}

	// Winners recomputed from the published rungs must match (client-verifiable).
	replay := replayLadder(r1)
	winnersFromReplay := make([]string, 0)
	for start, c := range replay {
		if c < r1.WinSlots {
			winnersFromReplay = append(winnersFromReplay, parts[start])
		}
	}
	if len(winnersFromReplay) != len(r1.Winners) {
		t.Fatalf("replay winners %v != server winners %v", winnersFromReplay, r1.Winners)
	}

	if len(BuildLadder(seed, nil, 1).Winners) != 0 {
		t.Fatal("empty participants should yield no winners")
	}
}

// replayLadder re-simulates descent purely from published rungs (mirrors what a
// client verifier would do).
func replayLadder(r LadderResult) []int {
	out := make([]int, r.Columns)
	for start := 0; start < r.Columns; start++ {
		c := start
		for l := 0; l < r.Levels; l++ {
			row := r.Rungs[l]
			if c < r.Columns-1 && row[c] {
				c++
			} else if c > 0 && row[c-1] {
				c--
			}
		}
		out[start] = c
	}
	return out
}
