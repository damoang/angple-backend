package gnuboard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
)

/*
내 글·내 댓글 검색 (bug/12292 · 사용자 요청 2026-08-05).

## 왜 별도 경로인가

검색 없는 목록(FindPostsByMember)은 **활성 게시판 122개에 fan-out** 한다. 거기에 LIKE 를
얹으면 `LIMIT 20` 조기 종료가 사라져 그 회원의 행을 전부 훑게 된다. 실측:

	댓글 28,068행(free 한 곳)  228~242 ms
	글    2,702행(free 한 곳)   81~121 ms

이 비용이 122개 게시판에 얹힌다. 반면 member_activity_feed 는 인덱스
(member_id, activity_type, is_deleted, ...)로 그 회원 5,100행만 훑어 **10ms 미만, 쿼리 1회**다.
20~60배 싸다. 그래서 검색일 때만 피드로 후보를 찾는다.

## ⛔ 피드가 준 ID 를 그대로 믿지 않는다 (#13109)

피드에는 write_table='v2_posts' 인 행이 섞여 있고, 그 write_id 는 g5_write_* 의 wr_id 와
무관한 번호다. 검증 없이 조회하면 **번호가 우연히 겹치는 남의 글이 내 글로 나온다.**
그래서 (1) 후보 단계에서 write_table 을 g5_write_* 로 한정하고,
(2) 본문 조회 단계에서 mb_id·wr_is_comment 를 정본에서 다시 확인한다.
기존 fan-out 의 largeBoardID 경로가 이미 같은 방어를 한다 — 같은 규칙을 따른다.

## 알려진 한계 (화면에 명시한다)

  - content_preview 는 255자다. 긴 글의 뒷부분은 찾히지 않는다.
  - 피드의 댓글 행이 실제 대비 0.02~0.25% 누락돼 있다(표본 3명 실측).
  - 그래서 검색 결과 건수는 검색 없는 목록의 건수와 완전히 일치하지 않을 수 있다.

이미지·영상만 있는 글은 content_preview 가 비어 있는데(전체의 2~8%), 이는 결손이 아니라
검색할 텍스트가 애초에 없는 것이다. 제목은 채워져 있어 제목으로는 찾힌다.
*/

// minSearchLen 미만이면 검색어를 무시하고 전체 목록을 준다.
// 1자 검색은 사실상 전체 매칭이라 인덱스로 좁힌 의미가 사라진다.
const minSearchLen = 2

// maxSearchLen 초과는 잘라낸다. 긴 문자열은 매칭될 리도 없고 로그만 더럽힌다.
const maxSearchLen = 50

// escapeLike 는 LIKE 와일드카드를 무력화한다.
//
// ⛔ 파라미터 바인딩은 SQL 인젝션은 막지만 **와일드카드는 막지 못한다.**
// 이스케이프하지 않으면 회원이 `%` 를 검색할 때 자기 글 전체가 나오고,
// `_` 는 아무 한 글자에나 걸린다. 쿼리에 ESCAPE '\\' 를 함께 준다.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// NormalizeSearchQuery 는 검색어를 다듬는다. 반환값이 빈 문자열이면 "검색 없음"이다.
func NormalizeSearchQuery(q string) string {
	q = strings.TrimSpace(q)
	if len([]rune(q)) < minSearchLen {
		return ""
	}
	if r := []rune(q); len(r) > maxSearchLen {
		q = string(r[:maxSearchLen])
	}
	return q
}

// feedHit 은 피드에서 찾은 후보 한 건 (게시판 + 글/댓글 번호).
type feedHit struct {
	BoardID string `gorm:"column:board_id"`
	WriteID int    `gorm:"column:write_id"`
}

// searchFeed 는 피드에서 후보와 총건수를 찾는다. activityType 1=글, 2=댓글.
func (r *myPageRepository) searchFeed(
	mbID, q string, activityType, page, limit int, extraCols string,
) ([]feedHit, int64, error) {
	like := "%" + escapeLike(q) + "%"

	// ⛔ write_table 한정이 #13109 방어의 1단계다.
	where := "member_id = ? AND activity_type = ? AND is_deleted = 0 AND write_table LIKE 'g5\\_write\\_%'"
	args := []any{mbID, activityType}

	where += " AND (" + extraCols + ")"
	// extraCols 안의 ? 개수만큼 like 를 채운다 — 호출부가 개수를 정한다.
	for i := 0; i < strings.Count(extraCols, "?"); i++ {
		args = append(args, like)
	}

	var total int64
	if err := r.db.Raw(
		"SELECT COUNT(*) FROM member_activity_feed WHERE "+where, args...,
	).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	offset := (page - 1) * limit
	var hits []feedHit
	// 정렬은 검색 없는 목록과 같은 기준(최신순)을 쓴다.
	if err := r.db.Raw(
		"SELECT board_id, write_id FROM member_activity_feed WHERE "+where+
			" ORDER BY source_created_at DESC, id DESC LIMIT ? OFFSET ?",
		append(args, limit, offset)...,
	).Scan(&hits).Error; err != nil {
		return nil, 0, err
	}
	return hits, total, nil
}

