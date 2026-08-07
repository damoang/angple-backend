package janggisrv

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ErrInsufficientPoint 는 참가비를 낼 잔액이 없을 때다.
var ErrInsufficientPoint = errors.New("보유 포인트가 부족합니다")

// Store 는 대국·참가비·전적의 영속화를 담당한다.
//
// ⛔ 포인트 차감은 반드시 여기의 트랜잭션 경로로만 한다. 잔액 확인을 락 없이
// 하고(예: CanAfford) 별도 트랜잭션에서 차감하면 동시 요청에 이중지출이 난다
// (나눔 응모 giving_bid_handler 가 같은 이유로 FOR UPDATE 를 쓴다).
type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// CreateGame 은 대국 행을 만들고 id 를 돌려준다.
func (s *Store) CreateGame(choMbID, hanMbID string, entryFee int) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	type row struct {
		ID int64 `gorm:"column:id"`
	}
	res := s.db.Exec(
		`INSERT INTO angple_janggi_games (cho_mb_id, han_mb_id, status, entry_fee, started_at)
		 VALUES (?, ?, 'playing', ?, NOW())`,
		choMbID, hanMbID, entryFee,
	)
	if res.Error != nil {
		return 0, res.Error
	}
	var r row
	if err := s.db.Raw("SELECT LAST_INSERT_ID() AS id").Scan(&r).Error; err != nil {
		return 0, err
	}
	return r.ID, nil
}

// ChargeEntryFee 는 한 명의 참가비를 원자적으로 차감한다.
//
// 단일 트랜잭션 안에서
//  1. g5_member 행 배타 락 + 잔액 확인 (TOCTOU 차단)
//  2. angple_janggi_entries INSERT — UNIQUE(game_id, mb_id) 가 재시도 중복의 최종 방어선
//     (g5_point 인덱스는 전부 non-unique 라 DB 가 이중 차감을 막아주지 못한다)
//  3. g5_point FIFO 차감 + mb_point 반영
func (s *Store) ChargeEntryFee(gameID int64, mbID string, amount int) error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var balance int
		if err := tx.Raw(
			"SELECT mb_point FROM g5_member WHERE mb_id = ? FOR UPDATE", mbID,
		).Scan(&balance).Error; err != nil {
			return err
		}
		if balance < amount {
			return ErrInsufficientPoint
		}
		if err := tx.Exec(
			`INSERT INTO angple_janggi_entries (game_id, mb_id, point_deducted, created_at)
			 VALUES (?, ?, ?, NOW())`,
			gameID, mbID, amount,
		).Error; err != nil {
			return err // UNIQUE 위반이면 이미 차감된 것 — 재차감하지 않는다
		}
		return deductPointTx(tx, mbID, amount,
			"장기 대국 참가비", "angple_janggi_games", fmt.Sprint(gameID), "omok_entry")
	})
}

// RefundEntryFee 는 대국이 성립하지 못했을 때 참가비를 되돌린다.
// (상대 잔액 부족·서버 재시작으로 인한 중단 등)
func (s *Store) RefundEntryFee(gameID int64, mbID string, amount int) error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 실제 차감된 기록이 있을 때만 환불한다(중복 환불 방지).
		var cnt int64
		if err := tx.Raw(
			"SELECT COUNT(*) FROM angple_janggi_entries WHERE game_id = ? AND mb_id = ? AND refunded_at IS NULL",
			gameID, mbID,
		).Scan(&cnt).Error; err != nil {
			return err
		}
		if cnt == 0 {
			return nil
		}
		if err := tx.Exec(
			"UPDATE angple_janggi_entries SET refunded_at = NOW() WHERE game_id = ? AND mb_id = ?",
			gameID, mbID,
		).Error; err != nil {
			return err
		}
		return creditPointTx(tx, mbID, amount,
			"장기 대국 참가비 환불", "angple_janggi_games", fmt.Sprint(gameID), "omok_entry_refund")
	})
}

