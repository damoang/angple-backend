package stacksrv

import (
	"errors"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
)

// ErrInsufficientPoint 는 참가비를 낼 잔액이 없을 때다.
var ErrInsufficientPoint = errors.New("보유 포인트가 부족합니다")

// PlayerStats 는 규칙 하나의 전적이다.
type PlayerStats struct {
	// Rating 은 ELO 레이팅이다.
	Rating int `json:"rating" gorm:"column:rating"`
	// Wins 는 승 수다.
	Wins int `json:"wins" gorm:"column:wins"`
	// Losses 는 패 수다.
	Losses int `json:"losses" gorm:"column:losses"`
	// Draws 는 무승부 수다.
	Draws int `json:"draws" gorm:"column:draws"`
}

// NewGame 은 새 대전 행의 값이다.
type NewGame struct {
	// Rule 은 대전 규칙이다.
	Rule string
	// Mode 는 매칭 모드다.
	Mode string
	// P1 은 첫째 자리 회원 아이디다.
	P1 string
	// P2 는 둘째 자리 회원 아이디다.
	P2 string
	// EntryFee 는 1인 참가비다.
	EntryFee int
	// Seed 는 엔진 시드다.
	Seed uint32
	// RematchOf 는 재대결이면 직전 판 id 다(아니면 0).
	RematchOf int64
}

// GameResult 는 끝난 대전의 결과다. Winner 가 빈 문자열이면 무승부다.
type GameResult struct {
	// GameID 는 대전 행 id 다.
	GameID int64
	// Rule 은 대전 규칙이다.
	Rule string
	// P1 은 첫째 자리 회원 아이디다.
	P1 string
	// P2 는 둘째 자리 회원 아이디다.
	P2 string
	// Winner 는 이긴 회원 아이디다(무승부는 빈 문자열).
	Winner string
	// Reason 은 끝난 사유다.
	Reason string
	// P1Lines 는 첫째 자리가 지운 줄 합(서버 합산)이다.
	P1Lines int
	// P2Lines 는 둘째 자리가 지운 줄 합(서버 합산)이다.
	P2Lines int
	// DurationMs 는 go 부터 끝까지의 시간이다.
	DurationMs int64
}

// GameStore 는 대전 기록·참가비·전적 저장소다. *Store 가 구현하고 테스트는 가짜를 쓴다.
type GameStore interface {
	// CreateGame 은 대전 행을 만들고 id 를 돌려준다.
	CreateGame(g NewGame) (int64, error)
	// ChargeEntryFee 는 한 명의 참가비를 원자적으로 차감한다.
	ChargeEntryFee(gameID int64, mbID string, amount int) error
	// RefundEntryFee 는 실제로 차감된 참가비만 한 번 돌려준다.
	RefundEntryFee(gameID int64, mbID string, amount int) error
	// AbortGame 은 성립하지 못한 대전을 닫는다.
	AbortGame(gameID int64, reason string) error
	// FinishGame 은 결과와 규칙별 전적·레이팅을 남긴다. 멱등이어야 한다(이미 끝난 판이면 아무것도 하지 않는다).
	FinishGame(r GameResult) error
	// Stats 는 규칙별 전적이다. 기록이 없으면 ok=false.
	Stats(mbID, rule string) (PlayerStats, bool)
	// Balance 는 보유 포인트다.
	Balance(mbID string) int
}

// Store 는 MySQL(GORM) 구현이다.
//
// 포인트 차감은 반드시 이 트랜잭션 경로로만 한다. 잔액 확인을 락 없이 하고 별도 트랜잭션에서
// 차감하면 동시 요청에 이중지출이 난다(장기·나눔 응모와 같은 FOR UPDATE 패턴).
type Store struct {
	// db 는 연결이다.
	db *gorm.DB
}

