package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/config"
	"github.com/damoang/angple-backend/internal/migration"
	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Migration target constants
const (
	targetPosts    = "posts"
	targetComments = "comments"
)

func main() {
	// CLI flags
	configPath := flag.String("config", "configs/config.dev.yaml", "config file path")
	target := flag.String("target", "all", "migration target: all, users, boards, posts, comments, files, scraps, messages, memos, points")
	dryRun := flag.Bool("dry-run", false, "show what would be migrated without executing")
	verify := flag.Bool("verify", false, "verify migration data integrity")
	rollback := flag.Bool("rollback", false, "rollback v2 data (truncate v2 tables)")
	batchSize := flag.Int("batch-size", 1000, "batch insert size")
	verbose := flag.Bool("verbose", false, "verbose SQL logging")
	flag.Parse()

	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
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

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get underlying DB: %v", err)
	}
	defer sqlDB.Close()

	// Ensure v2 schema exists
	if err := migration.RunV2Schema(db); err != nil {
		log.Fatalf("Failed to create v2 schema: %v", err)
	}

	if *rollback {
		runRollback(db)
		return
	}

	if *verify {
		runVerify(db)
		return
	}

	if *dryRun {
		log.Println("[dry-run] Would migrate:", *target)
		runDryRun(db, *target)
		return
	}

	runMigration(db, *target, *batchSize)
}

func runMigration(db *gorm.DB, target string, batchSize int) {
	start := time.Now()
	targets := parseTargets(target)

	for _, t := range targets {
		log.Printf("[migrate] Starting: %s", t)
		tStart := time.Now()

		var err error
		switch t {
		case "users":
			err = migrateUsers(db, batchSize)
		case "boards":
			err = migrateBoards(db)
		case targetPosts:
			err = migratePosts(db, batchSize)
		case targetComments:
			err = migrateComments(db, batchSize)
		case "files":
			err = migrateFiles(db, batchSize)
		case "scraps":
			err = migrateScraps(db, batchSize)
		case "messages":
			err = migrateMessages(db, batchSize)
		case "memos":
			err = migrateMemos(db, batchSize)
		case "points":
			err = migratePoints(db, batchSize)
		default:
			log.Printf("[migrate] Unknown target: %s", t)
			continue
		}

		if err != nil {
			log.Printf("[migrate] FAILED %s: %v", t, err)
			os.Exit(1)
		}
		log.Printf("[migrate] Completed %s in %v", t, time.Since(tStart))
	}

	log.Printf("[migrate] All migrations completed in %v", time.Since(start))
}

func parseTargets(target string) []string {
	if target == "all" {
		return []string{"users", "boards", "posts", "comments", "files", "scraps", "messages", "memos", "points"}
	}
	return strings.Split(target, ",")
}

// --- Migration functions ---

func migrateUsers(db *gorm.DB, _ int) error {
	sql := `
		INSERT IGNORE INTO v2_users (username, email, password, nickname, level, status, bio, created_at, updated_at)
		SELECT
			mb_id,
			CASE WHEN mb_email = '' THEN CONCAT(mb_id, '@legacy.local') ELSE mb_email END,
			mb_password,
			mb_nick,
			LEAST(mb_level, 10),
			CASE
				WHEN mb_leave_date != '' THEN 'inactive'
				WHEN mb_intercept_date != '' THEN 'banned'
				ELSE 'active'
			END,
			NULLIF(mb_profile, ''),
			mb_datetime,
			mb_datetime
		FROM g5_member
		WHERE mb_id != ''
	`
	result := db.Exec(sql)
	if result.Error != nil {
		return result.Error
	}
	log.Printf("[migrate:users] Migrated %d rows", result.RowsAffected)
	return nil
}

