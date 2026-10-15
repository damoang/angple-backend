package cron

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"gorm.io/gorm"
)

// cronEnvInt reads a positive int env var, falling back to def.
func cronEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// PopularSubscribeResult contains the result of a popular-post subscribe notify run.
type PopularSubscribeResult struct {
	Boards        int    `json:"boards"`         // level=2 구독자 있는 보드 수
	PostsNotified int    `json:"posts_notified"` // 이번 run 에 인기글로 통지된 글 수
	NotisCreated  int    `json:"notis_created"`  // 생성된 알림 수
	ExecutedAt    string `json:"executed_at"`
}

var boardSlugRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// runPopularSubscribeNotify sends a single "popular post" notification to level=2
// (인기글만) board subscribers when a post crosses the recommend threshold (#12607).
// 글 작성 시점엔 추천=0 이라 write 시 못 보내므로, 주기 cron 으로 임계값 도달 글을 잡아
// 보드당 글당 1회만 알림(g5_board_subscribe_notified 로 중복 방지).
func runPopularSubscribeNotify(db *gorm.DB) (*PopularSubscribeResult, error) {
	threshold := cronEnvInt("NOTI_POPULAR_THRESHOLD", 10)   // 추천글 기준과 정합되게 운영에서 조정
	windowDays := cronEnvInt("NOTI_POPULAR_WINDOW_DAYS", 3) // 최근 N일 내 글만 대상(오래된 글 재통지 방지)
	maxPostsPerBoard := cronEnvInt("NOTI_POPULAR_MAX_POSTS", 200)
	now := time.Now()
	cutoff := now.AddDate(0, 0, -windowDays)

	res := &PopularSubscribeResult{ExecutedAt: now.Format("2006-01-02 15:04:05")}

	// 1) level=2 구독자가 있는 보드 목록
	var boards []string
	if err := db.Table("g5_board_subscribe").Distinct("bo_table").
		Where("level = 2").Pluck("bo_table", &boards).Error; err != nil {
		return res, err
	}
	floorBoards := popularFloorBoards(db, boards)

	for _, board := range boards {
		if !boardSlugRe.MatchString(board) {
			continue // 동적 테이블명 인젝션 방지
		}
		res.Boards++

		// 2) 임계값 도달 + 최근 + 미통지 글
		posts, err := selectPopularCandidates(db, board, threshold, cutoff, maxPostsPerBoard, floorBoards[board])
		if err != nil {
			continue // 테이블 없음 등은 건너뜀
		}
		if len(posts) == 0 {
			continue
		}

		// 게시판 한글명 (없으면 slug)
		boardName := board
		db.Table("g5_board").Select("bo_subject").Where("bo_table = ?", board).Limit(1).Scan(&boardName)
		if boardName == "" {
			boardName = board
		}

		// 3) 이 보드의 level=2 구독자 (noti_board_subscribe OFF 제외)
		var subs []string
		db.Table("g5_board_subscribe").Select("mb_id").
			Where("bo_table = ? AND level = 2", board).Pluck("mb_id", &subs)
		offSet := map[string]bool{}
		if len(subs) > 0 {
			var off []string
			db.Table("g5_noti_preference").Select("mb_id").
				Where("noti_board_subscribe = 0 AND mb_id IN ?", subs).Pluck("mb_id", &off)
			for _, o := range off {
				offSet[o] = true
			}
		}

		for _, p := range posts {
			// claim: 통지 전에 먼저 마킹(원자적 INSERT IGNORE)해 동시 cron 실행 시
			// 같은 글 중복 통지를 막는다. 이미 선점됐으면(RowsAffected==0) skip.
			// 구독자 0명이어도 마킹돼 재스캔되지 않는다.
			claim := db.Exec("INSERT IGNORE INTO g5_board_subscribe_notified (bo_table, wr_id) VALUES (?, ?)", board, p.WrID)
			if claim.Error != nil || claim.RowsAffected == 0 {
				continue
			}
			res.PostsNotified++

			authorName := p.WrName
			if authorName == "" {
				authorName = p.MbID
			}
			// 작성자를 차단한 구독자는 제외 (bug/13252 — write_after_worker 와 같은 규칙).
			// ⛔ block_scope='all' 만. 'message' 는 쪽지 한정이라 거르면 안 된다.
			blockedBy := map[string]bool{}
			{
				var ids []string
				db.Table("g5_member_block").Select("mb_id").
					Where("blocked_mb_id = ? AND block_scope = 'all'", p.MbID).
					Pluck("mb_id", &ids)
				for _, id := range ids {
					blockedBy[id] = true
				}
			}
			// 작성자를 팔로우 중인 구독자도 제외 — 글 작성 시 follow 알림을 이미 받았다.
			// (level=1 구독은 write_after_worker 가 같은 제외를 하는데 여기(level=2)만 빠져
			// 팔로우+인기글 2중 알림이 갔다.)
			followerSet := map[string]bool{}
			{
				var ids []string
				db.Table("g5_member_follow").Select("mb_id").
					Where("target_id = ?", p.MbID).Pluck("mb_id", &ids)
				for _, id := range ids {
					followerSet[id] = true
				}
			}
			for _, sid := range subs {
				if sid == p.MbID || offSet[sid] || blockedBy[sid] || followerSet[sid] {
					continue
				}
				noti := &gnurepo.Notification{
					PhToCase: "subscribe", PhFromCase: "write", BoTable: board,
					WrID: p.WrID, MbID: sid, RelMbID: p.MbID,
					RelMbNick:  authorName,
					RelMsg:     fmt.Sprintf("%s 게시판 인기글: %s", boardName, p.Subject),
					RelURL:     fmt.Sprintf("/%s/%d", board, p.WrID),
					PhReaded:   "N",
					PhDatetime: now,
					WrParent:   p.WrID,
				}
				if err := db.Create(noti).Error; err == nil {
					res.NotisCreated++
				}
			}
		}
	}

	return res, nil
}