// NewStore 는 저장소를 만든다.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// CreateGame 은 대전 행을 만들고 id 를 돌려준다.
func (s *Store) CreateGame(g NewGame) (int64, error) {
	var rematchOf interface{}
	if g.RematchOf > 0 {
		rematchOf = g.RematchOf
	}
	var id int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`INSERT INTO angple_stack_games (rule, mode, p1_mb_id, p2_mb_id, status, entry_fee, seed, rematch_of, started_at)
			 VALUES (?, ?, ?, ?, 'playing', ?, ?, ?, NOW())`,
			g.Rule, g.Mode, g.P1, g.P2, g.EntryFee, g.Seed, rematchOf,
		).Error; err != nil {
			return err
		}
		return tx.Raw("SELECT LAST_INSERT_ID()").Scan(&id).Error
	})
	return id, err
}

// ChargeEntryFee 는 한 명의 참가비를 원자적으로 차감한다.
// 한 트랜잭션 안에서 g5_member 행 배타 락과 잔액 확인, angple_stack_entries INSERT
// (UNIQUE(game_id, mb_id) 가 중복 차감의 최종 방어선), g5_point 선입선출 차감과 mb_point 반영을 한다.
func (s *Store) ChargeEntryFee(gameID int64, mbID string, amount int) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var balance int
		if err := tx.Raw("SELECT mb_point FROM g5_member WHERE mb_id = ? FOR UPDATE", mbID).Scan(&balance).Error; err != nil {
			return err
		}
		if balance < amount {
			return ErrInsufficientPoint
		}
		if err := tx.Exec(
			`INSERT INTO angple_stack_entries (game_id, mb_id, point_deducted, created_at) VALUES (?, ?, ?, NOW())`,
			gameID, mbID, amount,
		).Error; err != nil {
			return err
		}
		return deductPointTx(tx, mbID, amount, "앙쌓기 대전 참가비", "angple_stack_games", fmt.Sprint(gameID), "stack_entry")
	})
}

// RefundEntryFee 는 참가비를 되돌린다. 차감 기록이 있고 아직 환불하지 않았을 때만 한 번 돌려준다.
func (s *Store) RefundEntryFee(gameID int64, mbID string, amount int) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var cnt int64
		if err := tx.Raw(
			"SELECT COUNT(*) FROM angple_stack_entries WHERE game_id = ? AND mb_id = ? AND refunded_at IS NULL FOR UPDATE",
			gameID, mbID,
		).Scan(&cnt).Error; err != nil {
			return err
		}
		if cnt == 0 {
			return nil
		}
		if err := tx.Exec(
			"UPDATE angple_stack_entries SET refunded_at = NOW() WHERE game_id = ? AND mb_id = ? AND refunded_at IS NULL",
			gameID, mbID,
		).Error; err != nil {
			return err
		}
		return creditPointTx(tx, mbID, amount, "앙쌓기 대전 참가비 환불", "angple_stack_games", fmt.Sprint(gameID), "stack_entry_refund")
	})
}

// AbortGame 은 성립하지 못한 대전을 닫는다(진행 중인 행만).
func (s *Store) AbortGame(gameID int64, reason string) error {
	return s.db.Exec(
		"UPDATE angple_stack_games SET status = 'aborted', end_reason = ?, ended_at = NOW() WHERE id = ? AND status = 'playing'",
		reason, gameID,
	).Error
}

// AbortStalePlayingGames 는 기동 시 진행 중으로 남은 대전을 닫고 참가비를 돌려준다.
// 대전 상태는 메모리에만 있어 재시작하면 이어갈 수 없다.
func (s *Store) AbortStalePlayingGames() (int, error) {
	type entry struct {
		GameID int64  `gorm:"column:game_id"`
		MbID   string `gorm:"column:mb_id"`
		Point  int    `gorm:"column:point_deducted"`
	}
	var entries []entry
	if err := s.db.Raw(
		`SELECT e.game_id, e.mb_id, e.point_deducted
		   FROM angple_stack_entries e
		   JOIN angple_stack_games g ON g.id = e.game_id
		  WHERE g.status = 'playing' AND e.refunded_at IS NULL`,
	).Scan(&entries).Error; err != nil {
		return 0, err
	}
	refunded := 0
	for _, e := range entries {
		if err := s.RefundEntryFee(e.GameID, e.MbID, e.Point); err == nil {
			refunded++
		}
	}
	err := s.db.Exec(
		"UPDATE angple_stack_games SET status = 'aborted', end_reason = 'server_restart', ended_at = NOW() WHERE status = 'playing'",
	).Error
	return refunded, err
}

