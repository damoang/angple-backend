package stacksrv

import (
	"time"

	"gorm.io/gorm"
)

// 뱀 게임 기록 저장 — 표 3개(angple_snake_runs·angple_snake_best·angple_snake_weekly).
// 최고·주간 기록은 「높을 때만 갱신」한다. MySQL 은 INSERT … ON DUPLICATE KEY UPDATE 로 한 문장에
// 처리해 동시에 끝난 판이 겹쳐도 안전하다(앙쌓기 SaveSoloRun 과 같은 식). 테스트용 sqlite 는
// UPDATE … WHERE score < ? 후 행이 없을 때만 INSERT 한다. 0점 판은 기록만 남기고 랭킹 표에는 넣지 않는다.
// 시각은 NOW() 대신 Go 에서 만든 KST 벽시계를 넣는다 — DB 세션 time_zone 에 기대지 않는다.

// snakeDayLayout 은 week_start 형식이다.
const snakeDayLayout = "2006-01-02"

// SnakeRow 는 뱀 게임 랭킹 한 줄이다. 회원 아이디는 싣지 않는다(닉네임만 공개).
type SnakeRow struct {
	// Rank 는 순위다.
	Rank int `json:"rank" gorm:"-"`
	// Nickname 은 닉네임이다.
	Nickname string `json:"nickname" gorm:"column:mb_nick"`
	// Score 는 점수다.
	Score int64 `json:"score" gorm:"column:score"`
	// AchievedAt 은 기록을 세운 시각(KST, RFC3339 +09:00)이다. 알 수 없으면 빈 문자열.
	AchievedAt string `json:"achievedAt" gorm:"-"`
	// AchievedRaw 는 DB 에서 문자열로 읽은 achieved_at 벽시계다(응답에는 싣지 않는다).
	AchievedRaw string `json:"-" gorm:"column:achieved_raw"`
}

// wallClockExpr 는 DATETIME 열을 "YYYY-MM-DD HH:MM:SS" 문자열로 읽는 식이다.
// 드라이버의 시각 해석(parseTime·loc)을 거치지 않게 한다. 테스트용 sqlite 에서는 strftime 을 쓴다.
func (s *Store) wallClockExpr(col string) string {
	if s.db.Name() == "sqlite" {
		return "COALESCE(strftime('%Y-%m-%d %H:%M:%S', " + col + "), '')"
	}
	return "COALESCE(DATE_FORMAT(" + col + ", '%Y-%m-%d %H:%i:%s'), '')"
}

// SnakeTop 은 뱀 게임 상위 n 줄이다. weekStart 가 영값이면 역대(best), 아니면 그 주(weekly).
// flagged 판은 best·weekly 에 들어가지 않으므로 자연히 빠진다. 탈퇴·이용 제한 회원은 뺀다.
func (s *Store) SnakeTop(weekStart time.Time, n int) ([]SnakeRow, error) {
	table, weekCond := "angple_snake_best", ""
	args := make([]interface{}, 0, 2)
	if !weekStart.IsZero() {
		table, weekCond = "angple_snake_weekly", "t.week_start = ? AND "
		args = append(args, weekStart.Format(snakeDayLayout))
	}
	args = append(args, n)
	query := "SELECT m.mb_nick, t.score, " + s.wallClockExpr("t.achieved_at") + " AS achieved_raw" +
		" FROM " + table + " t JOIN g5_member m ON m.mb_id = t.mb_id" +
		" WHERE " + weekCond + "m.mb_leave_date = '' AND m.mb_intercept_date = ''" +
		" ORDER BY t.score DESC, t.achieved_at ASC LIMIT ?"
	var rows []SnakeRow
	err := s.db.Raw(query, args...).Scan(&rows).Error
	return rows, err
}

