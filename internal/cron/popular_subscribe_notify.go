package cron

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
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

	type postRow struct {
		WrID    int
		Subject string
		MbID    string
		WrName  string
	}

	for _, board := range boards {
		if !boardSlugRe.MatchString(board) {
			continue // 동적 테이블명 인젝션 방지
		}
		res.Boards++

		// 2) 임계값 도달 + 최근 + 미통지 글
		var posts []postRow
		q := fmt.Sprintf(
			"SELECT w.wr_id AS wr_id, w.wr_subject AS subject, w.mb_id AS mb_id, w.wr_name AS wr_name "+
				"FROM g5_write_%s w "+
				"LEFT JOIN g5_board_subscribe_notified n ON n.bo_table = ? AND n.wr_id = w.wr_id "+
				"WHERE w.wr_is_comment = 0 AND w.wr_good >= ? AND w.wr_datetime >= ? AND n.wr_id IS NULL "+
				"ORDER BY w.wr_id LIMIT ?", board)
		if err := db.Raw(q, board, threshold, cutoff, maxPostsPerBoard).Scan(&posts).Error; err != nil {
			continue // 테이블 없음 등은 건너뜀
		}
		if len(posts) == 0 {
			continue
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
			authorName := p.WrName
			if authorName == "" {
				authorName = p.MbID
			}
			for _, sid := range subs {
				if sid == p.MbID || offSet[sid] {
					continue
				}
				noti := &gnurepo.Notification{
					PhToCase: "subscribe", PhFromCase: "write", BoTable: board,
					WrID: p.WrID, MbID: sid, RelMbID: p.MbID,
					RelMbNick:  authorName,
					RelMsg:     fmt.Sprintf("%s 게시판 인기글: %s", board, p.Subject),
					RelURL:     fmt.Sprintf("/%s/%d", board, p.WrID),
					PhReaded:   "N",
					PhDatetime: now,
					WrParent:   p.WrID,
				}
				if err := db.Create(noti).Error; err == nil {
					res.NotisCreated++
				}
			}
			// 통지 마킹 (구독자 0명이어도 마킹해 재스캔 방지)
			db.Exec("INSERT IGNORE INTO g5_board_subscribe_notified (bo_table, wr_id) VALUES (?, ?)", board, p.WrID)
			res.PostsNotified++
		}
	}

	return res, nil
}
