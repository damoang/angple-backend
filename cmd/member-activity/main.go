package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/damoang/angple-backend/internal/config"
	"github.com/damoang/angple-backend/internal/service"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	configPath := flag.String("config", "configs/config.prod.yaml", "config file path")
	mode := flag.String("mode", "verify", "mode: verify, backfill, or repair")
	board := flag.String("board", "", "single legacy board slug to process")
	scope := flag.String("scope", "legacy", "scope: legacy, v2, all")
	batchSize := flag.Int("batch-size", 500, "batch size for backfill")
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

	svc := service.NewMemberActivitySyncService(db)

	switch *mode {
	case "backfill":
		runBackfill(db, svc, *scope, *board, *batchSize)
	case "verify":
		runVerifyOrExit(db, svc, *scope, *board)
	case "repair":
		runRepair(db, svc, *scope, *board, *batchSize)
	default:
		log.Fatalf("unsupported mode: %s", *mode)
	}
}

func runBackfill(db *gorm.DB, svc *service.MemberActivitySyncService, scope, board string, batchSize int) {
	if scope == "all" || scope == "legacy" {
		boardIDs, err := getBoardIDs(db, board)
		if err != nil {
			log.Fatalf("failed to load board ids: %v", err)
		}
		for _, boardID := range boardIDs {
			report, err := svc.BackfillLegacyBoard(boardID, batchSize)
			if err != nil {
				log.Fatalf("legacy backfill failed for %s: %v", boardID, err)
			}
			log.Printf("[backfill][legacy] board=%s source_posts=%d source_comments=%d synced_posts=%d synced_comments=%d",
				report.BoardSlug, report.PostCount, report.CommentCount, report.ProcessedPosts, report.ProcessedComments)
		}
	}

	if scope == "all" || scope == "v2" {
		report, err := svc.BackfillV2(batchSize)
		if err != nil {
			log.Fatalf("v2 backfill failed: %v", err)
		}
		log.Printf("[backfill][v2] source_posts=%d source_comments=%d synced_posts=%d synced_comments=%d",
			report.PostCount, report.CommentCount, report.ProcessedPosts, report.ProcessedComments)
	}
}

func runRepair(db *gorm.DB, svc *service.MemberActivitySyncService, scope, board string, batchSize int) {
	log.Printf("[repair] pre-check verify start scope=%s board=%s", scope, board)
	if !runVerify(db, svc, scope, board) {
		log.Printf("[repair] mismatch detected, running backfill")
		runBackfill(db, svc, scope, board, batchSize)
		log.Printf("[repair] post-backfill verify start")
		if !runVerify(db, svc, scope, board) {
			fmt.Fprintln(os.Stderr, "member activity repair completed with remaining mismatches")
			os.Exit(1)
		}
		log.Printf("[repair] verify passed after backfill")
		return
	}
	log.Printf("[repair] verify already clean; backfill skipped")
}

func runVerifyOrExit(db *gorm.DB, svc *service.MemberActivitySyncService, scope, board string) {
	if !runVerify(db, svc, scope, board) {
		fmt.Fprintln(os.Stderr, "member activity verification found mismatches")
		os.Exit(1)
	}
}

func runVerify(db *gorm.DB, svc *service.MemberActivitySyncService, scope, board string) bool {
	hasMismatch := false

	if scope == "all" || scope == "legacy" {
		boardIDs, err := getBoardIDs(db, board)
		if err != nil {
			log.Fatalf("failed to load board ids: %v", err)
		}
		for _, boardID := range boardIDs {
			report, err := svc.VerifyLegacyBoard(boardID)
			if err != nil {
				log.Fatalf("legacy verify failed for %s: %v", boardID, err)
			}
			mismatch := report.SourcePosts != report.FeedPosts ||
				report.SourceComments != report.FeedComments ||
				report.SourceDeletedPosts != report.FeedDeletedPosts ||
				report.SourceDeletedComments != report.FeedDeletedComments
			if mismatch {
				hasMismatch = true
			}
			log.Printf("[verify][legacy] board=%s posts=%d/%d comments=%d/%d deleted_posts=%d/%d deleted_comments=%d/%d mismatch=%v",
				report.BoardSlug,
				report.SourcePosts, report.FeedPosts,
				report.SourceComments, report.FeedComments,
				report.SourceDeletedPosts, report.FeedDeletedPosts,
				report.SourceDeletedComments, report.FeedDeletedComments,
				mismatch)
		}
	}

	if scope == "all" || scope == "v2" {
		report, err := svc.VerifyV2()
		if err != nil {
			log.Fatalf("v2 verify failed: %v", err)
		}
		mismatch := report.SourcePosts != report.FeedPosts ||
			report.SourceComments != report.FeedComments ||
			report.SourceDeletedPosts != report.FeedDeletedPosts ||
			report.SourceDeletedComments != report.FeedDeletedComments
		if mismatch {
			hasMismatch = true
		}
		log.Printf("[verify][v2] posts=%d/%d comments=%d/%d deleted_posts=%d/%d deleted_comments=%d/%d mismatch=%v",
			report.SourcePosts, report.FeedPosts,
			report.SourceComments, report.FeedComments,
			report.SourceDeletedPosts, report.FeedDeletedPosts,
			report.SourceDeletedComments, report.FeedDeletedComments,
			mismatch)
	}

	if hasMismatch {
		return false
	}
	return true
}

func getBoardIDs(db *gorm.DB, onlyBoard string) ([]string, error) {
	if onlyBoard != "" {
		return []string{onlyBoard}, nil
	}

	var boardIDs []string
	if err := db.Table("g5_board").
		Where("bo_table <> ''").
		Order("bo_table ASC").
		Pluck("bo_table", &boardIDs).Error; err != nil {
		return nil, err
	}
	return boardIDs, nil
}
