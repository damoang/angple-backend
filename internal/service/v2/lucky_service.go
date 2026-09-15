package v2

import (
	"crypto/rand"
	"math/big"

	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// LuckyService holds the pure "나리야 럭키 포인트" roll logic. 지급/설정/DB 접근은 일절 없고
// 오직 확률 계산만 담당한다 — 훗날 플러그인으로 이관하기 쉽도록 부수효과를 뺐다.
type LuckyService interface {
	// RollLucky rolls a pair of dice. won = 두 눈이 같을 때(확률 1/PointDice).
	// 당첨 시 amount = 1..PointMax 균등난수, 미당첨 시 amount = 0.
	RollLucky(cfg v2repo.LuckyConfig) (won bool, amount int)
}

type luckyService struct{}

// NewLuckyService creates a new LuckyService
func NewLuckyService() LuckyService {
	return &luckyService{}
}

// RollLucky implements the pair-of-dice draw using crypto/rand.
func (s *luckyService) RollLucky(cfg v2repo.LuckyConfig) (bool, int) {
	dice := cfg.PointDice
	if dice < 1 {
		dice = 1
	}

	dice1 := cryptoRandN(dice) + 1 // 1..dice
	dice2 := cryptoRandN(dice) + 1 // 1..dice
	if dice1 != dice2 {
		return false, 0
	}

	max := cfg.PointMax
	if max < 1 {
		max = 1
	}
	amount := cryptoRandN(max) + 1 // 1..max
	return true, amount
}

// cryptoRandN returns a uniform integer in [0, n) using crypto/rand.
// n 은 호출부에서 >=1 로 보장한다. 난수 생성 실패(사실상 없음) 시 0 을 돌려준다.
func cryptoRandN(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}
