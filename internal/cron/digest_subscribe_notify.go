package cron

import (
	"fmt"
	"strings"
	"time"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"gorm.io/gorm"
)

// DigestSubscribeResult contains the result of a digest subscribe notify run.
type DigestSubscribeResult struct {
	Boards          int    `json:"boards"`           // level=3 구독자 있는 보드 수
	Seeded          int    `json:"seeded"`           // 첫 run 커서 초기화(통지 0)된 보드 수
	PostsSummarized int    `json:"posts_summarized"` // 이번 run 에 요약된 글 수
	NotisCreated    int    `json:"notis_created"`    // 생성된 요약 알림 수
	ExecutedAt      string `json:"executed_at"`
}

// runDigestSubscribeNotify sends one "digest" notification per level=3 (요약) board
// subscriber, summarizing posts newer than the per-board cursor (#12607 P1).
// 새 글마다 보내지 않고 cron 주기(예: 1일 1~2회)마다 모아서 1건으로 보낸다.
// 커서(g5_board_subscribe_digest_cursor.last_wr_id)를 넘는 글만 묶고 run 끝에 커서를
// 전진시켜 같은 글 중복 요약을 막는다. 커서가 없는 보드는 현재 MAX(wr_id)로 시드하고
// 통지하지 않아(소급 폭발 방지), 신규 level=3 구독도 안전하게 다음 run 부터 요약된다.
func runDigestSubscribeNotify(db *gorm.DB) (*DigestSubscribeResult, error) {
	maxPreview := cronEnvInt("NOTI_DIGEST_PREVIEW", 3)    // 미리보기 제목 개수
	maxPosts := cronEnvInt("NOTI_DIGEST_MAX_POSTS", 1000) // 보드당 1 run 집계 상한
	now := time.Now()
	res := &DigestSubscribeResult{ExecutedAt: now.Format("2006-01-02 15:04:05")}

	// 1) level=3 구독자가 있는 보드
	var boards []string
	if err := db.Table("g5_board_subscribe").Distinct("bo_table").
		Where("level = 3").Pluck("bo_table", &boards).Error; err != nil {
		return res, err
	}

	type postRow struct {
		WrID    int
		Subject string
	}

	for _, board := range boards {
		if !boardSlugRe.MatchString(board) {
			continue // 동적 테이블명 인젝션 방지
		}
		res.Boards++

		// 현재 커서
		var cursor struct{ LastWrID int }
		scan := db.Table("g5_board_subscribe_digest_cursor").
			Select("last_wr_id AS last_wr_id").Where("bo_table = ?", board).Limit(1).Scan(&cursor)
		hasCursor := scan.Error == nil && scan.RowsAffected > 0

		// 보드 최대 wr_id (테이블 없음 등은 건너뜀)
		var maxWr int
		if err := db.Raw(fmt.Sprintf(
			"SELECT COALESCE(MAX(wr_id),0) FROM g5_write_%s WHERE wr_is_comment=0", board)).
			Scan(&maxWr).Error; err != nil {
			continue
		}

		// 첫 run: 커서를 현재 MAX 로 시드만 하고 통지 0 (배포 이전 글 소급 폭발 방지)
		if !hasCursor {
			db.Exec("INSERT IGNORE INTO g5_board_subscribe_digest_cursor (bo_table, last_wr_id, updated_at) VALUES (?, ?, ?)",
				board, maxWr, now)
			res.Seeded++
			continue
		}

		if maxWr <= cursor.LastWrID {
			continue // 새 글 없음
		}

		// 2) cursor 초과 새 글
		var posts []postRow
		q := fmt.Sprintf(
			"SELECT wr_id AS wr_id, wr_subject AS subject FROM g5_write_%s "+
				"WHERE wr_is_comment=0 AND wr_id > ? ORDER BY wr_id LIMIT ?", board)
		if err := db.Raw(q, cursor.LastWrID, maxPosts).Scan(&posts).Error; err != nil {
			continue
		}
		if len(posts) == 0 {
			db.Exec("UPDATE g5_board_subscribe_digest_cursor SET last_wr_id=?, updated_at=? WHERE bo_table=?",
				maxWr, now, board)
			continue
		}

		count := len(posts)
		newCursor := posts[count-1].WrID

		previews := make([]string, 0, maxPreview)
		for i := 0; i < count && i < maxPreview; i++ {
			previews = append(previews, posts[i].Subject)
		}

		// 게시판 한글명
		boardName := board
		db.Table("g5_board").Select("bo_subject").Where("bo_table = ?", board).Limit(1).Scan(&boardName)
		if boardName == "" {
			boardName = board
		}

		// 3) level=3 구독자 (noti_board_subscribe OFF 제외)
		var subs []string
		db.Table("g5_board_subscribe").Select("mb_id").
			Where("bo_table = ? AND level = 3", board).Pluck("mb_id", &subs)
		if len(subs) == 0 {
			// 구독자 0이어도 커서는 전진 (다음 run 폭증 방지)
			db.Exec("UPDATE g5_board_subscribe_digest_cursor SET last_wr_id=?, updated_at=? WHERE bo_table=?",
				newCursor, now, board)
			continue
		}
		offSet := map[string]bool{}
		var off []string
		db.Table("g5_noti_preference").Select("mb_id").
			Where("noti_board_subscribe = 0 AND mb_id IN ?", subs).Pluck("mb_id", &off)
		for _, o := range off {
			offSet[o] = true
		}

		msg := fmt.Sprintf("%s 새 글 %d건: %s", boardName, count, strings.Join(previews, ", "))
		if count > len(previews) {
			msg += " 외"
		}

		for _, sid := range subs {
			if offSet[sid] {
				continue
			}
			noti := &gnurepo.Notification{
				PhToCase:      "subscribe",
				PhFromCase:    "digest",
				BoTable:       board,
				WrID:          newCursor,
				MbID:          sid,
				RelMsg:        msg,
				RelURL:        fmt.Sprintf("/%s", board),
				PhReaded:      "N",
				PhDatetime:    now,
				ParentSubject: boardName,
				WrParent:      newCursor,
			}
			if err := db.Create(noti).Error; err == nil {
				res.NotisCreated++
			}
		}
		res.PostsSummarized += count

		db.Exec("UPDATE g5_board_subscribe_digest_cursor SET last_wr_id=?, updated_at=? WHERE bo_table=?",
			newCursor, now, board)
	}

	return res, nil
}
