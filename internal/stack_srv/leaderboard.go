package stacksrv

import (
	"encoding/json"
	"net/http"
	"time"

	"gorm.io/gorm"
)

// 점수판 종류
const (
	BoardSoloWeek   = "solo_week"
	BoardSoloAll    = "solo_all"
	BoardVsAttack   = "vs_attack"
	BoardVsSprint40 = "vs_sprint40"
)

// LeaderboardSize 는 점수판 한 장의 인원이다.
const LeaderboardSize = 20

// kst 는 한국 표준시(UTC+9, 일광절약 없음)다. 컨테이너 tzdata 에 기대지 않으려고 고정 오프셋을 쓴다.
var kst = time.FixedZone("KST", 9*60*60)

// WeekStartKST 는 t 가 속한 주의 시작(KST 월요일 0시)이다. 주간 점수판은 이 시각에 새로 시작한다.
func WeekStartKST(t time.Time) time.Time {
	k := t.In(kst)
	offset := (int(k.Weekday()) + 6) % 7
	return time.Date(k.Year(), k.Month(), k.Day()-offset, 0, 0, 0, 0, kst)
}

// SoloRow 는 혼자하기 점수판 한 줄이다. 회원 아이디는 싣지 않는다(닉네임만 공개).
type SoloRow struct {
	// Rank 는 순위다.
	Rank int `json:"rank" gorm:"-"`
	// Nickname 은 닉네임이다.
	Nickname string `json:"nickname" gorm:"column:mb_nick"`
	// Score 는 점수다.
	Score int64 `json:"score" gorm:"column:score"`
	// Lines 는 지운 줄 합이다.
	Lines int64 `json:"lines" gorm:"column:cleared_lines"`
	// Level 은 레벨이다.
	Level int64 `json:"level" gorm:"column:level"`
}

// VersusRow 는 대전 점수판 한 줄이다. 회원 아이디는 싣지 않는다(닉네임만 공개).
type VersusRow struct {
	// Rank 는 순위다.
	Rank int `json:"rank" gorm:"-"`
	// Nickname 은 닉네임이다.
	Nickname string `json:"nickname" gorm:"column:mb_nick"`
	// Rating 은 레이팅이다.
	Rating int `json:"rating" gorm:"column:rating"`
	// Wins 는 승 수다.
	Wins int `json:"wins" gorm:"column:wins"`
	// Losses 는 패 수다.
	Losses int `json:"losses" gorm:"column:losses"`
	// Draws 는 무승부 수다.
	Draws int `json:"draws" gorm:"column:draws"`
}

// LeaderboardStore 는 점수판 조회 저장소다.
type LeaderboardStore interface {
	// SoloTop 은 혼자하기 상위 n 줄이다. weekStart 가 영값이면 역대, 아니면 그 주.
	SoloTop(weekStart time.Time, n int) ([]SoloRow, error)
	// VersusTop 은 대전 규칙별 레이팅 상위 n 줄이다.
	VersusTop(rule string, n int) ([]VersusRow, error)
}

// Leaderboard 는 GET /stack-ws/leaderboard?board= 처리기다. 공개·인증 불요.
// 서버 메모리 60초 캐시 + public, max-age=60 (기록은 판이 끝날 때만 변한다).
type Leaderboard struct {
	// store 는 저장소다.
	store LeaderboardStore
	// cache 는 보드별 응답 캐시다.
	cache jsonCache
	// now 는 시계다.
	now func() time.Time
}

// NewLeaderboard 는 점수판 처리기를 만든다.
func NewLeaderboard(store LeaderboardStore) *Leaderboard {
	return &Leaderboard{store: store, now: time.Now}
}

// ServeHTTP 는 board 하나를 돌려준다. 알 수 없는 board 는 400.
func (l *Leaderboard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeMethodNotAllowed(w)
		return
	}
	board := r.URL.Query().Get("board")
	if !knownBoard(board) {
		writeError(w, http.StatusBadRequest, "unknown_board")
		return
	}
	now := l.now()
	key := board
	if board == BoardSoloWeek {
		// 주가 바뀌면 캐시 키도 바뀐다 — 월요일 0시 직후에 지난주 표가 남지 않게.
		key = board + ":" + WeekStartKST(now).Format("2006-01-02")
	}
	body := l.cache.get(now, key, func() ([]byte, error) { return l.build(board, now) })
	if body == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writePublicJSON(w, body)
}

// knownBoard 는 열린 점수판인지다.
func knownBoard(board string) bool {
	switch board {
	case BoardSoloWeek, BoardSoloAll, BoardVsAttack, BoardVsSprint40:
		return true
	}
	return false
}

// build 는 점수판 하나의 응답 본문을 만든다.
func (l *Leaderboard) build(board string, now time.Time) ([]byte, error) {
	out := map[string]interface{}{"board": board}
	switch board {
	case BoardSoloWeek, BoardSoloAll:
		var week time.Time
		if board == BoardSoloWeek {
			week = WeekStartKST(now)
			out["weekStart"] = week.Format("2006-01-02")
		}
		rows, err := l.store.SoloTop(week, LeaderboardSize)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			rows[i].Rank = i + 1
		}
		if rows == nil {
			rows = []SoloRow{}
		}
		out["rows"] = rows
	default:
		rule := RuleAttack
		if board == BoardVsSprint40 {
			rule = RuleSprint40
		}
		rows, err := l.store.VersusTop(rule, LeaderboardSize)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			rows[i].Rank = i + 1
		}
		if rows == nil {
			rows = []VersusRow{}
		}
		out["rows"] = rows
	}
	return json.Marshal(out)
}

