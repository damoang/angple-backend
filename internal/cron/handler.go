package cron

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler handles internal cron job endpoints
type Handler struct {
	db     *gorm.DB
	secret string
}

// NewHandler creates a new cron Handler
func NewHandler(db *gorm.DB) *Handler {
	secret := os.Getenv("CRON_SECRET")
	if secret == "" {
		secret = "angple_cron_2024"
	}
	return &Handler{db: db, secret: secret}
}

// verifySecret checks the secret query parameter
func (h *Handler) verifySecret(c *gin.Context) bool {
	if c.Query("secret") != h.secret {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "invalid secret"})
		return false
	}
	return true
}

// MemberLockRelease handles POST /api/internal/cron/member-lock-release
func (h *Handler) MemberLockRelease(c *gin.Context) {
	if !h.verifySecret(c) {
		return
	}

	result, err := runMemberLockRelease(h.db)
	if err != nil {
		log.Printf("[Cron:member-lock-release] error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	log.Printf("[Cron:member-lock-release] released %d members: %v", result.ReleasedCount, result.ReleasedIDs)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// UpdateMemberLevels handles POST /api/internal/cron/update-member-levels
func (h *Handler) UpdateMemberLevels(c *gin.Context) {
	if !h.verifySecret(c) {
		return
	}

	result, err := runUpdateMemberLevels(h.db)
	if err != nil {
		log.Printf("[Cron:update-member-levels] error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	log.Printf("[Cron:update-member-levels] updated %d members", result.UpdatedCount)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// ProcessApprovedReports handles POST /api/internal/cron/process-approved-reports
func (h *Handler) ProcessApprovedReports(c *gin.Context) {
	if !h.verifySecret(c) {
		return
	}

	result, err := runProcessApprovedReports(h.db)
	if err != nil {
		log.Printf("[Cron:process-approved-reports] error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	log.Printf("[Cron:process-approved-reports] processed %d, errors %d", result.Processed, result.Errors)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// DisciplineRelease handles POST /api/internal/cron/discipline-release
// Restores levels and clears intercept dates for expired disciplines
func (h *Handler) DisciplineRelease(c *gin.Context) {
	if !h.verifySecret(c) {
		return
	}

	result, err := runDisciplineRelease(h.db)
	if err != nil {
		log.Printf("[Cron:discipline-release] error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	log.Printf("[Cron:discipline-release] levels restored: %d %v, intercepts released: %d %v",
		result.LevelRestoredCount, result.LevelRestoredIDs,
		result.InterceptReleasedCount, result.InterceptReleasedIDs)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// UpdateReportPattern handles POST /api/internal/cron/update-report-pattern
func (h *Handler) UpdateReportPattern(c *gin.Context) {
	if !h.verifySecret(c) {
		return
	}

	result, err := runUpdateReportPattern(h.db)
	if err != nil {
		log.Printf("[Cron:update-report-pattern] error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	log.Printf("[Cron:update-report-pattern] report generated: %s", result.Subject)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
