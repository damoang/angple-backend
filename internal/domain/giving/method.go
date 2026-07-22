package giving

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

// Giving draw methods. Stored in g5_giving_meta.method / g5_giving_draw.method.
const (
	// MethodLowestUnique 최저 유일 번호 (기존/복원): 아무도 안 고른 번호 중 최저 보유자 당첨.
	MethodLowestUnique = "lowest_unique"
	// MethodRandom 랜덤 추첨: 참가자 중 서버 RNG(commit-reveal)로 N명 선정.
	MethodRandom = "random"
	// MethodLadder 사다리타기: 서버가 시드로 결과 확정, 애니는 재생만.
	MethodLadder = "ladder"
	// MethodCuration 댓글 큐레이션: 주최자가 사연/정성을 보고 지정 (사유 공개 필수).
	MethodCuration = "curation"
	// MethodHostPick 자유 직접지명: 주최자 재량 100%, 사유 선택.
	MethodHostPick = "host_pick"
)

// DefaultMethod 미지정/레거시 글 폴백.
const DefaultMethod = MethodLowestUnique

// maxParsedNumbers 한 번의 파싱에서 허용하는 최대 번호 개수 (레거시 100000 상한 이식).
const maxParsedNumbers = 100000

// IsValidMethod reports whether m is a recognized giving method.
func IsValidMethod(m string) bool {
	switch m {
	case MethodLowestUnique, MethodRandom, MethodLadder, MethodCuration, MethodHostPick:
		return true
	}
	return false
}

// NormalizeMethod returns m if valid, else the default method.
func NormalizeMethod(m string) string {
	if IsValidMethod(m) {
		return m
	}
	return DefaultMethod
}

// IsPaid reports whether the method consumes points on entry (only lowest_unique).
func IsPaid(m string) bool { return m == MethodLowestUnique }

// IsHostDesignated reports whether the winner is chosen by the host rather than computed.
func IsHostDesignated(m string) bool { return m == MethodCuration || m == MethodHostPick }

// RequiresReason reports whether host designation must include a public reason.
func RequiresReason(m string) bool { return m == MethodCuration }

// IsAutoDraw reports whether the draw can run without host winner input.
func IsAutoDraw(m string) bool {
	return m == MethodLowestUnique || m == MethodRandom || m == MethodLadder
}

// ParseBidNumbers parses a CSV bid string like "1,3,5-10,15~20" into a sorted,
// deduplicated slice of positive integers. Ported from the legacy PHP
// parseAndValidateBidNumbers: comma separated, "-"/"~" ranges, all-zero and
// decimal tokens skipped. Total count is capped at maxParsedNumbers.
func ParseBidNumbers(s string) []int { //nolint:gocyclo // CSV/범위 파싱 응집 — 분해 시 상한/중복 경계 위험
	seen := make(map[int]struct{})
	add := func(n int) bool {
		if n <= 0 {
			return true
		}
		if _, ok := seen[n]; ok {
			return true
		}
		if len(seen) >= maxParsedNumbers {
			return false
		}
		seen[n] = struct{}{}
		return true
	}

	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// 0으로만 구성된 번호나 소숫점 포함 토큰은 무시
		if strings.Contains(part, ".") || isAllZero(part) {
			continue
		}
		if strings.ContainsAny(part, "-~") {
			sep := strings.IndexAny(part, "-~")
			start := atoiSafe(strings.TrimSpace(part[:sep]))
			end := atoiSafe(strings.TrimSpace(part[sep+1:]))
			if start > 0 && end > 0 && start <= end {
				for i := start; i <= end; i++ {
					if !add(i) {
						break
					}
				}
			}
			continue
		}
		if !add(atoiSafe(part)) {
			break
		}
	}

	out := make([]int, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

func isAllZero(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != '0' {
			return false
		}
	}
	return true
}

// atoiSafe parses leading integer semantics like PHP (int) cast, returning 0 on failure.
func atoiSafe(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// FormatNumbers renders a sorted int slice back to a compact CSV (no ranges).
func FormatNumbers(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

// LowestUniqueWinner determines the winner of a lowest-unique-number draw.
// bidsByNumber maps a number to the list of member IDs that bid on it. A number
// is "unique" when exactly one member chose it; the winner holds the smallest
// unique number. Returns the winning number, member ID, and whether a winner
// exists (all-duplicate boards have no winner).
func LowestUniqueWinner(bidsByNumber map[int][]string) (int, string, bool) {
	uniques := make([]int, 0)
	for num, bidders := range bidsByNumber {
		if len(bidders) == 1 {
			uniques = append(uniques, num)
		}
	}
	if len(uniques) == 0 {
		return 0, "", false
	}
	sort.Ints(uniques)
	win := uniques[0]
	return win, bidsByNumber[win][0], true
}

// DeriveSeed produces a commit-reveal seed deterministically from a server
// secret and the post identity. Because it is deterministic, its hash can be
// committed at setup time (g5_giving_meta.seed_hash) and the seed revealed at
// draw time; HMAC preimage resistance keeps the seed unpredictable to users
// (who do not hold the secret) while remaining reproducible for verification.
func DeriveSeed(secret, boTable string, wrID int) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(boTable + ":" + strconv.Itoa(wrID)))
	return hex.EncodeToString(mac.Sum(nil))
}

