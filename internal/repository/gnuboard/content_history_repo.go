package gnuboard

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// contentHistoryDatetimeLayout 은 previous_data 스냅샷의 wr_datetime 표기(그누보드 DATETIME) 이다.
const contentHistoryDatetimeLayout = "2006-01-02 15:04:05"

// RecordContentHistory 는 g5_da_content_history 에 감사 이력 한 건을 기록한다.
//
// 반드시 삭제/수정 UPDATE 와 "같은 트랜잭션(tx)" 으로 호출해야 한다. 이력 기록이 실패하면
// 반환된 error 를 상위에서 그대로 return 하여 트랜잭션 전체가 롤백되도록 한다(fail-closed).
// 이렇게 해야 본문은 지워졌는데 원본 이력만 유실되는 증거 누락(사법·KISA 대응)이 0 이 된다.
//
// snapshot 은 통일 스키마(PostHistorySnapshot / CommentHistorySnapshot)로 만든 map 을 넘긴다.
func RecordContentHistory(tx *gorm.DB, boTable string, wrID int, isComment int, mbID, wrName, operation, operatedBy string, operatedAt time.Time, snapshot map[string]any) error {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO g5_da_content_history
		(bo_table, wr_id, wr_is_comment, mb_id, wr_name, operation, operated_by, operated_at, previous_data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		boTable, wrID, isComment, mbID, wrName, operation, operatedBy, operatedAt, string(payload),
	).Error
}

// PostHistorySnapshot 은 게시글의 통일 previous_data 스냅샷 map 을 만든다.
func PostHistorySnapshot(wrSubject, wrContent, wrName, mbID string, wrDatetime time.Time) map[string]any {
	return map[string]any{
		"wr_subject":    wrSubject,
		"wr_content":    wrContent,
		"wr_name":       wrName,
		"mb_id":         mbID,
		"wr_datetime":   wrDatetime.Format(contentHistoryDatetimeLayout),
		"wr_is_comment": 0,
	}
}

// CommentHistorySnapshot 은 댓글의 통일 previous_data 스냅샷 map 을 만든다.
// 댓글에는 제목이 없으므로 wr_subject 는 공백으로 둔다.
func CommentHistorySnapshot(wrContent, wrName, mbID string, wrDatetime time.Time) map[string]any {
	return map[string]any{
		"wr_subject":    "",
		"wr_content":    wrContent,
		"wr_name":       wrName,
		"mb_id":         mbID,
		"wr_datetime":   wrDatetime.Format(contentHistoryDatetimeLayout),
		"wr_is_comment": 1,
	}
}