// FinishGame 은 결과를 남기고 (회원, 규칙)별 전적·레이팅을 갱신한다. 진행 중(playing)인 행일 때만
// 반영하므로 같은 결과로 여러 번 불러도 전적은 한 번만 더해진다.
// 장기와 같이 초대 대전도 전적·레이팅에 반영한다. 무승부는 레이팅을 바꾸지 않는다.
func (s *Store) FinishGame(r GameResult) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(
			`UPDATE angple_stack_games
			    SET status = 'finished', winner_mb_id = NULLIF(?, ''), end_reason = ?,
			        p1_lines = ?, p2_lines = ?, duration_ms = ?, ended_at = NOW()
			  WHERE id = ? AND status = 'playing'`,
			r.Winner, r.Reason, r.P1Lines, r.P2Lines, r.DurationMs, r.GameID,
		)
		if res.Error != nil {
			return res.Error
		}
		// 이미 끝난(또는 취소된) 판이면 전적을 다시 더하지 않는다. 커밋은 됐는데 응답이 오류로 온 뒤
		// persistFinish 가 다시 시도해도 승패·레이팅이 두 번 반영되지 않게 하는 멱등 장치다.
		if res.RowsAffected == 0 {
			return nil
		}
		if r.Winner == "" {
			if err := upsertStat(tx, r.P1, r.Rule, 0, 0, 1, 0); err != nil {
				return err
			}
			return upsertStat(tx, r.P2, r.Rule, 0, 0, 1, 0)
		}
		loser := r.P1
		if r.Winner == r.P1 {
			loser = r.P2
		}
		wDelta, lDelta := eloDelta(ratingOf(tx, r.Winner, r.Rule), ratingOf(tx, loser, r.Rule))
		if err := upsertStat(tx, r.Winner, r.Rule, 1, 0, 0, wDelta); err != nil {
			return err
		}
		return upsertStat(tx, loser, r.Rule, 0, 1, 0, lDelta)
	})
}

// Stats 는 규칙별 전적이다. 기록이 없으면 ok=false.
func (s *Store) Stats(mbID, rule string) (PlayerStats, bool) {
	var rows []PlayerStats
	if err := s.db.Raw(
		"SELECT rating, wins, losses, draws FROM angple_stack_stats WHERE mb_id = ? AND rule = ?", mbID, rule,
	).Scan(&rows).Error; err != nil || len(rows) == 0 {
		return PlayerStats{Rating: DefaultRating}, false
	}
	return rows[0], true
}

// Balance 는 보유 포인트다(조회 실패는 0 — 유료 매칭을 막는 쪽으로 닫힌다).
func (s *Store) Balance(mbID string) int {
	var p int
	if err := s.db.Raw("SELECT mb_point FROM g5_member WHERE mb_id = ?", mbID).Scan(&p).Error; err != nil {
		return 0
	}
	return p
}

// ratingOf 는 (회원, 규칙) 레이팅이다(없으면 기본값).
func ratingOf(tx *gorm.DB, mbID, rule string) int {
	var r int
	if err := tx.Raw("SELECT rating FROM angple_stack_stats WHERE mb_id = ? AND rule = ?", mbID, rule).Scan(&r).Error; err != nil || r == 0 {
		return DefaultRating
	}
	return r
}

