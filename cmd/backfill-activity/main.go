package main

import (
	"flag"
	"fmt"
	"html"
	"log"
	"regexp"
	"strings"

	"github.com/damoang/angple-backend/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var (
	reHTMLTag = regexp.MustCompile(`<[^>]*>`)
	reEmoTag  = regexp.MustCompile(`\{emo:[^}]+\}`)
	reMultiWS = regexp.MustCompile(`\s+`)
)

func main() {
	configPath := flag.String("config", "configs/config.dev.yaml", "config file path")
	dryRun := flag.Bool("dry-run", false, "show counts without writing")
	batchSize := flag.Int("batch-size", 500, "batch insert size")
	verbose := flag.Bool("verbose", false, "verbose SQL logging")
	skipLegacy := flag.Bool("skip-legacy", false, "skip g5_write_* backfill")
	skipV2 := flag.Bool("skip-v2", false, "skip v2_posts/v2_comments backfill")
	flag.Parse()

	loaded := config.LoadDotEnv()
	if len(loaded) > 0 {
		log.Printf("Loaded env files: %v", loaded)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logLevel := gormlogger.Warn
	if *verbose {
		logLevel = gormlogger.Info
	}

	db, err := gorm.Open(mysql.Open(cfg.Database.GetDSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(logLevel),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	if !*skipLegacy {
		backfillLegacy(db, *dryRun, *batchSize)
	}
	if !*skipV2 {
		backfillV2Posts(db, *dryRun, *batchSize)
		backfillV2Comments(db, *dryRun, *batchSize)
	}

	// Recalculate stats
	if !*dryRun {
		recalcStats(db)
	}

	log.Println("Backfill complete!")
}

// backfillLegacy processes all g5_write_* tables
func backfillLegacy(db *gorm.DB, dryRun bool, batchSize int) {
	log.Println("=== Phase 1: Legacy g5_write_* tables ===")

	// Get searchable boards map
	searchableBoards := getSearchableBoards(db)

	// Get all board tables
	var tables []string
	db.Raw("SELECT bo_table FROM g5_board ORDER BY bo_table").Pluck("bo_table", &tables)
	log.Printf("Found %d boards", len(tables))

	totalPosts := 0
	totalComments := 0

	for _, boardID := range tables {
		tableName := fmt.Sprintf("g5_write_%s", boardID)

		// Check table exists
		var count int64
		if err := db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", tableName).Scan(&count).Error; err != nil || count == 0 {
			continue
		}

		isSearchable := searchableBoards[boardID]

		// Posts: wr_is_comment = 0
		var postCount int64
		db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE wr_is_comment = 0", tableName)).Scan(&postCount)
		if postCount > 0 {
			if dryRun {
				log.Printf("  [dry-run] %s: %d posts", tableName, postCount)
			} else {
				n := backfillLegacyPosts(db, boardID, tableName, isSearchable, batchSize)
				log.Printf("  %s: %d posts upserted", tableName, n)
				totalPosts += n
			}
		}

		// Comments: wr_is_comment > 0
		var commentCount int64
		db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE wr_is_comment > 0", tableName)).Scan(&commentCount)
		if commentCount > 0 {
			if dryRun {
				log.Printf("  [dry-run] %s: %d comments", tableName, commentCount)
			} else {
				n := backfillLegacyComments(db, boardID, tableName, isSearchable, batchSize)
				log.Printf("  %s: %d comments upserted", tableName, n)
				totalComments += n
			}
		}
	}

	log.Printf("Legacy total: %d posts, %d comments", totalPosts, totalComments)
}

type legacyRow struct {
	WrID        int    `gorm:"column:wr_id"`
	MbID        string `gorm:"column:mb_id"`
	WrSubject   string `gorm:"column:wr_subject"`
	WrContent   string `gorm:"column:wr_content"`
	WrName      string `gorm:"column:wr_name"`
	WrOption    string `gorm:"column:wr_option"`
	WrParent    int    `gorm:"column:wr_parent"`
	WrDatetime  string `gorm:"column:wr_datetime"`
	WrLast      string `gorm:"column:wr_last"`
	WrIsComment int    `gorm:"column:wr_is_comment"`
}

func backfillLegacyPosts(db *gorm.DB, boardID, tableName string, isSearchable bool, batchSize int) int {
	total := 0
	offset := 0
	for {
		var rows []legacyRow
		db.Raw(fmt.Sprintf("SELECT wr_id, mb_id, wr_subject, wr_content, wr_name, wr_option, wr_datetime, wr_last FROM `%s` WHERE wr_is_comment = 0 ORDER BY wr_id LIMIT ? OFFSET ?", tableName), batchSize, offset).Scan(&rows)
		if len(rows) == 0 {
			break
		}

		for _, r := range rows {
			isSecret := strings.Contains(r.WrOption, "secret")
			isPublic := !isSecret && isSearchable

			sql := `INSERT INTO member_activity_feed
				(member_id, board_id, write_table, write_id, activity_type, is_public, is_deleted, title, content_preview, author_name, wr_option, source_created_at, source_updated_at)
			VALUES (?, ?, ?, ?, 1, ?, 0, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				title = VALUES(title), content_preview = VALUES(content_preview),
				author_name = VALUES(author_name), wr_option = VALUES(wr_option),
				is_public = VALUES(is_public), source_updated_at = VALUES(source_updated_at)`

			db.Exec(sql, r.MbID, boardID, tableName, r.WrID,
				boolToInt(isPublic),
				truncate(r.WrSubject, 255),
				stripHTMLPreview(r.WrContent),
				r.WrName, r.WrOption,
				r.WrDatetime, nilIfEmpty(r.WrLast))
		}
		total += len(rows)
		offset += batchSize
	}
	return total
}

func backfillLegacyComments(db *gorm.DB, boardID, tableName string, isSearchable bool, batchSize int) int {
	total := 0
	offset := 0
	for {
		var rows []legacyRow
		db.Raw(fmt.Sprintf(`SELECT c.wr_id, c.mb_id, c.wr_content, c.wr_name, c.wr_option, c.wr_parent, c.wr_datetime, c.wr_last,
			COALESCE(p.wr_subject, '') as wr_subject
			FROM `+"`%s`"+` c LEFT JOIN `+"`%s`"+` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0
			WHERE c.wr_is_comment > 0
			ORDER BY c.wr_id LIMIT ? OFFSET ?`, tableName, tableName), batchSize, offset).Scan(&rows)
		if len(rows) == 0 {
			break
		}

		for _, r := range rows {
			isPublic := isSearchable // comments don't have their own secret flag typically

			sql := `INSERT INTO member_activity_feed
				(member_id, board_id, write_table, write_id, parent_write_id, activity_type, is_public, is_deleted, content_preview, parent_title, author_name, wr_option, source_created_at, source_updated_at)
			VALUES (?, ?, ?, ?, ?, 2, ?, 0, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				content_preview = VALUES(content_preview), parent_title = VALUES(parent_title),
				author_name = VALUES(author_name), wr_option = VALUES(wr_option),
				is_public = VALUES(is_public), source_updated_at = VALUES(source_updated_at)`

			db.Exec(sql, r.MbID, boardID, tableName, r.WrID, r.WrParent,
				boolToInt(isPublic),
				stripHTMLPreview(r.WrContent),
				truncate(r.WrSubject, 255),
				r.WrName, r.WrOption,
				r.WrDatetime, nilIfEmpty(r.WrLast))
		}
		total += len(rows)
		offset += batchSize
	}
	return total
}

// backfillV2Posts processes v2_posts table
func backfillV2Posts(db *gorm.DB, dryRun bool, batchSize int) {
	log.Println("=== Phase 2a: v2_posts ===")

	searchableBoards := getSearchableBoards(db)

	var totalCount int64
	db.Raw("SELECT COUNT(*) FROM v2_posts WHERE status != 'deleted'").Scan(&totalCount)
	log.Printf("v2_posts: %d rows to process", totalCount)

	if dryRun {
		return
	}

	offset := 0
	processed := 0
	for {
		type v2PostRow struct {
			ID        uint64 `gorm:"column:id"`
			BoardID   uint64 `gorm:"column:board_id"`
			UserID    uint64 `gorm:"column:user_id"`
			Title     string `gorm:"column:title"`
			Content   string `gorm:"column:content"`
			IsSecret  bool   `gorm:"column:is_secret"`
			Status    string `gorm:"column:status"`
			CreatedAt string `gorm:"column:created_at"`
			UpdatedAt string `gorm:"column:updated_at"`
			// joined fields
			BoardSlug string `gorm:"column:bo_table"`
			MbID      string `gorm:"column:mb_id"`
			MbNick    string `gorm:"column:mb_nick"`
		}

		var rows []v2PostRow
		db.Raw(`SELECT p.id, p.board_id, p.user_id, p.title, p.content, p.is_secret, p.status, p.created_at, p.updated_at,
			b.slug as bo_table, COALESCE(m.mb_id, '') as mb_id, COALESCE(m.mb_nick, '') as mb_nick
			FROM v2_posts p
			JOIN v2_boards b ON b.id = p.board_id
			LEFT JOIN g5_member m ON m.mb_no = p.user_id
			WHERE p.status != 'deleted'
			ORDER BY p.id LIMIT ? OFFSET ?`, batchSize, offset).Scan(&rows)

		if len(rows) == 0 {
			break
		}

		for _, r := range rows {
			isSearchable := searchableBoards[r.BoardSlug]
			isPublic := !r.IsSecret && isSearchable
			wrOption := ""
			if r.IsSecret {
				wrOption = "secret"
			}

			sql := `INSERT INTO member_activity_feed
				(member_id, board_id, write_table, write_id, activity_type, is_public, is_deleted, title, content_preview, author_name, wr_option, source_created_at, source_updated_at)
			VALUES (?, ?, 'v2_posts', ?, 1, ?, 0, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				title = VALUES(title), content_preview = VALUES(content_preview),
				author_name = VALUES(author_name), wr_option = VALUES(wr_option),
				is_public = VALUES(is_public), source_updated_at = VALUES(source_updated_at)`

			db.Exec(sql, r.MbID, r.BoardSlug, r.ID,
				boolToInt(isPublic),
				truncate(r.Title, 255),
				stripHTMLPreview(r.Content),
				r.MbNick, wrOption,
				r.CreatedAt, nilIfEmpty(r.UpdatedAt))
		}
		processed += len(rows)
		offset += batchSize
		if processed%5000 == 0 {
			log.Printf("  v2_posts: %d/%d processed", processed, totalCount)
		}
	}
	log.Printf("v2_posts: %d upserted", processed)
}

// backfillV2Comments processes v2_comments table
func backfillV2Comments(db *gorm.DB, dryRun bool, batchSize int) {
	log.Println("=== Phase 2b: v2_comments ===")

	searchableBoards := getSearchableBoards(db)

	var totalCount int64
	db.Raw("SELECT COUNT(*) FROM v2_comments WHERE status != 'deleted'").Scan(&totalCount)
	log.Printf("v2_comments: %d rows to process", totalCount)

	if dryRun {
		return
	}

	offset := 0
	processed := 0
	for {
		type v2CommentRow struct {
			ID        uint64  `gorm:"column:id"`
			PostID    uint64  `gorm:"column:post_id"`
			UserID    uint64  `gorm:"column:user_id"`
			Content   string  `gorm:"column:content"`
			Status    string  `gorm:"column:status"`
			CreatedAt string  `gorm:"column:created_at"`
			UpdatedAt string  `gorm:"column:updated_at"`
			ParentID  *uint64 `gorm:"column:parent_id"`
			// joined
			BoardSlug string `gorm:"column:bo_table"`
			PostTitle string `gorm:"column:post_title"`
			MbID      string `gorm:"column:mb_id"`
			MbNick    string `gorm:"column:mb_nick"`
		}

		var rows []v2CommentRow
		db.Raw(`SELECT c.id, c.post_id, c.user_id, c.content, c.status, c.created_at, c.updated_at, c.parent_id,
			b.slug as bo_table, COALESCE(p.title, '') as post_title,
			COALESCE(m.mb_id, '') as mb_id, COALESCE(m.mb_nick, '') as mb_nick
			FROM v2_comments c
			JOIN v2_posts p ON p.id = c.post_id
			JOIN v2_boards b ON b.id = p.board_id
			LEFT JOIN g5_member m ON m.mb_no = c.user_id
			WHERE c.status != 'deleted'
			ORDER BY c.id LIMIT ? OFFSET ?`, batchSize, offset).Scan(&rows)

		if len(rows) == 0 {
			break
		}

		for _, r := range rows {
			isSearchable := searchableBoards[r.BoardSlug]

			sql := `INSERT INTO member_activity_feed
				(member_id, board_id, write_table, write_id, parent_write_id, activity_type, is_public, is_deleted, content_preview, parent_title, author_name, source_created_at, source_updated_at)
			VALUES (?, ?, 'v2_comments', ?, ?, 2, ?, 0, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				content_preview = VALUES(content_preview), parent_title = VALUES(parent_title),
				author_name = VALUES(author_name), is_public = VALUES(is_public),
				source_updated_at = VALUES(source_updated_at)`

			db.Exec(sql, r.MbID, r.BoardSlug, r.ID, r.PostID,
				boolToInt(isSearchable),
				stripHTMLPreview(r.Content),
				truncate(r.PostTitle, 255),
				r.MbNick,
				r.CreatedAt, nilIfEmpty(r.UpdatedAt))
		}
		processed += len(rows)
		offset += batchSize
		if processed%5000 == 0 {
			log.Printf("  v2_comments: %d/%d processed", processed, totalCount)
		}
	}
	log.Printf("v2_comments: %d upserted", processed)
}

// recalcStats recalculates member_activity_stats from member_activity_feed
func recalcStats(db *gorm.DB) {
	log.Println("=== Recalculating member_activity_stats ===")

	if err := db.Exec("TRUNCATE TABLE member_activity_stats").Error; err != nil {
		log.Fatalf("Failed to truncate stats: %v", err)
	}

	sql := `INSERT INTO member_activity_stats (member_id, board_id, post_count, comment_count, public_post_count, public_comment_count)
	SELECT
		member_id,
		board_id,
		SUM(CASE WHEN activity_type = 1 AND is_deleted = 0 THEN 1 ELSE 0 END),
		SUM(CASE WHEN activity_type = 2 AND is_deleted = 0 THEN 1 ELSE 0 END),
		SUM(CASE WHEN activity_type = 1 AND is_deleted = 0 AND is_public = 1 THEN 1 ELSE 0 END),
		SUM(CASE WHEN activity_type = 2 AND is_deleted = 0 AND is_public = 1 THEN 1 ELSE 0 END)
	FROM member_activity_feed
	GROUP BY member_id, board_id`

	result := db.Exec(sql)
	if result.Error != nil {
		log.Fatalf("Failed to recalc stats: %v", result.Error)
	}
	log.Printf("Stats recalculated: %d rows", result.RowsAffected)
}

// getSearchableBoards returns a map of board_id -> bo_use_search=1
func getSearchableBoards(db *gorm.DB) map[string]bool {
	type boardSearch struct {
		BoTable     string `gorm:"column:bo_table"`
		BoUseSearch int    `gorm:"column:bo_use_search"`
	}
	var boards []boardSearch
	db.Raw("SELECT bo_table, bo_use_search FROM g5_board").Scan(&boards)

	result := make(map[string]bool, len(boards))
	for _, b := range boards {
		result[b.BoTable] = b.BoUseSearch == 1
	}
	return result
}

// --- helpers ---

func stripHTMLPreview(s string) string {
	s = reHTMLTag.ReplaceAllString(s, "")
	s = reEmoTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = reMultiWS.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	return truncate(s, 200)
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nilIfEmpty(s string) *string {
	if s == "" || s == "0000-00-00 00:00:00" {
		return nil
	}
	return &s
}