func migrateBoards(db *gorm.DB) error {
	sql := `
		INSERT IGNORE INTO v2_boards (slug, name, description, is_active, order_num, created_at, updated_at)
		SELECT
			bo_table,
			bo_subject,
			NULLIF(bo_content_head, ''),
			TRUE,
			bo_order,
			NOW(),
			NOW()
		FROM g5_board
	`
	result := db.Exec(sql)
	if result.Error != nil {
		return result.Error
	}
	log.Printf("[migrate:boards] Migrated %d rows", result.RowsAffected)
	return nil
}

func migratePosts(db *gorm.DB, _ int) error {
	boardIDs, err := getBoardIDs(db)
	if err != nil {
		return err
	}

	var totalRows int64
	for _, boardID := range boardIDs {
		tableName := fmt.Sprintf("g5_write_%s", boardID)
		if !tableExists(db, tableName) {
			continue
		}

		sql := fmt.Sprintf(`
			INSERT IGNORE INTO v2_posts (board_id, user_id, title, content, status, view_count, comment_count, is_notice, created_at, updated_at)
			SELECT
				(SELECT id FROM v2_boards WHERE slug = '%s'),
				COALESCE((SELECT id FROM v2_users WHERE username = w.mb_id LIMIT 1), 1),
				w.wr_subject,
				w.wr_content,
				'published',
				w.wr_hit,
				w.wr_comment,
				CASE WHEN w.wr_option LIKE '%%%%notice%%%%' THEN TRUE ELSE FALSE END,
				w.wr_datetime,
				COALESCE(NULLIF(w.wr_last, ''), w.wr_datetime)
			FROM %s w
			WHERE w.wr_is_comment = 0 AND w.wr_id = w.wr_parent
		`, boardID, tableName)

		result := db.Exec(sql)
		if result.Error != nil {
			log.Printf("[migrate:posts] Warning: %s: %v", tableName, result.Error)
			continue
		}
		totalRows += result.RowsAffected
	}
	log.Printf("[migrate:posts] Migrated %d rows across %d boards", totalRows, len(boardIDs))
	return nil
}

// migratePerBoard iterates all boards and executes a SQL builder function per board.
// sqlBuilder receives (boardID, tableName) and returns the SQL to execute.
func migratePerBoard(db *gorm.DB, label string, sqlBuilder func(boardID, tableName string) string) error {
	boardIDs, err := getBoardIDs(db)
	if err != nil {
		return err
	}

	var totalRows int64
	for _, boardID := range boardIDs {
		tableName := fmt.Sprintf("g5_write_%s", boardID)
		if !tableExists(db, tableName) {
			continue
		}

		result := db.Exec(sqlBuilder(boardID, tableName))
		if result.Error != nil {
			log.Printf("[migrate:%s] Warning: %s: %v", label, boardID, result.Error)
			continue
		}
		totalRows += result.RowsAffected
	}
	log.Printf("[migrate:%s] Migrated %d rows", label, totalRows)
	return nil
}

func migrateComments(db *gorm.DB, _ int) error {
	return migratePerBoard(db, "comments", func(boardID, tableName string) string {
		return fmt.Sprintf(`
			INSERT IGNORE INTO v2_comments (post_id, user_id, content, depth, status, created_at, updated_at)
			SELECT
				p.id,
				COALESCE((SELECT id FROM v2_users WHERE username = w.mb_id LIMIT 1), 1),
				w.wr_content,
				0,
				'active',
				w.wr_datetime,
				w.wr_datetime
			FROM %s w
			JOIN v2_posts p ON p.board_id = (SELECT id FROM v2_boards WHERE slug = '%s')
				AND p.title = (SELECT wr_subject FROM %s WHERE wr_id = w.wr_parent AND wr_is_comment = 0 LIMIT 1)
				AND p.created_at = (SELECT wr_datetime FROM %s WHERE wr_id = w.wr_parent AND wr_is_comment = 0 LIMIT 1)
			WHERE w.wr_is_comment = 1
		`, tableName, boardID, tableName, tableName)
	})
}

