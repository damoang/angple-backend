package gnuboard

import (
	"strconv"

	"gorm.io/gorm"
)

// luckyRelAction 은 럭키 당첨 포인트 기록의 po_rel_action 값이다.
const luckyRelAction = "@lucky"

// LuckyPointsByWrID 는 한 게시판(slug)의 wr_id 목록에 대해 럭키 당첨 금액을
// 한 번의 쿼리로 조회해 map[wrID]amount 로 돌려준다. 당첨 기록이 없는 wr_id 는
// 맵에 없다(호출부에서 0 으로 폴백).
//
// 소스는 g5_point (po_rel_action='@lucky') 다 — 2026-03-06 이전 레거시 당첨
// 37만 건까지 포함하려면 이 원장을 직접 봐야 한다. po_rel_table 은 게시판 슬러그,
// po_rel_id 는 wr_id(문자열), 금액은 po_point 다. 글·댓글은 같은 g5_write_{slug}
// 에서 wr_id 가 유일하므로 이 조회 하나로 둘 다 커버한다.
//
// ⭐ MAX(po_point) — 레거시 데이터에 같은 wr_id 로 중복 당첨 행이 있을 수 있어
//
//	최댓값으로 방어한다(대부분 1행이라 결과 동일).
//
// ⚠️ 빈 목록이면 쿼리를 치지 않는다(목록당 정확히 1쿼리, per-item 반복 금지).
// (po_rel_table, po_rel_id, po_rel_action) 인덱스(idx_lucky_display)를 탄다.
func LuckyPointsByWrID(db *gorm.DB, slug string, wrIDs []int) (map[int]int, error) {
	out := make(map[int]int)
	if db == nil || slug == "" || len(wrIDs) == 0 {
		return out, nil
	}
	// po_rel_id 는 VARCHAR 라 문자열로 비교해 인덱스를 온전히 탄다.
	seen := make(map[int]struct{}, len(wrIDs))
	ids := make([]string, 0, len(wrIDs))
	for _, id := range wrIDs {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, strconv.Itoa(id))
	}
	if len(ids) == 0 {
		return out, nil
	}

	var rows []struct {
		RelID  string `gorm:"column:po_rel_id"`
		Amount int    `gorm:"column:amt"`
	}
	if err := db.Table("g5_point").
		Select("po_rel_id, MAX(po_point) AS amt").
		Where("po_rel_action = ? AND po_rel_table = ? AND po_rel_id IN ?", luckyRelAction, slug, ids).
		Group("po_rel_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		wrID, err := strconv.Atoi(r.RelID)
		if err != nil {
			continue
		}
		out[wrID] = r.Amount
	}
	return out, nil
}
