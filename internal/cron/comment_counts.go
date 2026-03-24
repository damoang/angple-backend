package cron

import (
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"
)

type SyncVisibleCommentCountsResult struct {
	BoardsChecked int      `json:"boards_checked"`
	BoardsSynced  int      `json:"boards_synced"`
	RowsUpdated   int64    `json:"rows_updated"`
	Errors        int      `json:"errors"`
	ErrorBoards   []string `json:"error_boards,omitempty"`
}

func runSyncVisibleCommentCounts(db *gorm.DB) (*SyncVisibleCommentCountsResult, error) {
	var boards []string
	if err := db.Table("g5_board").
		Select("bo_table").
		Where("bo_table <> ''").
		Scan(&boards).Error; err != nil {
		return nil, err
	}

	result := &SyncVisibleCommentCountsResult{
		BoardsChecked: len(boards),
		ErrorBoards:   make([]string, 0),
	}

	for _, boardID := range boards {
		if strings.TrimSpace(boardID) == "" {
			continue
		}

		rows, err := syncVisibleCommentCountsForBoard(db, boardID)
		if err != nil {
			result.Errors++
			result.ErrorBoards = append(result.ErrorBoards, boardID)
			log.Printf("[Cron:sync-visible-comment-counts] board=%s error=%v", boardID, err)
			continue
		}
		if rows > 0 {
			result.BoardsSynced++
			result.RowsUpdated += rows
		}
	}

	return result, nil
}

func syncVisibleCommentCountsForBoard(db *gorm.DB, boardID string) (int64, error) {
	table := fmt.Sprintf("g5_write_%s", boardID)
	switch db.Dialector.Name() {
	case "sqlite":
		return syncVisibleCommentCountsForBoardSQLite(db, table)
	default:
		return syncVisibleCommentCountsForBoardMySQL(db, table)
	}
}

func syncVisibleCommentCountsForBoardMySQL(db *gorm.DB, table string) (int64, error) {
	query := fmt.Sprintf(`
		UPDATE %[1]s AS p
		LEFT JOIN (
			SELECT wr_parent, COUNT(*) AS visible_count
			FROM %[1]s
			WHERE wr_is_comment = 1 AND wr_deleted_at IS NULL
			GROUP BY wr_parent
		) AS c ON c.wr_parent = p.wr_id
		SET p.wr_comment = COALESCE(c.visible_count, 0)
		WHERE p.wr_is_comment = 0
		  AND COALESCE(p.wr_comment, 0) <> COALESCE(c.visible_count, 0)
	`, table)
	res := db.Exec(query)
	return res.RowsAffected, res.Error
}

func syncVisibleCommentCountsForBoardSQLite(db *gorm.DB, table string) (int64, error) {
	query := fmt.Sprintf(`
		UPDATE %[1]s
		SET wr_comment = (
			SELECT COUNT(*)
			FROM %[1]s AS c
			WHERE c.wr_parent = %[1]s.wr_id
			  AND c.wr_is_comment = 1
			  AND c.wr_deleted_at IS NULL
		)
		WHERE wr_is_comment = 0
		  AND COALESCE(wr_comment, 0) <> COALESCE((
			SELECT COUNT(*)
			FROM %[1]s AS c
			WHERE c.wr_parent = %[1]s.wr_id
			  AND c.wr_is_comment = 1
			  AND c.wr_deleted_at IS NULL
		), 0)
	`, table)
	res := db.Exec(query)
	return res.RowsAffected, res.Error
}
