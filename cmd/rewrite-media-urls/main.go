package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/damoang/angple-backend/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type rewritePlan struct {
	boardID         string
	tableName       string
	contentMatches  int64
	wr10Matches     int64
	contentExamples []int
	wr10Examples    []int
}

func main() {
	configPath := flag.String("config", "configs/config.prod.yaml", "config file path")
	apply := flag.Bool("apply", false, "apply database updates")
	board := flag.String("board", "", "single board id to process")
	limit := flag.Int("limit", 5, "example row limit per board")
	sampleOnly := flag.Bool("sample-only", false, "skip full count scans and only fetch example row ids")
	verbose := flag.Bool("verbose", false, "verbose SQL logging")
	flag.Parse()

	loaded := config.LoadDotEnv()
	if len(loaded) > 0 {
		log.Printf("Loaded env files: %v", loaded)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	cdnURL := strings.TrimRight(cfg.Storage.CDNURL, "/")
	if cdnURL == "" {
		log.Fatal("CDN_URL is empty; refusing to continue")
	}

	logLevel := gormlogger.Warn
	if *verbose {
		logLevel = gormlogger.Info
	}

	db, err := gorm.Open(mysql.Open(cfg.Database.GetDSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(logLevel),
	})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to get underlying DB: %v", err)
	}
	defer sqlDB.Close()

	boardIDs, err := getBoardIDs(db)
	if err != nil {
		log.Fatalf("failed to load board ids: %v", err)
	}

	if *board != "" {
		boardIDs = []string{*board}
	}

	var totalContent int64
	var totalWr10 int64
	var changedTables int

	for _, boardID := range boardIDs {
		tableName := fmt.Sprintf("g5_write_%s", boardID)
		if !tableExists(db, tableName) {
			continue
		}

		plan, err := buildPlan(db, boardID, tableName, *limit, *sampleOnly)
		if err != nil {
			log.Printf("[audit] %s failed: %v", tableName, err)
			continue
		}

		if plan.contentMatches == 0 && plan.wr10Matches == 0 {
			continue
		}

		changedTables++
		totalContent += plan.contentMatches
		totalWr10 += plan.wr10Matches

		log.Printf("[audit] %s content=%d wr10=%d content_examples=%v wr10_examples=%v",
			tableName, plan.contentMatches, plan.wr10Matches, plan.contentExamples, plan.wr10Examples)

		if !*apply {
			continue
		}

		if err := applyRewrites(db, tableName, cdnURL); err != nil {
			log.Printf("[apply] %s failed: %v", tableName, err)
			continue
		}
		log.Printf("[apply] %s updated", tableName)
	}

	log.Printf("[summary] tables=%d content_matches=%d wr10_matches=%d apply=%v",
		changedTables, totalContent, totalWr10, *apply)
}

func buildPlan(db *gorm.DB, boardID, tableName string, limit int, sampleOnly bool) (*rewritePlan, error) {
	plan := &rewritePlan{
		boardID:   boardID,
		tableName: tableName,
	}

	contentWhere := mediaContentWhereClause()
	wr10Where := mediaWr10WhereClause()

	if !sampleOnly {
		if err := db.Table(tableName).Where(contentWhere).Count(&plan.contentMatches).Error; err != nil {
			return nil, err
		}
		if err := db.Table(tableName).Where(wr10Where).Count(&plan.wr10Matches).Error; err != nil {
			return nil, err
		}
	}

	if err := db.Table(tableName).
		Select("wr_id").
		Where(contentWhere).
		Order("wr_id DESC").
		Limit(limit).
		Scan(&plan.contentExamples).Error; err != nil {
		return nil, err
	}
	if err := db.Table(tableName).
		Select("wr_id").
		Where(wr10Where).
		Order("wr_id DESC").
		Limit(limit).
		Scan(&plan.wr10Examples).Error; err != nil {
		return nil, err
	}

	if sampleOnly {
		plan.contentMatches = int64(len(plan.contentExamples))
		plan.wr10Matches = int64(len(plan.wr10Examples))
	}

	return plan, nil
}

func applyRewrites(db *gorm.DB, tableName, cdnURL string) error {
	contentExpr := normalizeContentSQL("wr_content", cdnURL)
	wr10Expr := normalizeWr10SQL("wr_10", cdnURL)

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			fmt.Sprintf("UPDATE `%s` SET wr_content = %s WHERE %s", tableName, contentExpr, mediaContentWhereClause()),
		).Error; err != nil {
			return err
		}

		if err := tx.Exec(
			fmt.Sprintf("UPDATE `%s` SET wr_10 = %s WHERE %s", tableName, wr10Expr, mediaWr10WhereClause()),
		).Error; err != nil {
			return err
		}

		return nil
	})
}

func normalizeContentSQL(column, cdnURL string) string {
	replacements := [][2]string{
		{`src="/data/`, `src="` + cdnURL + `/data/`},
		{`src='/data/`, `src='` + cdnURL + `/data/`},
		{`src="data/`, `src="` + cdnURL + `/data/`},
		{`src='data/`, `src='` + cdnURL + `/data/`},
		{`href="/data/`, `href="` + cdnURL + `/data/`},
		{`href='/data/`, `href='` + cdnURL + `/data/`},
		{`href="data/`, `href="` + cdnURL + `/data/`},
		{`href='data/`, `href='` + cdnURL + `/data/`},
	}

	expr := column
	for _, pair := range replacements {
		expr = fmt.Sprintf("REPLACE(%s, '%s', '%s')", expr, escapeSQL(pair[0]), escapeSQL(pair[1]))
	}
	return expr
}

func normalizeWr10SQL(column, cdnURL string) string {
	return fmt.Sprintf(
		"CASE "+
			"WHEN %s LIKE '/data/%%' THEN CONCAT('%s', %s) "+
			"WHEN %s LIKE 'data/%%' THEN CONCAT('%s/', %s) "+
			"ELSE %s END",
		column, escapeSQL(cdnURL), column,
		column, escapeSQL(cdnURL), column,
		column,
	)
}

func mediaContentWhereClause() string {
	return strings.Join([]string{
		"wr_content LIKE '%src=\"/data/%'",
		"wr_content LIKE '%src=''/data/%'",
		"wr_content LIKE '%src=\"data/%'",
		"wr_content LIKE '%src=''data/%'",
		"wr_content LIKE '%href=\"/data/%'",
		"wr_content LIKE '%href=''/data/%'",
		"wr_content LIKE '%href=\"data/%'",
		"wr_content LIKE '%href=''data/%'",
	}, " OR ")
}

func mediaWr10WhereClause() string {
	return "wr_10 LIKE '/data/%' OR wr_10 LIKE 'data/%'"
}

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

func escapeSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