// SoloTop 은 혼자하기 상위 n 줄이다(flagged 판은 best·weekly 에 들어가지 않으므로 자연히 빠진다).
// 탈퇴했거나 이용 제한 중인 회원은 공개 점수판에서 뺀다(자동 등업과 같은 조건).
func (s *Store) SoloTop(weekStart time.Time, n int) ([]SoloRow, error) {
	var rows []SoloRow
	if weekStart.IsZero() {
		err := s.db.Raw(
			`SELECT m.mb_nick, b.score, b.cleared_lines, b.level
			   FROM angple_stack_solo_best b
			   JOIN g5_member m ON m.mb_id = b.mb_id
			  WHERE m.mb_leave_date = '' AND m.mb_intercept_date = ''
			  ORDER BY b.score DESC, b.achieved_at ASC LIMIT ?`, n,
		).Scan(&rows).Error
		return rows, err
	}
	err := s.db.Raw(
		`SELECT m.mb_nick, w.score, w.cleared_lines, w.level
		   FROM angple_stack_solo_weekly w
		   JOIN g5_member m ON m.mb_id = w.mb_id
		  WHERE w.week_start = ? AND m.mb_leave_date = '' AND m.mb_intercept_date = ''
		  ORDER BY w.score DESC, w.achieved_at ASC LIMIT ?`, weekStart.Format("2006-01-02"), n,
	).Scan(&rows).Error
	return rows, err
}

// VersusTop 은 대전 규칙별 레이팅 상위 n 줄이다(첫 판이 끝나야 stats 행이 생긴다). 탈퇴·이용 제한 회원은 뺀다.
func (s *Store) VersusTop(rule string, n int) ([]VersusRow, error) {
	var rows []VersusRow
	err := s.db.Raw(
		`SELECT m.mb_nick, t.rating, t.wins, t.losses, t.draws
		   FROM angple_stack_stats t
		   JOIN g5_member m ON m.mb_id = t.mb_id
		  WHERE t.rule = ? AND m.mb_leave_date = '' AND m.mb_intercept_date = ''
		  ORDER BY t.rating DESC, t.wins DESC LIMIT ?`, rule, n,
	).Scan(&rows).Error
	return rows, err
}

// SaveSoloRun 은 판을 남기고, 통과한 판이면 역대·주간 최고 기록을 높을 때만 갱신한 뒤 내 기록을 돌려준다.
func (s *Store) SaveSoloRun(rec SoloRecord) (SoloStanding, error) {
	week := rec.WeekStart.Format("2006-01-02")
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`INSERT INTO angple_stack_solo_runs
			   (run_id, mb_id, seed, score, cleared_lines, level, ticks, pieces, clears_json, elapsed_ms,
			    flagged, flag_reason, source, started_at, finished_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'game', ?, NOW())`,
			rec.RunID, rec.MbID, rec.Seed, rec.Score, rec.Lines, rec.Level, rec.Ticks, rec.Pieces, rec.ClearsJSON, rec.ElapsedMs,
			rec.Flagged, rec.FlagReason, rec.StartedAt.In(kst).Format("2006-01-02 15:04:05"),
		).Error; err != nil {
			return err
		}
		if rec.Flagged {
			return nil
		}
		// MySQL 은 ON DUPLICATE KEY UPDATE 의 대입을 왼쪽부터 하므로 score 는 맨 끝에 바꾼다.
		if err := tx.Exec(
			`INSERT INTO angple_stack_solo_best (mb_id, score, cleared_lines, level, run_id, achieved_at)
			 VALUES (?, ?, ?, ?, ?, NOW())
			 ON DUPLICATE KEY UPDATE
			   cleared_lines = IF(VALUES(score) > score, VALUES(cleared_lines), cleared_lines),
			   level = IF(VALUES(score) > score, VALUES(level), level),
			   run_id = IF(VALUES(score) > score, VALUES(run_id), run_id),
			   achieved_at = IF(VALUES(score) > score, NOW(), achieved_at),
			   score = GREATEST(score, VALUES(score))`,
			rec.MbID, rec.Score, rec.Lines, rec.Level, rec.RunID,
		).Error; err != nil {
			return err
		}
		return tx.Exec(
			`INSERT INTO angple_stack_solo_weekly (week_start, mb_id, score, cleared_lines, level, run_id, achieved_at)
			 VALUES (?, ?, ?, ?, ?, ?, NOW())
			 ON DUPLICATE KEY UPDATE
			   cleared_lines = IF(VALUES(score) > score, VALUES(cleared_lines), cleared_lines),
			   level = IF(VALUES(score) > score, VALUES(level), level),
			   run_id = IF(VALUES(score) > score, VALUES(run_id), run_id),
			   achieved_at = IF(VALUES(score) > score, NOW(), achieved_at),
			   score = GREATEST(score, VALUES(score))`,
			week, rec.MbID, rec.Score, rec.Lines, rec.Level, rec.RunID,
		).Error
	})
	if err != nil {
		return SoloStanding{}, err
	}
	var st SoloStanding
	if err := s.db.Raw("SELECT COALESCE(MAX(score), 0) FROM angple_stack_solo_best WHERE mb_id = ?", rec.MbID).Scan(&st.Best).Error; err != nil {
		return st, err
	}
	if err := s.db.Raw("SELECT COALESCE(MAX(score), 0) FROM angple_stack_solo_weekly WHERE week_start = ? AND mb_id = ?", week, rec.MbID).Scan(&st.WeekBest).Error; err != nil {
		return st, err
	}
	if st.WeekBest > 0 {
		var ahead int
		if err := s.db.Raw("SELECT COUNT(*) FROM angple_stack_solo_weekly WHERE week_start = ? AND score > ?", week, st.WeekBest).Scan(&ahead).Error; err != nil {
			return st, err
		}
		st.RankWeek = ahead + 1
	}
	return st, nil
}