func migrateFiles(db *gorm.DB, _ int) error {
	return migratePerBoard(db, "files", func(boardID, tableName string) string {
		return fmt.Sprintf(`
			INSERT IGNORE INTO v2_files (post_id, user_id, original_name, stored_name, mime_type, file_size, storage_path, download_count, created_at)
			SELECT
				p.id,
				COALESCE((SELECT id FROM v2_users WHERE username = w.mb_id LIMIT 1), 1),
				f.bf_source,
				f.bf_file,
				COALESCE(NULLIF(f.bf_content, ''), 'application/octet-stream'),
				f.bf_filesize,
				CONCAT('data/file/%s/', f.bf_file),
				f.bf_download,
				f.bf_datetime
			FROM g5_board_file f
			JOIN %s w ON w.wr_id = f.wr_id AND w.wr_is_comment = 0 AND w.wr_id = w.wr_parent
			JOIN v2_posts p ON p.board_id = (SELECT id FROM v2_boards WHERE slug = '%s')
				AND p.title = w.wr_subject
				AND p.created_at = w.wr_datetime
			WHERE f.bo_table = '%s'
		`, boardID, tableName, boardID, boardID)
	})
}

func migrateScraps(db *gorm.DB, _ int) error {
	sql := `
		INSERT IGNORE INTO v2_scraps (user_id, post_id, created_at)
		SELECT
			COALESCE((SELECT id FROM v2_users WHERE username = s.mb_id LIMIT 1), 1),
			COALESCE((
				SELECT p.id FROM v2_posts p
				JOIN v2_boards b ON p.board_id = b.id AND b.slug = s.bo_table
				WHERE p.id = s.wr_id
				LIMIT 1
			), 0),
			s.ms_datetime
		FROM g5_scrap s
		HAVING post_id > 0
	`
	result := db.Exec(sql)
	if result.Error != nil {
		return result.Error
	}
	log.Printf("[migrate:scraps] Migrated %d rows", result.RowsAffected)
	return nil
}

func migrateMessages(db *gorm.DB, _ int) error {
	sql := `
		INSERT IGNORE INTO v2_messages (sender_id, receiver_id, content, is_read, read_at, created_at)
		SELECT
			COALESCE((SELECT id FROM v2_users WHERE username = m.me_send_mb_id LIMIT 1), 1),
			COALESCE((SELECT id FROM v2_users WHERE username = m.me_recv_mb_id LIMIT 1), 1),
			m.me_memo,
			CASE WHEN m.me_read_datetime IS NOT NULL AND m.me_read_datetime != '0000-00-00 00:00:00' THEN TRUE ELSE FALSE END,
			CASE WHEN m.me_read_datetime IS NOT NULL AND m.me_read_datetime != '0000-00-00 00:00:00' THEN m.me_read_datetime ELSE NULL END,
			m.me_send_datetime
		FROM g5_memo m
		WHERE m.me_type = 'recv'
	`
	result := db.Exec(sql)
	if result.Error != nil {
		return result.Error
	}
	log.Printf("[migrate:messages] Migrated %d rows", result.RowsAffected)
	return nil
}

func migrateMemos(db *gorm.DB, _ int) error {
	// Check if g5_member_memo exists
	if !tableExists(db, "g5_member_memo") {
		log.Println("[migrate:memos] g5_member_memo table not found, skipping")
		return nil
	}

	sql := `
		INSERT IGNORE INTO v2_memos (user_id, target_user_id, content, color, created_at, updated_at)
		SELECT
			COALESCE((SELECT id FROM v2_users WHERE username = m.member_id LIMIT 1), 1),
			COALESCE((SELECT id FROM v2_users WHERE username = m.target_member_id LIMIT 1), 1),
			COALESCE(NULLIF(m.memo_detail, ''), m.memo),
			COALESCE(NULLIF(m.color, ''), 'yellow'),
			m.created_at,
			COALESCE(m.updated_at, m.created_at)
		FROM g5_member_memo m
	`
	result := db.Exec(sql)
	if result.Error != nil {
		return result.Error
	}
	log.Printf("[migrate:memos] Migrated %d rows", result.RowsAffected)
	return nil
}