// raiseSnakeBest 는 table 에서 키가 같은 행의 점수를 높을 때만 바꾸고(run_id·achieved_at 도 그때만), 행이 없으면 만든다.
// table·keyWhere·insertCols·insertMarks 는 이 파일 안의 고정 문자열만 넘긴다(사용자 값 아님).
func raiseSnakeBest(tx *gorm.DB, table, keyWhere, insertCols, insertMarks string, keyArgs []interface{}, rec SnakeRecord, achieved string) error {
	insertArgs := append(append(make([]interface{}, 0, len(keyArgs)+3), keyArgs...), rec.Score, rec.RunID, achieved)
	insert := "INSERT INTO " + table + " (" + insertCols + ", score, run_id, achieved_at) VALUES (" + insertMarks + ", ?, ?, ?)"
	if tx.Name() != "sqlite" {
		// MySQL 은 ON DUPLICATE KEY UPDATE 의 대입을 왼쪽부터 하므로 score 는 맨 끝에 바꾼다.
		return tx.Exec(insert+
			" ON DUPLICATE KEY UPDATE"+
			" run_id = IF(VALUES(score) > score, VALUES(run_id), run_id),"+
			" achieved_at = IF(VALUES(score) > score, VALUES(achieved_at), achieved_at),"+
			" score = GREATEST(score, VALUES(score))", insertArgs...).Error
	}
	upd := tx.Exec(
		"UPDATE "+table+" SET score = ?, run_id = ?, achieved_at = ? WHERE "+keyWhere+" AND score < ?",
		append(append([]interface{}{rec.Score, rec.RunID, achieved}, keyArgs...), rec.Score)...,
	)
	if upd.Error != nil || upd.RowsAffected > 0 {
		return upd.Error
	}
	var exists int64
	if err := tx.Raw("SELECT COUNT(*) FROM "+table+" WHERE "+keyWhere, keyArgs...).Scan(&exists).Error; err != nil || exists > 0 {
		return err
	}
	return tx.Exec(insert, insertArgs...).Error
}

// SaveSnakeRun 은 판을 남기고, 통과한 판이면 역대·주간 최고 기록을 높을 때만 갱신한 뒤 내 기록을 돌려준다.
func (s *Store) SaveSnakeRun(rec SnakeRecord) (SoloStanding, error) {
	week := rec.WeekStart.Format(snakeDayLayout)
	achieved := achievedWallKST(time.Now())
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`INSERT INTO angple_snake_runs
			   (run_id, mb_id, seed, score, ticks, inputs_json, elapsed_ms, flagged, flag_reason, started_at, finished_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rec.RunID, rec.MbID, rec.Seed, rec.Score, rec.Ticks, rec.InputsJSON, rec.ElapsedMs,
			rec.Flagged, rec.FlagReason, achievedWallKST(rec.StartedAt), achieved,
		).Error; err != nil {
			return err
		}
		if rec.Flagged || rec.Score <= 0 {
			return nil
		}
		if err := raiseSnakeBest(tx, "angple_snake_best", "mb_id = ?", "mb_id", "?",
			[]interface{}{rec.MbID}, rec, achieved); err != nil {
			return err
		}
		return raiseSnakeBest(tx, "angple_snake_weekly", "week_start = ? AND mb_id = ?", "week_start, mb_id", "?, ?",
			[]interface{}{week, rec.MbID}, rec, achieved)
	})
	if err != nil {
		return SoloStanding{}, err
	}
	return s.snakeStanding(rec.MbID, week)
}

// snakeBests 는 snakeStanding 의 한 줄 조회 결과다.
type snakeBests struct {
	// Best 는 역대 최고 점수다.
	Best int64 `gorm:"column:best"`
	// WeekBest 는 이번 주 최고 점수다.
	WeekBest int64 `gorm:"column:week_best"`
}

// snakeStanding 은 내 역대 최고·이번 주 최고·이번 주 순위다.
// 순위는 공개 랭킹과 같은 조건(탈퇴·이용 제한 회원 제외)으로 앞선 사람만 센다.
func (s *Store) snakeStanding(mbID, week string) (SoloStanding, error) {
	var b snakeBests
	err := s.db.Raw(
		`SELECT (SELECT COALESCE(MAX(score), 0) FROM angple_snake_best WHERE mb_id = ?) AS best,
		        (SELECT COALESCE(MAX(score), 0) FROM angple_snake_weekly WHERE week_start = ? AND mb_id = ?) AS week_best`,
		mbID, week, mbID,
	).Scan(&b).Error
	st := SoloStanding{Best: b.Best, WeekBest: b.WeekBest}
	if err != nil || st.WeekBest <= 0 {
		return st, err
	}
	var ahead int64
	err = s.db.Raw(
		`SELECT COUNT(*) FROM angple_snake_weekly w
		   JOIN g5_member m ON m.mb_id = w.mb_id
		  WHERE w.week_start = ? AND w.score > ? AND m.mb_leave_date = '' AND m.mb_intercept_date = ''`,
		week, st.WeekBest,
	).Scan(&ahead).Error
	st.RankWeek = int(ahead) + 1
	return st, err
}