// FinishGame 은 대국 결과를 기록하고 양쪽 전적·레이팅을 갱신한다.
// winnerMbID 가 빈 문자열이면 무승부.
func (s *Store) FinishGame(gameID int64, choMbID, hanMbID, winnerMbID, reason string, moves []Move) error {
	if s == nil || s.db == nil {
		return nil
	}
	movesJSON := marshalMoves(moves)
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`UPDATE angple_janggi_games
			    SET status = 'finished', winner_mb_id = NULLIF(?, ''), end_reason = ?,
			        ended_at = NOW(), moves_json = ?
			  WHERE id = ?`,
			winnerMbID, reason, movesJSON, gameID,
		).Error; err != nil {
			return err
		}
		if winnerMbID == "" {
			if err := upsertStat(tx, choMbID, 0, 0, 1, 0); err != nil {
				return err
			}
			return upsertStat(tx, hanMbID, 0, 0, 1, 0)
		}
		loserMbID := choMbID
		if winnerMbID == choMbID {
			loserMbID = hanMbID
		}
		wR := s.ratingOf(tx, winnerMbID)
		lR := s.ratingOf(tx, loserMbID)
		wDelta, lDelta := eloDelta(wR, lR)
		if err := upsertStat(tx, winnerMbID, 1, 0, 0, wDelta); err != nil {
			return err
		}
		return upsertStat(tx, loserMbID, 0, 1, 0, lDelta)
	})
}

// AbortGame 은 서버 중단 등으로 성립하지 못한 대국을 정리한다(참가비는 환불).
func (s *Store) AbortGame(gameID int64, reason string) error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Exec(
		"UPDATE angple_janggi_games SET status = 'aborted', end_reason = ?, ended_at = NOW() WHERE id = ? AND status = 'playing'",
		reason, gameID,
	).Error
}

