package gnuboard

import (
	"fmt"
	"log"
	"sort"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
)

// 본인이 삭제한 글·댓글 목록 (bug/13341).
//
// 제보자 요구: "작성자는 자기가 작성한 글 리스트는 모두 알 수 있어야 한다.
// 편의를 위해 전체 게시물을 삭제한 글과 남은 글로 분리해서 볼 수 있게 해 달라."
//
// 그래서 기존 목록(생존글)은 그대로 두고 **삭제글 전용 목록**을 따로 낸다.
// 남의 화면에서는 절대 호출되지 않는다 — 핸들러가 세션의 mb_id 로만 조회한다.
//
// ⛔ member_activity_feed 의 is_deleted 는 쓰지 않는다. 댓글 동기화가 오래
//    어긋나 있었다(실측: 삭제 1,520건 중 피드 반영 4건). 정본 g5_write_* 만 본다.
//    보드 수만큼 쿼리가 나가지만 (mb_id, wr_is_comment) 인덱스를 타고,
//    "삭제글 보기"는 상시 화면이 아니라 명시적 조회다.

// deletedCondFor 는 "삭제됨" 조건을 **별칭까지 붙여** 만든다.
//
// ⛔ 예전에는 상수였다:
//
//	const deletedCond = "wr_deleted_at IS NOT NULL AND wr_deleted_at <> '0000-00-00...'"
//
//	이 상수는 wr_deleted_at 을 **두 번** 참조한다. 호출부가 self-join 쿼리에서
//	`"AND c." + deletedCond` 로 붙이면 **앞쪽에만 별칭이 붙는다:**
//	  AND c.wr_deleted_at IS NOT NULL AND wr_deleted_at <> '0000-00-00...'
//	                                     ^^^^^^^^^^^^^ 미한정 → MySQL 1052 ambiguous
//
// ⭐ 2026-08-22 bug/13675 실측 재현:
//
//	같은 쿼리에서 미한정 → ERROR 1052 / 둘 다 c. 한정 → 15,080건.
//	COUNT 쿼리는 join 이 없어 **성공**했다 → total 은 채워지고 목록만 0건.
//	그래서 화면에는 "삭제한 댓글 N건"이라 떠 있는데 리스트가 비어 있었다.
//
// ⛔ 문자열 이어붙이기로 별칭을 다는 한 이 실수는 반드시 재발한다.
//
//	별칭을 **인자로 받아 모든 참조에 붙인다** — 한 곳만 빠뜨리는 것이 불가능해진다.
//	alias 가 빈 문자열이면 한정자 없이(단일 테이블 쿼리용) 만든다.
func deletedCondFor(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return fmt.Sprintf(
		"%swr_deleted_at IS NOT NULL AND %swr_deleted_at <> '0000-00-00 00:00:00'",
		prefix, prefix,
	)
}

// FindDeletedPostsByMember 는 본인이 삭제한 글을 최신순으로 돌려준다.
func (r *myPageRepository) FindDeletedPostsByMember(mbID string, page, limit int) ([]gnuboard.MyPost, int64, error) {
	boards := r.getActiveBoards()
	if len(boards) == 0 {
		return nil, 0, nil
	}

	var all []gnuboard.MyPost
	var total int64
	for _, boardID := range boards {
		table := fmt.Sprintf("g5_write_%s", boardID)

		var cnt int64
		if err := r.db.Raw(
			fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0 AND %s", table, deletedCondFor("")),
			mbID,
		).Scan(&cnt).Error; err != nil || cnt == 0 {
			// ⛔ 여기서 err 와 cnt==0 을 한 덩어리로 넘긴다. 0건은 정상이지만 err 는 아니다.
			//    조회가 깨져도 그 보드가 조용히 빠져 total 이 실제보다 작아진다.
			if err != nil {
				log.Printf("[mypage] 삭제글 COUNT 실패 board=%s: %v — 이 보드가 집계에서 빠진다", boardID, err)
			}
			continue
		}
		total += cnt

		var rows []gnuboard.MyPost
		// 페이지 조립을 위해 보드별로 (page*limit) 만큼만 가져와 뒤에서 합쳐 자른다.
		if err := r.db.Raw(
			fmt.Sprintf(
				"SELECT wr_id, wr_subject, wr_content, wr_hit, wr_good, wr_nogood, wr_comment,"+
					" wr_datetime, mb_id, wr_name, wr_option, wr_file, '%s' as board_id,"+
					" wr_deleted_at AS deleted_at FROM `%s`"+
					" WHERE mb_id = ? AND wr_is_comment = 0 AND %s"+
					" ORDER BY wr_datetime DESC LIMIT %d",
				boardID, table, deletedCondFor(""), page*limit,
			), mbID,
		).Scan(&rows).Error; err != nil {
			// ⛔ 삼키면 total 은 채워지고 목록만 비는 조합이 된다 — 화면에 "N건"이라 떠 있는데
			//    리스트가 비어 사용자는 원인을 알 수 없고, 서버 지표로도 안 잡힌다(bug/13675).
			log.Printf("[mypage] 삭제글 조회 실패 board=%s: %v — 목록에서 빠진다", boardID, err)
			continue
		}
		all = append(all, rows...)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].WrDatetime.After(all[j].WrDatetime) })
	return paginateSlice(all, page, limit), total, nil
}