// SeedHash returns the hex SHA-256 of a seed. Anyone can check that
// SeedHash(revealed_seed) == committed seed_hash.
func SeedHash(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

// InputHash returns the hex SHA-256 of a participant/bid snapshot, sorted for
// determinism, so the draw input can be audited after the fact.
func InputHash(items []string) string {
	cp := append([]string(nil), items...)
	sort.Strings(cp)
	sum := sha256.Sum256([]byte(strings.Join(cp, ",")))
	return hex.EncodeToString(sum[:])
}

// prngUint64 returns a deterministic 64-bit value from seed and a counter.
// Mirrored on the frontend (random-select.ts / ladder.ts) so clients can
// reproduce and verify draw outcomes.
func prngUint64(seed string, counter int) uint64 {
	h := sha256.Sum256([]byte(seed + ":" + strconv.Itoa(counter)))
	return binary.BigEndian.Uint64(h[:8])
}

// deterministicShuffle applies a seeded Fisher-Yates shuffle in place.
func deterministicShuffle(seed string, items []string) {
	for i := len(items) - 1; i > 0; i-- {
		//nolint:gosec // G115: 모듈로 (i+1) 로 범위 제한, 오버플로 불가
		j := int(prngUint64(seed, i) % uint64(i+1)) // #nosec G115 -- 모듈로 (i+1) 로 0<=j<=i 보장
		items[i], items[j] = items[j], items[i]
	}
}

// RandomWinners deterministically selects up to n winners from participants
// using a seeded shuffle. participants must be pre-sorted by the caller for a
// stable, verifiable input ordering.
func RandomWinners(seed string, participants []string, n int) []string {
	if n <= 0 {
		n = 1
	}
	pool := append([]string(nil), participants...)
	deterministicShuffle(seed, pool)
	if n > len(pool) {
		n = len(pool)
	}
	winners := append([]string(nil), pool[:n]...)
	sort.Strings(winners)
	return winners
}

// LadderResult is the authoritative outcome of a ladder draw plus the rung
// layout needed to replay the animation on the client.
type LadderResult struct {
	Columns  int      `json:"columns"`
	Levels   int      `json:"levels"`
	Rungs    [][]bool `json:"rungs"`   // Rungs[level][gap]: connection between column gap and gap+1
	EndCol   []int    `json:"end_col"` // EndCol[startColumn] = destination column
	WinSlots int      `json:"win_slots"`
	Winners  []string `json:"winners"`
}

// BuildLadder generates a deterministic ladder from the seed, simulates each
// participant's descent (server authority), and marks those landing in the
// first winSlots destination columns as winners. The returned rung layout, when
// replayed by the client, reproduces exactly the same destinations.
func BuildLadder(seed string, participants []string, winSlots int) LadderResult { //nolint:gocyclo // 사다리 생성·강하 시뮬레이션 응집 — 분해 시 결정성 경계 위험
	cols := len(participants)
	if cols == 0 {
		return LadderResult{Winners: []string{}}
	}
	if winSlots <= 0 {
		winSlots = 1
	}
	if winSlots > cols {
		winSlots = cols
	}
	levels := cols * 2
	if levels < 8 {
		levels = 8
	}

	rungs := make([][]bool, levels)
	counter := 0
	for l := 0; l < levels; l++ {
		row := make([]bool, cols-1) // one slot per gap
		for gap := 0; gap < cols-1; gap++ {
			// No two adjacent rungs on the same level (standard ladder rule).
			if gap > 0 && row[gap-1] {
				continue
			}
			if prngUint64(seed, counter)%2 == 0 {
				row[gap] = true
			}
			counter++
		}
		rungs[l] = row
	}

	endCol := make([]int, cols)
	for start := 0; start < cols; start++ {
		c := start
		for l := 0; l < levels; l++ {
			row := rungs[l]
			if c < cols-1 && row[c] {
				c++
			} else if c > 0 && row[c-1] {
				c--
			}
		}
		endCol[start] = c
	}

	winners := make([]string, 0, winSlots)
	for start := 0; start < cols; start++ {
		if endCol[start] < winSlots {
			winners = append(winners, participants[start])
		}
	}
	sort.Strings(winners)

	return LadderResult{
		Columns:  cols,
		Levels:   levels,
		Rungs:    rungs,
		EndCol:   endCol,
		WinSlots: winSlots,
		Winners:  winners,
	}
}