// upsertStat 은 (회원, 규칙) 전적 행을 만들거나 더한다. 레이팅 하한은 100.
func upsertStat(tx *gorm.DB, mbID, rule string, win, loss, draw, ratingDelta int) error {
	return tx.Exec(
		`INSERT INTO angple_stack_stats (mb_id, rule, wins, losses, draws, rating, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, NOW())
		 ON DUPLICATE KEY UPDATE
		   wins = wins + VALUES(wins), losses = losses + VALUES(losses),
		   draws = draws + VALUES(draws),
		   rating = GREATEST(100, rating + ?), updated_at = NOW()`,
		mbID, rule, win, loss, draw, DefaultRating+ratingDelta, ratingDelta,
	).Error
}

// eloDelta 는 K=32 Elo 변동(승자 +, 패자 −)이다. 최소 1점은 움직인다.
func eloDelta(winnerRating, loserRating int) (int, int) {
	const k = 32.0
	expected := 1.0 / (1.0 + math.Pow(10, float64(loserRating-winnerRating)/400.0))
	delta := int(k * (1.0 - expected))
	if delta < 1 {
		delta = 1
	}
	return delta, -delta
}

// creditPointTx 는 tx 안에서 포인트를 적립한다(g5_point 기록 + mb_point 반영).
func creditPointTx(tx *gorm.DB, mbID string, point int, content, relTable, relID, relAction string) error {
	if err := tx.Table("g5_member").Where("mb_id = ?", mbID).
		UpdateColumn("mb_point", gorm.Expr("mb_point + ?", point)).Error; err != nil {
		return err
	}
	return insertPointLog(tx, mbID, point, content, relTable, relID, relAction)
}

// deductPointTx 는 tx 안에서 포인트를 선입선출로 차감한다(만료 임박 적립분부터 소진).
func deductPointTx(tx *gorm.DB, mbID string, amount int, content, relTable, relID, relAction string) error {
	type credit struct {
		PoID       int `gorm:"column:po_id"`
		PoPoint    int `gorm:"column:po_point"`
		PoUsePoint int `gorm:"column:po_use_point"`
	}
	var credits []credit
	if err := tx.Raw(`
		SELECT po_id, po_point, po_use_point FROM g5_point
		WHERE mb_id = ? AND po_expired = 0 AND po_point > 0 AND (po_point - po_use_point) > 0
		ORDER BY po_expire_date ASC, po_id ASC FOR UPDATE`, mbID).Scan(&credits).Error; err != nil {
		return err
	}
	remaining := amount
	for _, c := range credits {
		if remaining <= 0 {
			break
		}
		consume := c.PoPoint - c.PoUsePoint
		if consume > remaining {
			consume = remaining
		}
		updates := map[string]interface{}{"po_use_point": c.PoUsePoint + consume}
		if c.PoUsePoint+consume >= c.PoPoint {
			updates["po_expired"] = 100
		}
		if err := tx.Table("g5_point").Where("po_id = ?", c.PoID).Updates(updates).Error; err != nil {
			return err
		}
		remaining -= consume
	}
	if err := tx.Table("g5_member").Where("mb_id = ?", mbID).
		UpdateColumn("mb_point", gorm.Expr("mb_point - ?", amount)).Error; err != nil {
		return err
	}
	return insertPointLog(tx, mbID, -amount, content, relTable, relID, relAction)
}

// insertPointLog 는 g5_point 에 한 줄을 남긴다(po_mb_point 는 반영 후 잔액).
func insertPointLog(tx *gorm.DB, mbID string, point int, content, relTable, relID, relAction string) error {
	var mbPoint int
	if err := tx.Table("g5_member").Select("mb_point").Where("mb_id = ?", mbID).Scan(&mbPoint).Error; err != nil {
		return err
	}
	return tx.Exec(
		`INSERT INTO g5_point (mb_id, po_datetime, po_content, po_point, po_use_point,
		    po_expired, po_expire_date, po_mb_point, po_rel_table, po_rel_id, po_rel_action)
		 VALUES (?, ?, ?, ?, 0, 0, '9999-12-31', ?, ?, ?, ?)`,
		mbID, time.Now(), content, point, mbPoint, relTable, relID, relAction,
	).Error
}
