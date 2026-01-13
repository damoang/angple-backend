package main

import (
	"fmt"
	"log"

	"github.com/damoang/angple-backend/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	// 설정 로드
	cfg, err := config.Load("configs/config.local.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// MySQL 연결
	dsn := cfg.Database.GetDSN()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// UTF-8 설정
	db.Exec("SET NAMES utf8mb4")
	db.Exec("SET CHARACTER SET utf8mb4")
	db.Exec("SET character_set_connection=utf8mb4")

	// 기존 데이터 삭제
	db.Exec("DELETE FROM g5_write_free WHERE wr_id IN (1, 2, 3)")

	// UTF-8로 새 데이터 삽입
	posts := []map[string]interface{}{
		{
			"wr_id":         1,
			"wr_subject":    "첫 번째 테스트 게시글",
			"wr_content":    "안녕하세요! 첫 번째 테스트 게시글입니다.",
			"wr_name":       "관리자",
			"mb_id":         "admin",
			"wr_hit":        0,
			"wr_datetime":   "2026-01-12 23:00:00",
			"wr_is_comment": 0,
			"wr_parent":     1,
			"wr_num":        3,
			"wr_reply":      "",
		},
		{
			"wr_id":         2,
			"wr_subject":    "두 번째 테스트 게시글",
			"wr_content":    "백엔드 API가 정상 작동합니다!",
			"wr_name":       "관리자",
			"mb_id":         "admin",
			"wr_hit":        0,
			"wr_datetime":   "2026-01-12 23:05:00",
			"wr_is_comment": 0,
			"wr_parent":     2,
			"wr_num":        2,
			"wr_reply":      "",
		},
		{
			"wr_id":         3,
			"wr_subject":    "SvelteKit 5 + Go Fiber 통합 테스트",
			"wr_content":    "프론트엔드와 백엔드가 성공적으로 연동되었습니다!",
			"wr_name":       "관리자",
			"mb_id":         "admin",
			"wr_hit":        0,
			"wr_datetime":   "2026-01-12 23:10:00",
			"wr_is_comment": 0,
			"wr_parent":     3,
			"wr_num":        1,
			"wr_reply":      "",
		},
	}

	for _, post := range posts {
		result := db.Table("g5_write_free").Create(post)
		if result.Error != nil {
			log.Printf("Failed to insert post %d: %v", post["wr_id"], result.Error)
		} else {
			log.Printf("✅ Inserted: %s", post["wr_subject"])
		}
	}

	fmt.Println("\n✅ UTF-8 데이터 삽입 완료!")
}
