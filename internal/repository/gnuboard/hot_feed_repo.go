package gnuboard

// hot_feed_repo.go — 크로스보드 핫(공감) 피드 (GET /api/v2/feed/hot).
//
// 배경: 앱 공감 탭이 쓰던 v1 empathy API 는 g5_write_empathy 테이블이 없어
// 항상 빈 목록을 반환했다(전 사용자 빈 화면). 웹 공감글은 cron 이 만드는 JSON
// 파일이라 재사용할 수 없어, 라이브 SQL 집계로 새로 만든다.
//
// 성능 설계 — 보드별 서브쿼리는 반드시 2단 바운딩으로 풀스캔을 막는다:
//
//	① 내부: PK(wr_id) 역순으로 최근 LIMIT 2000 행만 뜬다 — 보드당 스캔 상한.
//	   wr_datetime 인덱스에 의존하지 않는다(없는 보드가 많다).
//	② 외부: 그 2000행 안에서만 시간 창(hours)·wr_good >= 1 을 필터한다.
//
// ⛔ 캡의 의도: 보드당 「최근 2000글」 밖의 오래된 글은 시간 창 안이어도 후보에서
// 빠진다. 이것은 버그가 아니라 설계다 — 2000글이 시간 창(최대 72h)보다 빨리 도는
// 보드는 free 뿐이고, free 는 그 안의 글이 곧 최신 글이라 실질 손실이 없다.
// 병합 정렬(wr_good DESC)은 바운디드 풀(보드 수 × ≤2000행 중 필터 통과분)에서만 돈다.

import (
	"fmt"
	"strings"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
)

// hotFeedScanCapPerBoard 는 보드당 후보 스캔 상한(PK 역순 최근 N행)이다.
// 이 값 밖의 오래된 글은 시간 창 안이어도 후보에서 제외된다(위 파일 주석 참조).
const hotFeedScanCapPerBoard = 2000

// hotFeedDefaultHours 는 hours 파라미터의 기본값이자, 허용 밖 값의 폴백이다.
const hotFeedDefaultHours = 24

// hotFeedAllowedHours 는 허용된 시간 창이다. 임의 값을 받으면 쿼리 플랜·캐시 키가
// 무한히 갈라지므로 화이트리스트로 고정한다.
var hotFeedAllowedHours = map[int]bool{6: true, 12: true, 24: true, 72: true}

// NormalizeHotHours 는 hours 를 허용값(6/12/24/72)으로 정규화한다.
// 허용 밖 값(0·음수 포함)은 기본 24 로 떨어진다. 핸들러(meta 에코)와
// 쿼리 빌더가 같은 판정을 쓰도록 여기 한 곳에 둔다.
func NormalizeHotHours(hours int) int {
	if hotFeedAllowedHours[hours] {
		return hours
	}
	return hotFeedDefaultHours
}

