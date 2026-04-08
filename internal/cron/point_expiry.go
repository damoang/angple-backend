package cron

import (
	"fmt"
	"log"
	"net/http"
	"time"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/gin-gonic/gin"
)

// PointExpiryResult contains the result of a point expiry batch run
type PointExpiryResult struct {
	ExpiredCount int    `json:"expired_count"`
	ExecutedAt   string `json:"executed_at"`
}

// PointExpiryNotifyResult contains the result of a point expiry notification run
type PointExpiryNotifyResult struct {
	NotifiedCount int    `json:"notified_count"`
	ExecutedAt    string `json:"executed_at"`
}

// PointExpiry handles POST /api/internal/cron/point-expiry
func (h *Handler) PointExpiry(c *gin.Context) {
	if !h.verifySecret(c) {
		return
	}

	if h.pointConfigRepo == nil || h.gnuPointWriteRepo == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": PointExpiryResult{
			ExpiredCount: 0,
			ExecutedAt:   time.Now().Format(time.RFC3339),
		}, "message": "point expiry not configured"})
		return
	}

	// Check if expiry is enabled
	config, err := h.pointConfigRepo.GetPointConfig()
	if err != nil {
		log.Printf("[Cron:point-expiry] config error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if !config.ExpiryEnabled {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": PointExpiryResult{
			ExpiredCount: 0,
			ExecutedAt:   time.Now().Format(time.RFC3339),
		}, "message": "expiry disabled"})
		return
	}

	expired, err := h.gnuPointWriteRepo.ExpireBatch(1000)
	if err != nil {
		log.Printf("[Cron:point-expiry] error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	result := PointExpiryResult{
		ExpiredCount: expired,
		ExecutedAt:   time.Now().Format(time.RFC3339),
	}
	log.Printf("[Cron:point-expiry] expired %d point entries", expired)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// PointExpiryNotify handles POST /api/internal/cron/point-expiry-notify
func (h *Handler) PointExpiryNotify(c *gin.Context) {
	if !h.verifySecret(c) {
		return
	}

	if h.pointConfigRepo == nil || h.gnuPointWriteRepo == nil || h.notiRepo == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": PointExpiryNotifyResult{
			NotifiedCount: 0,
			ExecutedAt:    time.Now().Format(time.RFC3339),
		}, "message": "point expiry notify not configured"})
		return
	}

	config, err := h.pointConfigRepo.GetPointConfig()
	if err != nil {
		log.Printf("[Cron:point-expiry-notify] config error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if !config.ExpiryEnabled {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": PointExpiryNotifyResult{
			NotifiedCount: 0,
			ExecutedAt:    time.Now().Format(time.RFC3339),
		}, "message": "expiry disabled"})
		return
	}

	// Get members with points expiring within 7 days
	expiringMembers, err := h.gnuPointWriteRepo.GetExpiringPoints(7, 500)
	if err != nil {
		log.Printf("[Cron:point-expiry-notify] error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	notified := 0
	today := time.Now().Format("2006-01-02")
	for _, m := range expiringMembers {
		// Check for duplicate notification today
		dedupeKey := fmt.Sprintf("point_expiry_%s", today)
		var count int64
		h.db.Table("g5_na_noti").
			Where("mb_id = ? AND ph_from_case = ? AND parent_subject = ?", m.MbID, "point_expiry", dedupeKey).
			Count(&count)
		if count > 0 {
			continue
		}

		noti := &gnurepo.Notification{
			MbID:          m.MbID,
			PhFromCase:    "point_expiry",
			PhToCase:      "me",
			BoTable:       "@system",
			WrID:          0,
			RelMbID:       "system",
			RelMbNick:     "시스템",
			RelMsg:        fmt.Sprintf("7일 내 %dP가 만료됩니다", m.ExpiringAmount),
			RelURL:        "/point",
			PhReaded:      "N",
			ParentSubject: dedupeKey,
		}
		if err := h.notiRepo.Create(noti); err != nil {
			log.Printf("[Cron:point-expiry-notify] notification failed for %s: %v", m.MbID, err)
			continue
		}
		notified++
	}

	result := PointExpiryNotifyResult{
		NotifiedCount: notified,
		ExecutedAt:    time.Now().Format(time.RFC3339),
	}
	log.Printf("[Cron:point-expiry-notify] notified %d members", notified)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
