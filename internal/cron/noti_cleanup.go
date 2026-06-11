package cron

import (
	"os"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// NotiCleanupResult contains the result of a notification cleanup run.
type NotiCleanupResult struct {
	Deleted    int64  `json:"deleted"` // 이번 run 에 삭제된 행 수
	Batches    int    `json:"batches"`
	LastID     int64  `json:"last_id"` // 이번 run 이 도달한 ph_id 워터마크
	Capped     bool   `json:"capped"`  // maxBatches 도달로 중단 (다음 run 에서 계속)
	ExecutedAt string `json:"executed_at"`
}

// runNotiCleanup prunes g5_na_noti to curb table bloat (#12607: 760만 행).
// 삭제 대상: ① 읽은 알림 중 readOlderThanDays 경과, ② 읽음 무관 allOlderThanDays 경과.
//
// g5_na_noti 에는 ph_datetime/ph_readed 선두 인덱스가 없어 조건 기반 LIMIT DELETE 는
// 매 배치마다 760만 행 풀스캔을 유발한다. 이를 피하려 PRIMARY KEY(ph_id, auto_increment)
// 오름차순 윈도우로 walk 하며 [cursor, cursor+window) 범위만 PK 인덱스로 스캔/삭제한다.
// 오래된 행일수록 ph_id 가 작아 앞쪽 윈도우에서 먼저 지워지고, 삭제분만큼 다음 run 의
// MIN(ph_id) 가 올라가 진행된다. maxBatches 로 1회 실행량(락/부하)을 제한한다.
func runNotiCleanup(db *gorm.DB) (*NotiCleanupResult, error) {
	readOlderThanDays := envInt("NOTI_CLEANUP_READ_DAYS", 30)
	allOlderThanDays := envInt("NOTI_CLEANUP_ALL_DAYS", 90)
	window := int64(envInt("NOTI_CLEANUP_BATCH", 5000)) // ph_id 윈도우 폭 (PK 범위 스캔)
	maxBatches := envInt("NOTI_CLEANUP_MAX_BATCHES", 200)

	now := time.Now()
	readCutoff := now.AddDate(0, 0, -readOlderThanDays).Format("2006-01-02 15:04:05")
	allCutoff := now.AddDate(0, 0, -allOlderThanDays).Format("2006-01-02 15:04:05")

	res := &NotiCleanupResult{ExecutedAt: now.Format("2006-01-02 15:04:05")}

	// 테이블 ph_id 범위 (한 번만 조회)
	var bounds struct {
		MinID int64
		MaxID int64
	}
	if err := db.Raw("SELECT COALESCE(MIN(ph_id),0) AS min_id, COALESCE(MAX(ph_id),0) AS max_id FROM g5_na_noti").
		Scan(&bounds).Error; err != nil {
		return res, err
	}
	if bounds.MaxID == 0 {
		return res, nil // 빈 테이블
	}

	// 한 윈도우 내에서 두 조건을 OR 로 한 번에 삭제(범위 재주행 방지).
	const where = "ph_id >= ? AND ph_id < ? AND ((ph_readed = 'Y' AND ph_datetime < ?) OR ph_datetime < ?)"

	for cursor := bounds.MinID; cursor <= bounds.MaxID; cursor += window {
		if res.Batches >= maxBatches {
			res.Capped = true
			break
		}
		hi := cursor + window // [cursor, hi)
		r := db.Exec("DELETE FROM g5_na_noti WHERE "+where, cursor, hi, readCutoff, allCutoff)
		if r.Error != nil {
			return res, r.Error
		}
		res.Batches++
		res.Deleted += r.RowsAffected
		res.LastID = hi
	}

	return res, nil
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}