// buildHotFeedQuery 는 크로스보드 핫 피드 SQL 과 바인딩 인자를 만든다.
//
// boards 는 반드시 GetSearchableBoards 검증 목록의 slug 여야 한다(인젝션 방지).
// 방어적으로 activityBoardSlugRe 재검증에 실패한 slug 는 조용히 건너뛴다.
//
// 보드별 분기(2단 바운딩):
//
//	(SELECT * FROM (
//	   SELECT <컬럼들, '{board}' AS board_id> FROM `g5_write_{board}`
//	   WHERE wr_is_comment = 0 AND <secret/lock/삭제 필터: mypage 패턴과 동일>
//	   ORDER BY wr_id DESC LIMIT 2000            -- ① 보드당 스캔 상한(PK 역순)
//	 ) t{i}
//	 WHERE t{i}.wr_datetime >= DATE_SUB(NOW(), INTERVAL ? HOUR)  -- ② 창은 캡 안에서만
//	   AND t{i}.wr_good >= 1 [AND t{i}.mb_id NOT IN ?])
//
// 외부: ORDER BY wr_good DESC, wr_id DESC LIMIT ? OFFSET ?
// (wr_id 는 보드 간 범위가 달라 동률(wr_good) 타이브레이크로만 쓴다.)
//
// limit 은 호출부가 has_more 판정을 위해 +1 해서 넘긴다(FindHotAcrossBoards 참조).
// 보드가 하나도 남지 않으면 ("", nil) 을 돌려준다 — 호출부가 빈 결과로 처리한다.
func buildHotFeedQuery(boards []string, hours, limit, offset int, excludeMbIDs []string) (string, []interface{}) {
	hours = NormalizeHotHours(hours)

	var branches []string
	var args []interface{}
	i := 0
	for _, slug := range boards {
		if slug == "" || !activityBoardSlugRe.MatchString(slug) {
			continue
		}
		alias := fmt.Sprintf("t%d", i)
		i++
		// #nosec G201 -- slug 는 GetSearchableBoards(정본 g5_board) 목록이며
		//               activityBoardSlugRe 로 재검증됐다.
		inner := fmt.Sprintf(
			"SELECT wr_id, wr_subject, LEFT(wr_content, 1000) AS wr_content, wr_datetime, wr_10,"+
				" wr_hit, wr_good, wr_comment, mb_id, wr_name, wr_option, '%s' AS board_id"+
				" FROM `g5_write_%s`"+
				" WHERE wr_is_comment = 0"+
				" AND (wr_option NOT LIKE '%%secret%%' OR wr_option IS NULL)"+
				" AND (wr_7 IS NULL OR wr_7 != 'lock')"+
				" AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')"+
				" ORDER BY wr_id DESC LIMIT %d",
			slug, slug, hotFeedScanCapPerBoard)
		outer := fmt.Sprintf(
			"(SELECT * FROM (%s) %s WHERE %s.wr_datetime >= DATE_SUB(NOW(), INTERVAL ? HOUR) AND %s.wr_good >= 1",
			inner, alias, alias, alias)
		args = append(args, hours)
		if len(excludeMbIDs) > 0 {
			outer += fmt.Sprintf(" AND %s.mb_id NOT IN ?", alias)
			args = append(args, excludeMbIDs)
		}
		outer += ")"
		branches = append(branches, outer)
	}
	if len(branches) == 0 {
		return "", nil
	}

	sql := fmt.Sprintf(
		"SELECT * FROM (%s) AS hot ORDER BY wr_good DESC, wr_id DESC LIMIT ? OFFSET ?",
		strings.Join(branches, " UNION ALL "))
	args = append(args, limit, offset)
	return sql, args
}

// FindHotAcrossBoards 는 검색가능 게시판 전체에서 최근 hours 시간 내
// 공감(wr_good) 1 이상 글을 공감순으로 모아 offset 페이지네이션으로 돌려준다.
// 두 번째 반환값은 has_more(다음 페이지 존재) 판정이다 — limit+1 조회로 확인한다.
//
// slug 목록은 GetSearchableBoards(bo_use_search=1, 캐시 5분)만 쓴다.
// 관리자·징계 게시판(adm/disciplinelog/…)이 새지 않고, 실패 시 fail-closed 로
// error 를 올린다(빈 필터로 진행하지 않는다).
//
// ⛔ 캡 문서화: 보드당 최근 hotFeedScanCapPerBoard(2000)글 밖의 오래된 글은
// 시간 창 안이어도 후보에서 제외된다(의도 — 파일 상단 주석 참조).
func (r *myPageRepository) FindHotAcrossBoards(hours, limit, offset int, excludeMbIDs []string) ([]gnuboard.FeedPost, bool, error) {
	if limit <= 0 {
		return nil, false, nil
	}
	if offset < 0 {
		offset = 0
	}
	boards, err := r.GetSearchableBoards()
	if err != nil {
		return nil, false, err
	}
	slugs := make([]string, 0, len(boards))
	for _, b := range boards {
		slugs = append(slugs, b.BoTable)
	}

	// has_more 판정용으로 한 행 더 뜬다.
	sql, args := buildHotFeedQuery(slugs, hours, limit+1, offset, excludeMbIDs)
	if sql == "" {
		return nil, false, nil
	}

	var rows []gnuboard.FeedPost
	if err := r.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
}