// AbortStalePlayingGames 는 기동 시 'playing' 으로 남은 대국을 정리하고 참가비를 환불한다.
// 게임 상태는 메모리에만 있으므로 재시작하면 이어갈 수 없다 — 돈만 돌려준다.
func (s *Store) AbortStalePlayingGames() (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	type entry struct {
		GameID int64  `gorm:"column:game_id"`
		MbID   string `gorm:"column:mb_id"`
		Point  int    `gorm:"column:point_deducted"`
	}
	var entries []entry
	if err := s.db.Raw(
		`SELECT e.game_id, e.mb_id, e.point_deducted
		   FROM angple_janggi_entries e
		   JOIN angple_janggi_games g ON g.id = e.game_id
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
	if err := s.db.Exec(
		"UPDATE angple_janggi_games SET status = 'aborted', end_reason = 'server_restart', ended_at = NOW() WHERE status = 'playing'",
	).Error; err != nil {
		return refunded, err
	}
	return refunded, nil
}

// Rating 은 회원의 현재 레이팅을 돌려준다(기록 없으면 기본값).
func (s *Store) Rating(mbID string) int {
	if s == nil || s.db == nil {
		return DefaultRating
	}
	return s.ratingOf(s.db, mbID)
}

// Stats 는 전적을 돌려준다.
func (s *Store) Stats(mbID string) (wins, losses, draws, rating int) {
	rating = DefaultRating
	if s == nil || s.db == nil {
		return
	}
	var row struct {
		Wins   int `gorm:"column:wins"`
		Losses int `gorm:"column:losses"`
		Draws  int `gorm:"column:draws"`
		Rating int `gorm:"column:rating"`
	}
	if err := s.db.Raw(
		"SELECT wins, losses, draws, rating FROM angple_janggi_stats WHERE mb_id = ?", mbID,
	).Scan(&row).Error; err != nil || row.Rating == 0 {
		return 0, 0, 0, DefaultRating
	}
	return row.Wins, row.Losses, row.Draws, row.Rating
}

// RankingRow 는 공개 랭킹 한 줄이다.
// ⛔ mb_id 는 담지 않는다 — 공개 표면에는 닉네임까지만 나간다(개인정보 원칙).
type RankingRow struct {
	Nickname string `gorm:"column:mb_nick" json:"nickname"`
	Wins     int    `gorm:"column:wins" json:"wins"`
	Losses   int    `gorm:"column:losses" json:"losses"`
	Draws    int    `gorm:"column:draws" json:"draws"`
	Rating   int    `gorm:"column:rating" json:"rating"`
}

// RankingTop 은 레이팅 상위 n 명을 돌려준다. 대국 이력이 있는 회원만 나온다
// (stats 행은 첫 대국 종료 때 생기므로 별도 필터가 필요 없다).
func (s *Store) RankingTop(n int) []RankingRow {
	if s == nil || s.db == nil {
		return nil
	}
	if n <= 0 || n > 100 {
		n = 20
	}
	var rows []RankingRow
	if err := s.db.Raw(
		`SELECT m.mb_nick, t.wins, t.losses, t.draws, t.rating
		 FROM angple_janggi_stats t
		 JOIN g5_member m ON m.mb_id = t.mb_id
		 ORDER BY t.rating DESC, t.wins DESC LIMIT ?`, n,
	).Scan(&rows).Error; err != nil {
		return nil
	}
	return rows
}

func (s *Store) ratingOf(tx *gorm.DB, mbID string) int {
	var r int
	if err := tx.Raw("SELECT rating FROM angple_janggi_stats WHERE mb_id = ?", mbID).Scan(&r).Error; err != nil || r == 0 {
		return DefaultRating
	}
	return r
}

func upsertStat(tx *gorm.DB, mbID string, win, loss, draw, ratingDelta int) error {
	return tx.Exec(
		`INSERT INTO angple_janggi_stats (mb_id, wins, losses, draws, rating, updated_at)
		 VALUES (?, ?, ?, ?, ?, NOW())
		 ON DUPLICATE KEY UPDATE
		   wins = wins + VALUES(wins), losses = losses + VALUES(losses),
		   draws = draws + VALUES(draws),
		   rating = GREATEST(100, rating + ?), updated_at = NOW()`,
		mbID, win, loss, draw, DefaultRating+ratingDelta, ratingDelta,
	).Error
}

// eloDelta 는 K=32 Elo 변동을 돌려준다.
func eloDelta(winnerRating, loserRating int) (int, int) {
	const k = 32.0
	expected := 1.0 / (1.0 + pow10(float64(loserRating-winnerRating)/400.0))
	delta := int(k * (1.0 - expected))
	if delta < 1 {
		delta = 1
	}
	return delta, -delta
}

func pow10(x float64) float64 {
	// math.Pow(10, x) 와 같다. import 최소화를 위해 직접.
	result := 1.0
	neg := x < 0
	if neg {
		x = -x
	}
	whole := int(x)
	frac := x - float64(whole)
	for i := 0; i < whole; i++ {
		result *= 10
	}
	// 소수부는 근사(2.302585 = ln10)
	result *= expApprox(frac * 2.302585092994046)
	if neg {
		return 1 / result
	}
	return result
}

func expApprox(x float64) float64 {
	sum, term := 1.0, 1.0
	for i := 1; i <= 12; i++ {
		term *= x / float64(i)
		sum += term
	}
	return sum
}

// creditPointTx / deductPointTx 는 gnuboard_point_write_repo 의 적립·차감을
// tx 스코프로 옮긴 것이다(나눔 응모와 동일 방식 — 원자성을 위해 필요).
func creditPointTx(tx *gorm.DB, mbID string, point int, content, relTable, relID, relAction string) error {
	if err := tx.Table("g5_member").Where("mb_id = ?", mbID).
		UpdateColumn("mb_point", gorm.Expr("mb_point + ?", point)).Error; err != nil {
		return err
	}
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
		available := c.PoPoint - c.PoUsePoint
		consume := available
		if consume > remaining {
			consume = remaining
		}
		newUse := c.PoUsePoint + consume
		updates := map[string]interface{}{"po_use_point": newUse}
		if newUse >= c.PoPoint {
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
	var mbPoint int
	if err := tx.Table("g5_member").Select("mb_point").Where("mb_id = ?", mbID).Scan(&mbPoint).Error; err != nil {
		return err
	}
	return tx.Exec(
		`INSERT INTO g5_point (mb_id, po_datetime, po_content, po_point, po_use_point,
		    po_expired, po_expire_date, po_mb_point, po_rel_table, po_rel_id, po_rel_action)
		 VALUES (?, ?, ?, ?, 0, 0, '9999-12-31', ?, ?, ?, ?)`,
		mbID, time.Now(), content, -amount, mbPoint, relTable, relID, relAction,
	).Error
}