func migratePoints(db *gorm.DB, _ int) error {
	// v2 doesn't have a dedicated points table yet.
	// Update v2_users point balance from g5_member instead.
	// If a v2_points table is added later, this can be expanded.
	log.Println("[migrate:points] Syncing point balances to v2_users (if point column exists)...")

	// For now, just log g5_point stats
	var count int64
	db.Raw("SELECT COUNT(*) FROM g5_point").Scan(&count)
	log.Printf("[migrate:points] g5_point has %d records (full history migration deferred until v2_points table is added)", count)
	return nil
}

// --- Helpers ---

func getBoardIDs(db *gorm.DB) ([]string, error) {
	var boardIDs []string
	if err := db.Raw("SELECT bo_table FROM g5_board").Scan(&boardIDs).Error; err != nil {
		return nil, err
	}
	return boardIDs, nil
}

func tableExists(db *gorm.DB, tableName string) bool {
	var count int64
	db.Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?", tableName).Scan(&count)
	return count > 0
}

// --- Verify ---

func runVerify(db *gorm.DB) {
	log.Println("[verify] Comparing record counts v1 vs v2...")

	type countPair struct {
		label   string
		v1Query string
		v2Query string
	}

	pairs := []countPair{
		{"users", "SELECT COUNT(*) FROM g5_member WHERE mb_id != ''", "SELECT COUNT(*) FROM v2_users"},
		{"boards", "SELECT COUNT(*) FROM g5_board", "SELECT COUNT(*) FROM v2_boards"},
		{targetPosts, "", "SELECT COUNT(*) FROM v2_posts"},
		{targetComments, "", "SELECT COUNT(*) FROM v2_comments"},
		{"scraps", "SELECT COUNT(*) FROM g5_scrap", "SELECT COUNT(*) FROM v2_scraps"},
		{"files", "SELECT COUNT(*) FROM g5_board_file", "SELECT COUNT(*) FROM v2_files"},
	}

	// Check if g5_memo exists
	if tableExists(db, "g5_memo") {
		pairs = append(pairs, countPair{"messages", "SELECT COUNT(*) FROM g5_memo WHERE me_type = 'recv'", "SELECT COUNT(*) FROM v2_messages"})
	}

	// Calculate total v1 posts/comments across dynamic tables
	boardIDs, err := getBoardIDs(db)
	if err != nil {
		log.Printf("[verify] Warning: failed to get board IDs: %v", err)
		return
	}
	var totalV1Posts, totalV1Comments int64
	for _, boardID := range boardIDs {
		tableName := fmt.Sprintf("g5_write_%s", boardID)
		if !tableExists(db, tableName) {
			continue
		}
		var pCount, cCount int64
		db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE wr_is_comment = 0 AND wr_id = wr_parent", tableName)).Scan(&pCount)
		db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE wr_is_comment = 1", tableName)).Scan(&cCount)
		totalV1Posts += pCount
		totalV1Comments += cCount
	}

	fmt.Println()
	fmt.Println("╔══════════════╦══════════════╦══════════════╦═══════╗")
	fmt.Println("║ Entity       ║  v1 (g5_*)   ║  v2 (v2_*)   ║ Match ║")
	fmt.Println("╠══════════════╬══════════════╬══════════════╬═══════╣")

	for _, p := range pairs {
		var v1Count, v2Count int64

		switch p.label {
		case targetPosts:
			v1Count = totalV1Posts
		case targetComments:
			v1Count = totalV1Comments
		default:
			if p.v1Query != "" {
				db.Raw(p.v1Query).Scan(&v1Count)
			}
		}
		db.Raw(p.v2Query).Scan(&v2Count)

		match := "✗"
		if v1Count == v2Count {
			match = "✓"
		}
		fmt.Printf("║ %-12s ║ %12d ║ %12d ║   %s   ║\n", p.label, v1Count, v2Count, match)
	}

	fmt.Println("╚══════════════╩══════════════╩══════════════╩═══════╝")
	fmt.Println()
}

