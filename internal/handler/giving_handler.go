package handler

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	givingdomain "github.com/damoang/angple-backend/internal/domain/giving"
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
	Extra4           string `json:"extra_4,omitempty"`
	Extra5           string `json:"extra_5"`
	GivingStart      string `json:"giving_start,omitempty"`
	GivingEnd        string `json:"giving_end,omitempty"`
	GivingStatus     string `json:"giving_status"`
	ParticipantCount int    `json:"participant_count"`
	IsPaused         bool   `json:"is_paused"`
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
		Wr4              string `gorm:"column:wr_4"` // start_time
		Wr5              string `gorm:"column:wr_5"` // end_time
		Wr7              string `gorm:"column:wr_7"` // state
		ParticipantCount int    `gorm:"column:participant_count"`
	}

	query := h.db.Table("g5_write_giving AS g").
		Select(`g.wr_id, g.wr_subject, g.wr_4, g.wr_5, g.wr_7,
			COALESCE((SELECT COUNT(DISTINCT b.mb_id) FROM g5_giving_bid b WHERE b.wr_id = g.wr_id), 0) AS participant_count`).
		Where("g.wr_is_comment = 0").
		Where("g.wr_deleted_at IS NULL")

	var rows []givingRow
	if err := query.Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to fetch giving list",
		})
		return
	}

	items := make([]GivingListItem, 0, len(rows))
	for _, r := range rows {
		meta := givingdomain.Normalize(now, givingdomain.Meta{
			StartRaw:         r.Wr4,
			EndRaw:           r.Wr5,
			StateRaw:         r.Wr7,
			ParticipantCount: r.ParticipantCount,
		})

		if meta.Status == givingdomain.StatusNoGiving {
			continue
		}
		if tab == "active" && meta.Status == givingdomain.StatusEnded {
			continue
		}
		if tab == "ended" && meta.Status != givingdomain.StatusEnded {
			continue
		}

		items = append(items, GivingListItem{
			ID:               r.WrID,
			Title:            r.WrSubject,
			Extra4:           r.Wr4,
			Extra5:           r.Wr5,
			GivingStart:      meta.GivingStart,
			GivingEnd:        meta.GivingEnd,
			GivingStatus:     string(meta.Status),
			ParticipantCount: meta.ParticipantCount,
			IsPaused:         meta.IsPaused,
			IsUrgent:         meta.IsUrgent,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		switch sortBy {
		case "newest":
			return items[i].ID > items[j].ID
		case "urgent":
			fallthrough
		default:
			return items[i].GivingEnd < items[j].GivingEnd
		}
	})
	if len(items) > limit {
		items = items[:limit]
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}
