package migration

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// WidenCommentReplyColumns widens g5_write_* wr_comment_reply from varchar(5) to varchar(10).
//
// varchar(5)는 댓글 계층을 최대 5단계로 제한하는데, sql_mode 가 비엄격이면 깊이 6+ 답글의
// 계층 문자열("AAAAAA")이 조용히 5자로 잘려 부모와 같은 계층(형제)으로 강등된다
// (economy/77128 사례). varchar(5→10)은 두 길이 모두 1-byte length prefix 라
// ALGORITHM=INPLACE, LOCK=NONE 의 metadata-only 변경으로 무중단이다.
// idempotent: COLUMN_TYPE='varchar(5)' 인 테이블만 대상이라 재실행 시 no-op.
func WidenCommentReplyColumns(db *gorm.DB) error {
	var tables []string
	if err := db.Raw(`
		SELECT TABLE_NAME FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		AND TABLE_NAME LIKE 'g5_write_%'
		AND COLUMN_NAME = 'wr_comment_reply'
		AND COLUMN_TYPE = 'varchar(5)'
	`).Scan(&tables).Error; err != nil {
		return fmt.Errorf("failed to list wr_comment_reply varchar(5) tables: %w", err)
	}
	if len(tables) == 0 {
		return nil
	}

	log.Printf("[Migration] Widening wr_comment_reply varchar(5)→varchar(10) on %d tables", len(tables))
	successCount := 0
	for _, table := range tables {
		// INPLACE+LOCK=NONE 강제 — 조건 미충족 시 에러로 중단시켜 copy 알고리즘 실행을 막는다.
		sql := fmt.Sprintf("ALTER TABLE `%s` MODIFY wr_comment_reply varchar(10) NOT NULL, ALGORITHM=INPLACE, LOCK=NONE", table)
		if err := db.Exec(sql).Error; err != nil {
			log.Printf("[Migration] Warning: failed to widen wr_comment_reply on %s: %v", table, err)
			continue
		}
		successCount++
	}
	log.Printf("[Migration] Widened wr_comment_reply on %d/%d tables", successCount, len(tables))
	return nil
}