// groupByBoard 는 후보를 게시판별로 묶는다. 한 페이지는 limit 건이라 게시판 수도 그 이하다.
func groupByBoard(hits []feedHit) map[string][]int {
	m := make(map[string][]int, 4)
	for _, h := range hits {
		// 테이블명에 들어갈 값이라 형식을 지킨다 (같은 파일군의 activityBoardSlugRe 재사용)
		if !activityBoardSlugRe.MatchString(h.BoardID) {
			continue // 테이블명에 들어갈 값이라 형식을 지킨다
		}
		m[h.BoardID] = append(m[h.BoardID], h.WriteID)
	}
	return m
}

// 아래 두 메서드가 MyPageRepository 를 만족하는지 컴파일 시점에 확인한다.
var _ interface {
	SearchPostsByMember(mbID, q string, page, limit int) ([]gnuboard.MyPost, int64, error)
	SearchCommentsByMember(mbID, q string, page, limit int) ([]gnuboard.MyCommentRow, int64, error)
} = (*myPageRepository)(nil)

// SearchPostsByMember 는 내 글을 제목·본문 앞부분에서 찾는다.
func (r *myPageRepository) SearchPostsByMember(
	mbID, q string, page, limit int,
) ([]gnuboard.MyPost, int64, error) {
	hits, total, err := r.searchFeed(mbID, q, 1, page, limit, "title LIKE ? OR content_preview LIKE ?")
	if err != nil || total == 0 {
		return nil, total, err
	}

	var posts []gnuboard.MyPost
	for boardID, ids := range groupByBoard(hits) {
		var rows []gnuboard.MyPost
		// ⛔ mb_id·wr_is_comment 재확인 — #13109 방어의 2단계.
		r.db.Raw(
			fmt.Sprintf(
				"SELECT wr_id, wr_subject, wr_content, wr_hit, wr_good, wr_nogood, wr_comment,"+
					" wr_datetime, mb_id, wr_name, wr_option, wr_file, '%s' as board_id,"+
					" wr_deleted_at AS deleted_at FROM `g5_write_%s`"+
					" WHERE wr_id IN ? AND mb_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL",
				boardID, boardID,
			), ids, mbID,
		).Scan(&rows)
		posts = append(posts, rows...)
	}

	sort.Slice(posts, func(i, j int) bool { return posts[i].WrDatetime.After(posts[j].WrDatetime) })
	return posts, total, nil
}

// SearchCommentsByMember 는 내 댓글을 내용·원글 제목에서 찾는다.
//
// 원글 제목까지 찾는 이유: 댓글에는 제목이 없어서, 사람은 "그 글에 뭐라고 썼더라"로
// 기억한다. 원글 제목으로 찾을 수 있어야 실제로 쓸모가 있다.
func (r *myPageRepository) SearchCommentsByMember(
	mbID, q string, page, limit int,
) ([]gnuboard.MyCommentRow, int64, error) {
	hits, total, err := r.searchFeed(mbID, q, 2, page, limit, "content_preview LIKE ? OR parent_title LIKE ?")
	if err != nil || total == 0 {
		return nil, total, err
	}

	var comments []gnuboard.MyCommentRow
	for boardID, ids := range groupByBoard(hits) {
		var rows []gnuboard.MyCommentRow
		r.db.Raw(
			fmt.Sprintf(
				"SELECT c.wr_id, c.wr_content, c.wr_datetime, c.mb_id, c.wr_name, c.wr_parent,"+
					" c.wr_good, c.wr_nogood, c.wr_option,"+
					" CASE WHEN p.wr_deleted_at IS NOT NULL THEN '[삭제된 글]'"+
					" ELSE COALESCE(p.wr_subject, '') END as post_title,"+
					" '%s' as board_id, c.wr_deleted_at AS deleted_at,"+
					" p.wr_deleted_at AS parent_deleted_at"+
					" FROM `g5_write_%s` c"+
					" LEFT JOIN `g5_write_%s` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0"+
					" WHERE c.wr_id IN ? AND c.mb_id = ? AND c.wr_is_comment = 1 AND c.wr_deleted_at IS NULL",
				boardID, boardID, boardID,
			), ids, mbID,
		).Scan(&rows)
		comments = append(comments, rows...)
	}

	sort.Slice(comments, func(i, j int) bool {
		return comments[i].WrDatetime.After(comments[j].WrDatetime)
	})
	return comments, total, nil
}
