package cron

import (
	"os"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// NotiCleanupResult contains the result of a notification cleanup run.
type NotiCleanupResult struct {
	ReadDeleted  int64  `json:"read_deleted"`  // 읽음 + readOlderThanDays 경과
	StaleDeleted int64  `json:"stale_deleted"` // 읽음 무관 + allOlderThanDays 경과
	Batches      int    `json:"batches"`
	Capped       bool   `json:"capped"` // maxBatches 도달로 중단 (다음 run 에서 계속)
	ExecutedAt   string `json:"executed_at"`
}

// runNotiCleanup prunes g5_na_noti to curb table bloat (#12607: 760만 행).
// 읽은 알림은 readOlderThanDays, 읽지 않은 오래된 알림은 allOlderThanDays 후 삭제한다.
// 락/슬로우쿼리 방지를 위해 batchSize 단위 DELETE 를 반복하며, maxBatches 로 1회 실행량을 제한한다.
func runNotiCleanup(db *gorm.DB) (*NotiCleanupResult, error) {
	readOlderThanDays := envInt("NOTI_CLEANUP_READ_DAYS", 30)
	allOlderThanDays := envInt("NOTI_CLEANUP_ALL_DAYS", 90)
	batchSize := envInt("NOTI_CLEANUP_BATCH", 5000)
	maxBatches := envInt("NOTI_CLEANUP_MAX_BATCHES", 200) // 5000*200 = 1M rows/run 안전 상한

	now := time.Now()
	readCutoff := now.AddDate(0, 0, -readOlderThanDays).Format("2006-01-02 15:04:05")
	allCutoff := now.AddDate(0, 0, -allOlderThanDays).Format("2006-01-02 15:04:05")

	res := &NotiCleanupResult{ExecutedAt: now.Format("2006-01-02 15:04:05")}

	deleteLoop := func(where string, args ...interface{}) (int64, error) {
		var total int64
		for res.Batches < maxBatches {
			q := append([]interface{}{}, args...)
			r := db.Exec("DELETE FROM g5_na_noti WHERE "+where+" LIMIT ?", append(q, batchSize)...)
			if r.Error != nil {
				return total, r.Error
			}
			res.Batches++
			total += r.RowsAffected
			if r.RowsAffected < int64(batchSize) {
				return total, nil // 더 지울 것 없음
			}
			if res.Batches >= maxBatches {
				res.Capped = true
				return total, nil
			}
		}
		return total, nil
	}

	// 1) 읽은 알림 중 readCutoff 이전
	read, err := deleteLoop("ph_readed = 'Y' AND ph_datetime < ?", readCutoff)
	if err != nil {
		return res, err
	}
	res.ReadDeleted = read

	// 2) 읽음 무관 allCutoff 이전 (오래된 미읽음 포함)
	if !res.Capped {
		stale, err := deleteLoop("ph_datetime < ?", allCutoff)
		if err != nil {
			return res, err
		}
		res.StaleDeleted = stale
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