// FindDeletedCommentsByMember 는 본인이 삭제한 댓글을 최신순으로 돌려준다.
func (r *myPageRepository) FindDeletedCommentsByMember(mbID string, page, limit int) ([]gnuboard.MyCommentRow, int64, error) {
	boards := r.getActiveBoards()
	if len(boards) == 0 {
		return nil, 0, nil
	}

	var all []gnuboard.MyCommentRow
	var total int64
	for _, boardID := range boards {
		table := fmt.Sprintf("g5_write_%s", boardID)

		var cnt int64
		if err := r.db.Raw(
			fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE mb_id = ? AND wr_is_comment = 1 AND %s", table, deletedCondFor("")),
			mbID,
		).Scan(&cnt).Error; err != nil || cnt == 0 {
			if err != nil {
				log.Printf("[mypage] 삭제댓글 COUNT 실패 board=%s: %v — 이 보드가 집계에서 빠진다", boardID, err)
			}
			continue
		}
		total += cnt

		var rows []gnuboard.MyCommentRow
		if err := r.db.Raw(
			fmt.Sprintf(
				"SELECT c.wr_id, c.wr_content, c.wr_datetime, c.mb_id, c.wr_name, c.wr_parent,"+
					" c.wr_good, c.wr_nogood, c.wr_option,"+
					" CASE WHEN p.wr_deleted_at IS NOT NULL THEN '[삭제된 글]'"+
					" ELSE COALESCE(p.wr_subject, '') END as post_title,"+
					" '%s' as board_id, c.wr_deleted_at AS deleted_at,"+
					" p.wr_deleted_at AS parent_deleted_at"+
					" FROM `%s` c LEFT JOIN `%s` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0"+
					" WHERE c.mb_id = ? AND c.wr_is_comment = 1 AND %s"+
					" ORDER BY c.wr_datetime DESC LIMIT %d",
				boardID, table, table, deletedCondFor("c"), page*limit,
			), mbID,
		).Scan(&rows).Error; err != nil {
			// ⛔ bug/13675 가 정확히 여기서 침묵했다.
			//    ambiguous column(1052)으로 매 보드가 실패했는데 로그 한 줄 없어,
			//    "삭제한 댓글 1,508건"이라 떠 있는 화면에 리스트만 0건이었다.
			//    제보가 없었으면 못 찾았다.
			log.Printf("[mypage] 삭제댓글 조회 실패 board=%s: %v — 목록에서 빠진다", boardID, err)
			continue
		}
		all = append(all, rows...)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].WrDatetime.After(all[j].WrDatetime) })
	return paginateSlice(all, page, limit), total, nil
}

// paginateSlice 는 합쳐진 결과에서 요청 페이지 구간만 잘라낸다.
func paginateSlice[T any](items []T, page, limit int) []T {
	start := (page - 1) * limit
	if start >= len(items) {
		return nil
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