// --- Dry Run ---

func runDryRun(db *gorm.DB, target string) {
	targets := parseTargets(target)
	boardIDs, err := getBoardIDs(db)
	if err != nil {
		log.Printf("[dry-run] Warning: failed to get board IDs: %v", err)
	}

	for _, t := range targets {
		switch t {
		case "users":
			var count int64
			db.Raw("SELECT COUNT(*) FROM g5_member WHERE mb_id != ''").Scan(&count)
			log.Printf("[dry-run:users] %d records to migrate from g5_member", count)
		case "boards":
			var count int64
			db.Raw("SELECT COUNT(*) FROM g5_board").Scan(&count)
			log.Printf("[dry-run:boards] %d records to migrate from g5_board", count)
		case targetPosts:
			var total int64
			for _, boardID := range boardIDs {
				tableName := fmt.Sprintf("g5_write_%s", boardID)
				if !tableExists(db, tableName) {
					continue
				}
				var count int64
				db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE wr_is_comment = 0 AND wr_id = wr_parent", tableName)).Scan(&count)
				total += count
			}
			log.Printf("[dry-run:posts] %d records to migrate across %d boards", total, len(boardIDs))
		case targetComments:
			var total int64
			for _, boardID := range boardIDs {
				tableName := fmt.Sprintf("g5_write_%s", boardID)
				if !tableExists(db, tableName) {
					continue
				}
				var count int64
				db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE wr_is_comment = 1", tableName)).Scan(&count)
				total += count
			}
			log.Printf("[dry-run:comments] %d records to migrate across %d boards", total, len(boardIDs))
		case "files":
			var count int64
			db.Raw("SELECT COUNT(*) FROM g5_board_file").Scan(&count)
			log.Printf("[dry-run:files] %d records to migrate from g5_board_file", count)
		case "scraps":
			var count int64
			db.Raw("SELECT COUNT(*) FROM g5_scrap").Scan(&count)
			log.Printf("[dry-run:scraps] %d records to migrate from g5_scrap", count)
		case "messages":
			if tableExists(db, "g5_memo") {
				var count int64
				db.Raw("SELECT COUNT(*) FROM g5_memo WHERE me_type = 'recv'").Scan(&count)
				log.Printf("[dry-run:messages] %d records to migrate from g5_memo", count)
			}
		case "memos":
			if tableExists(db, "g5_member_memo") {
				var count int64
				db.Raw("SELECT COUNT(*) FROM g5_member_memo").Scan(&count)
				log.Printf("[dry-run:memos] %d records to migrate from g5_member_memo", count)
			}
		case "points":
			var count int64
			db.Raw("SELECT COUNT(*) FROM g5_point").Scan(&count)
			log.Printf("[dry-run:points] %d records in g5_point (deferred)", count)
		}
	}
}

// --- Rollback ---

func runRollback(db *gorm.DB) {
	log.Println("[rollback] WARNING: This will TRUNCATE all v2 data tables!")
	log.Println("[rollback] Press Ctrl+C to cancel within 5 seconds...")
	time.Sleep(5 * time.Second)

	tables := []string{
		"v2_comments",
		"v2_files",
		"v2_scraps",
		"v2_messages",
		"v2_memos",
		"v2_posts",
		"v2_boards",
		"v2_users",
	}

	db.Exec("SET FOREIGN_KEY_CHECKS = 0")
	for _, t := range tables {
		result := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s", t))
		if result.Error != nil {
			log.Printf("[rollback] Warning: %s: %v", t, result.Error)
		} else {
			log.Printf("[rollback] Truncated %s", t)
		}
	}
	db.Exec("SET FOREIGN_KEY_CHECKS = 1")

	log.Println("[rollback] Complete. All v2 data tables truncated.")
}
