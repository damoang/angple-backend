package gnuboard

// hot_feed_repo.go — 크로스보드 핫(공감) 피드 (GET /api/v2/feed/hot).
//
// 배경: 앱 공감 탭이 쓰던 v1 empathy API 는 g5_write_empathy 테이블이 없어
// 항상 빈 목록을 반환했다(전 사용자 빈 화면). 웹 공감글은 cron 이 만드는 JSON
// 파일이라 재사용할 수 없어, 라이브 SQL 집계로 새로 만든다.
//
// 성능 설계 — 보드별 서브쿼리는 반드시 2단 바운딩으로 풀스캔을 막는다:
//
//	① 내부: (wr_is_comment, wr_id) 인덱스를 역순으로 타 최근 LIMIT 2000 행만 뜬다 — 보드당 스캔 상한.
//	   ⛔ FORCE INDEX (wr_is_comment) 로 고정한다. 힌트가 없으면 옵티마이저가
//	   idx_comment_deleted 를 골라 157만 행 filesort 를 해 이 상한이 무력화된다(2026-09-21 5xx).
//	   wr_datetime 인덱스에 의존하지 않는다(없는 보드가 많다).
//	② 외부: 그 2000행 안에서만 시간 창(hours)·wr_good >= 1 을 필터한다.
//
// ⛔ 캡의 의도: 보드당 「최근 2000글」 밖의 오래된 글은 시간 창 안이어도 후보에서
// 빠진다. 이것은 버그가 아니라 설계다 — 2000글이 시간 창(최대 72h)보다 빨리 도는
// 보드는 free 뿐이고, free 는 그 안의 글이 곧 최신 글이라 실질 손실이 없다.
// 병합 정렬(wr_good DESC)은 바운디드 풀(보드 수 × ≤2000행 중 필터 통과분)에서만 돈다.

import (
	"errors"
	"fmt"
	"strings"

	mysqldrv "github.com/go-sql-driver/mysql"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
)

// hotFeedScanCapPerBoard 는 보드당 후보 스캔 상한(wr_id 역순 최근 N행)이다.
// 이 값 밖의 오래된 글은 시간 창 안이어도 후보에서 제외된다(위 파일 주석 참조).
const hotFeedScanCapPerBoard = 2000

// hotFeedIndexHint 는 보드별 내부 서브쿼리의 인덱스 힌트다. 캡(LIMIT 2000)과 한 쌍이다 —
// 힌트 없이 캡만 두면 옵티마이저가 다른 인덱스를 골라 캡 이전에 전 행을 훑는다.
// 인덱스 (wr_is_comment, wr_id) 는 WHERE wr_is_comment=0 + ORDER BY wr_id DESC 를
// Backward index scan 으로 소화한다. 대상 보드(bo_use_search=1) 124/124 가 보유(2026-09-21).
const hotFeedIndexHint = " FORCE INDEX (wr_is_comment)"

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
//	   SELECT <컬럼들, '{board}' AS board_id> FROM `g5_write_{board}` FORCE INDEX (wr_is_comment)
//	   WHERE wr_is_comment = 0 AND <secret/lock/삭제 필터: mypage 패턴과 동일>
//	   ORDER BY wr_id DESC LIMIT 2000            -- ① 보드당 스캔 상한(인덱스 역순)
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
	return buildHotFeedQueryWithHint(boards, hours, limit, offset, excludeMbIDs, hotFeedIndexHint)
}

// buildHotFeedQueryWithHint 는 buildHotFeedQuery 의 본체다. indexHint 는 보드별 FROM 절 뒤에
// 그대로 붙는다(빈 문자열이면 힌트 없음). 힌트 없는 변형은 FindHotAcrossBoards 의 1176 폴백
// (인덱스 없는 보드가 끼었을 때)에서만 쓴다 — 평상시엔 반드시 힌트가 있어야 한다.
func buildHotFeedQueryWithHint(boards []string, hours, limit, offset int, excludeMbIDs []string, indexHint string) (string, []interface{}) {
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
		//
		// ⛔ FORCE INDEX (wr_is_comment) 필수 — 힌트 없이 두면 옵티마이저가
		//    idx_comment_deleted(wr_is_comment, wr_deleted_at)를 골라 free 에서
		//    157만 행을 훑고 filesort 한다(EXPLAIN rows 1,569,565). 「wr_id 역순 LIMIT 2000
		//    = 보드당 스캔 상한」이라는 이 파일의 설계가 무력화돼 서브쿼리 하나가 4.2초,
		//    UNION 전체 5.2~6초 → 오리진 타임아웃 → CF 엣지 502/520/522 (2026-09-21).
		//    (wr_is_comment, wr_id) 인덱스는 WHERE wr_is_comment=0 + ORDER BY wr_id DESC 를
		//    Backward index scan 으로 그대로 소화한다 — 실측 free 4,149ms → 14.6ms,
		//    UNION 124보드 5,216ms → 414ms. PRIMARY 강제는 작은 보드(promotion·referral·car)
		//    에서 2~4배 손해라 택하지 않았다. 대상 보드 124/124 에 이 인덱스가 있다.
		//    인덱스 없는 보드가 끼면 MySQL 1176 → FindHotAcrossBoards 가 힌트 없이 재시도한다.
		inner := fmt.Sprintf(
			"SELECT wr_id, wr_subject, LEFT(wr_content, 1000) AS wr_content, wr_datetime, wr_10,"+
				" wr_hit, wr_good, wr_comment, mb_id, wr_name, wr_option, '%s' AS board_id"+
				" FROM `g5_write_%s`%s"+
				" WHERE wr_is_comment = 0"+
				" AND (wr_option NOT LIKE '%%secret%%' OR wr_option IS NULL)"+
				" AND (wr_7 IS NULL OR wr_7 != 'lock')"+
				" AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')"+
				" ORDER BY wr_id DESC LIMIT %d",
			slug, slug, indexHint, hotFeedScanCapPerBoard)
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
		// 인덱스 없는 보드가 UNION 에 끼면 MySQL 1176(Key doesn't exist)으로 전체가 실패한다.
		// 대상 124 보드 전부 인덱스가 있어 평상시엔 오지 않는 경로지만, 신설 보드가 인덱스 없이
		// 만들어지면 핫 피드가 통째로 빈 화면이 된다 — 힌트 없이 한 번 재시도해 살린다.
		// (느리더라도 동작이 우선. 재시도가 잦으면 그 보드에 인덱스를 만들어야 한다.)
		if !isMissingIndexErr(err) {
			return nil, false, err
		}
		fallbackSQL, fallbackArgs := buildHotFeedQueryWithHint(slugs, hours, limit+1, offset, excludeMbIDs, "")
		rows = nil
		if err2 := r.db.Raw(fallbackSQL, fallbackArgs...).Scan(&rows).Error; err2 != nil {
			return nil, false, err2
		}
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
}

// isMissingIndexErr 는 FORCE INDEX 대상 인덱스가 테이블에 없을 때의 MySQL 1176
// ("Key 'x' doesn't exist in table 'y'")인지 판정한다. 드라이버 타입 단언이 우선이고,
// 래핑돼 타입을 잃은 경우를 위해 메시지 문자열도 본다.
func isMissingIndexErr(err error) bool {
	if err == nil {
		return false
	}
	var me *mysqldrv.MySQLError
	if errors.As(err, &me) && me.Number == 1176 {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "Error 1176") || strings.Contains(msg, "doesn't exist in table")
}
