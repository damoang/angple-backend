package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GivingHandler handles giving plugin API endpoints
type GivingHandler struct {
	db *gorm.DB
}

// NewGivingHandler creates a new GivingHandler
func NewGivingHandler(db *gorm.DB) *GivingHandler {
	return &GivingHandler{db: db}
}

// GivingListItem represents a giving item in list response
type GivingListItem struct {
	ID               int    `json:"id"`
	Title            string `json:"title"`
	Extra5           string `json:"extra_5"`
	ParticipantCount int    `json:"participant_count"`
	IsUrgent         bool   `json:"is_urgent"`
}

// List returns giving posts filtered by tab (active/ended)
// GET /api/plugins/giving/list?tab=active&limit=8&sort=urgent
func (h *GivingHandler) List(c *gin.Context) {
	tab := c.DefaultQuery("tab", "active")
	sortBy := c.DefaultQuery("sort", "urgent")
	limitStr := c.DefaultQuery("limit", "8")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 50 {
		limit = 8
	}

	now := time.Now()

	type givingRow struct {
		WrID             int    `gorm:"column:wr_id"`
		WrSubject        string `gorm:"column:wr_subject"`
		Wr5              string `gorm:"column:wr_5"` // end_time
		ParticipantCount int    `gorm:"column:participant_count"`
	}

	query := h.db.Table("g5_write_giving AS g").
		Select(`g.wr_id, g.wr_subject, g.wr_5,
			COALESCE((SELECT COUNT(DISTINCT b.mb_id) FROM g5_giving_bid b WHERE b.wr_id = g.wr_id), 0) AS participant_count`).
		Where("g.wr_is_comment = 0")

	nowStr := now.Format("2006-01-02T15:04")
	switch tab {
	case "active":
		query = query.
			Where("(g.wr_7 = '' OR g.wr_7 IS NULL)").
			Where("g.wr_5 != '' AND g.wr_5 > ?", nowStr)
	case "ended":
		query = query.Where(
			"(g.wr_5 != '' AND g.wr_5 <= ?) OR g.wr_7 = '2'", nowStr,
		)
	}

	switch sortBy {
	case "urgent":
		query = query.Order("g.wr_5 ASC")
	case "newest":
		query = query.Order("g.wr_id DESC")
	default:
		query = query.Order("g.wr_5 ASC")
	}

	var rows []givingRow
	if err := query.Limit(limit).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to fetch giving list",
		})
		return
	}

	items := make([]GivingListItem, 0, len(rows))
	for _, r := range rows {
		isUrgent := false
		if r.Wr5 != "" {
			if endTime, err := parseGivingTime(r.Wr5); err == nil {
				diff := endTime.Sub(now)
				isUrgent = diff > 0 && diff <= 24*time.Hour
			}
		}

		items = append(items, GivingListItem{
			ID:               r.WrID,
			Title:            r.WrSubject,
			Extra5:           r.Wr5,
			ParticipantCount: r.ParticipantCount,
			IsUrgent:         isUrgent,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}

// parseGivingTime parses various time formats used in giving posts
func parseGivingTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	loc, _ := time.LoadLocation("Asia/Seoul")
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time: %s", s)
}