// popularPostRow is a candidate post for the popular-post subscribe notification.
type popularPostRow struct {
	WrID    int
	Subject string
	MbID    string
	WrName  string
}

// popularPostFloor returns the smallest wr_id written at or after cutoff.
// found=false means the board has no post in the window. Test seam: tests may
// replace it to simulate a failing floor lookup.
var popularPostFloor = func(db *gorm.DB, board string, cutoff time.Time) (floor int64, found bool, err error) {
	var minID sql.NullInt64
	q := fmt.Sprintf("SELECT MIN(wr_id) FROM g5_write_%s WHERE wr_datetime >= ?", board)
	if err := db.Raw(q, cutoff).Row().Scan(&minID); err != nil {
		return 0, false, err
	}
	if !minID.Valid {
		return 0, false, nil
	}
	return minID.Int64, true, nil
}

// popularDatetimeIndexedTables returns which of tables have an index whose
// first column is wr_datetime. Test seam: sqlite has no information_schema.
var popularDatetimeIndexedTables = func(db *gorm.DB, tables []string) ([]string, error) {
	var names []string
	err := db.Raw(
		"SELECT DISTINCT TABLE_NAME FROM information_schema.STATISTICS "+
			"WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ? "+
			"AND SEQ_IN_INDEX = 1 AND COLUMN_NAME = 'wr_datetime'", tables).
		Scan(&names).Error
	return names, err
}

// popularFloorBoards returns the boards whose write table can serve the floor
// lookup from a wr_datetime index. 인덱스가 없는 게시판은 하한 조회가 전체 스캔이
// 되므로 기존 쿼리 그대로 둔다. 조회 실패 시 모든 게시판에서 하한을 쓰지 않는다.
func popularFloorBoards(db *gorm.DB, boards []string) map[string]bool {
	result := map[string]bool{}
	tables := make([]string, 0, len(boards))
	for _, board := range boards {
		if boardSlugRe.MatchString(board) {
			tables = append(tables, "g5_write_"+board)
		}
	}
	if len(tables) == 0 {
		return result
	}
	names, err := popularDatetimeIndexedTables(db, tables)
	if err != nil {
		log.Printf("[Cron:popular-subscribe-notify] index lookup failed, scanning without floor: %v", err)
		return result
	}
	for _, name := range names {
		if board, ok := strings.CutPrefix(name, "g5_write_"); ok {
			result[board] = true
		}
	}
	return result
}

// selectPopularCandidates returns posts in board that reached threshold since
// cutoff and have not been notified yet. useFloor 이면 대상 기간의 첫 글 번호(하한)를
// 먼저 구해 그 뒤 구간만 읽는다 — ORDER BY wr_id 로 게시판 처음부터 훑는 것을 막는다.
// 기간 안 글이 없으면 빈 결과, 하한 조회가 실패하면 하한 없이 기존 쿼리로 조회한다.
func selectPopularCandidates(db *gorm.DB, board string, threshold int, cutoff time.Time, limit int, useFloor bool) ([]popularPostRow, error) {
	floorCond := ""
	args := []interface{}{board, threshold, cutoff}
	if !useFloor {
		args = append(args, limit)
		return queryPopularCandidates(db, board, floorCond, args)
	}
	floor, found, err := popularPostFloor(db, board, cutoff)
	switch {
	case err != nil:
		log.Printf("[Cron:popular-subscribe-notify] board=%s floor lookup failed, scanning without floor: %v", board, err)
	case !found:
		return nil, nil
	default:
		floorCond = "AND w.wr_id >= ? "
		args = append(args, floor)
	}
	args = append(args, limit)
	return queryPopularCandidates(db, board, floorCond, args)
}

// queryPopularCandidates runs the candidate query, with floorCond inserted
// before the notified check when a floor is used.
func queryPopularCandidates(db *gorm.DB, board, floorCond string, args []interface{}) ([]popularPostRow, error) {
	var posts []popularPostRow
	q := fmt.Sprintf(
		"SELECT w.wr_id AS wr_id, w.wr_subject AS subject, w.mb_id AS mb_id, w.wr_name AS wr_name "+
			"FROM g5_write_%s w "+
			"LEFT JOIN g5_board_subscribe_notified n ON n.bo_table = ? AND n.wr_id = w.wr_id "+
			"WHERE w.wr_is_comment = 0 AND w.wr_good >= ? AND w.wr_datetime >= ? "+floorCond+"AND n.wr_id IS NULL "+
			"ORDER BY w.wr_id LIMIT ?", board)
	if err := db.Raw(q, args...).Scan(&posts).Error; err != nil {
		return nil, err
	}
	return posts, nil
}
