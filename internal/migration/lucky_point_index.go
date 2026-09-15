package migration

import "gorm.io/gorm"

// AddLuckyPointDisplayIndex 는 럭키 당첨 금액을 글/댓글 목록에 병합하기 위한
// 배치 조회 인덱스를 추가한다(멱등).
//
// 조회 패턴은 (po_rel_table, po_rel_id, po_rel_action) 등가조건이다. 기존
// g5_point index1 은 mb_id 선두라 이 패턴에 안 맞아, 37만 행 규모에서 풀스캔
// 위험이 있다. 목록당 1쿼리가 그 스캔을 타면 목록 응답이 통째로 느려진다.
//
// ⭐ 존재 시 스킵(addIndexIfMissing) — 운영은 저트래픽 윈도우에 온라인 DDL 로
//
//	선반영하고, 이 스텝은 기록·신규 환경 재현용이다.
func AddLuckyPointDisplayIndex(db *gorm.DB) error {
	return addIndexIfMissing(db, "g5_point", "idx_lucky_display",
		"CREATE INDEX idx_lucky_display ON g5_point (po_rel_table, po_rel_id, po_rel_action)")
}
