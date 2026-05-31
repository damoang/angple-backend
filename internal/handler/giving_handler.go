package handler

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	givingdomain "github.com/damoang/angple-backend/internal/domain/giving"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// givingBoardSlug is the g5_write_{slug} table suffix + g5_board_file.bo_table key.
const givingBoardSlug = "giving"

// GivingHandler handles giving plugin API endpoints
type GivingHandler struct {
	db       *gorm.DB
	fileRepo *gnurepo.FileRepository
	cdnURL   string
}

// NewGivingHandler creates a new GivingHandler.
// fileRepo + cdnURL 는 thumbnail enrich (g5_board_file → CDN URL) 에 사용.
func NewGivingHandler(db *gorm.DB, fileRepo *gnurepo.FileRepository, cdnURL string) *GivingHandler {
	return &GivingHandler{
		db:       db,
		fileRepo: fileRepo,
		cdnURL:   strings.TrimRight(cdnURL, "/"),
	}
}

// GivingListItem represents a giving item in list response
type GivingListItem struct {
	ID               int    `json:"id"`
	Title            string `json:"title"`
	Extra4           string `json:"extra_4,omitempty"`
	Extra5           string `json:"extra_5"`
	Extra10          string `json:"extra_10,omitempty"`
	Thumbnail        string `json:"thumbnail,omitempty"`
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
		Wr4              string `gorm:"column:wr_4"`  // start_time
		Wr5              string `gorm:"column:wr_5"`  // end_time
		Wr7              string `gorm:"column:wr_7"`  // state
		Wr10             string `gorm:"column:wr_10"` // image URL (사용자 입력)
		ParticipantCount int    `gorm:"column:participant_count"`
	}

	query := h.db.Table("g5_write_giving AS g").
		Select(`g.wr_id, g.wr_subject, g.wr_4, g.wr_5, g.wr_7, g.wr_10,
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

		// 시작/종료 시각 미입력 (wr_4='' 또는 wr_5='') 글도 active 탭에 노출.
		// damoang-backend ListActive 정책 (wr_4='' OR wr_5 > NOW + wr_5='' OR wr_5 > NOW) 과 일치.
		// /giving/2238 처럼 시간 미정 진행중 글이 noGiving 으로 분류되어 응답 누락되던 #new 버그 fix.
		if meta.Status == givingdomain.StatusNoGiving {
			if tab == "ended" {
				continue
			}
			// active 탭 에는 noGiving 도 포함 (진행중 으로 간주)
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
			Extra10:          r.Wr10,
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

	// Thumbnail enrich: extra_10 비어있는 row 에 한해 g5_board_file 첫 이미지 사용.
	// batch IN (bo_table='giving', wr_id IN …). frontend giving-card 의 fallback 우선순위
	// (extra_10 → thumbnail → images[0]) 와 일관.
	if h.fileRepo != nil {
		needIDs := make([]int, 0, len(items))
		for _, it := range items {
			if it.Extra10 == "" {
				needIDs = append(needIDs, it.ID)
			}
		}
		if len(needIDs) > 0 {
			if files, ferr := h.fileRepo.GetFirstImagesByPostIDs(givingBoardSlug, needIDs); ferr == nil && len(files) > 0 {
				for i := range items {
					if items[i].Extra10 != "" {
						continue
					}
					if fname, ok := files[items[i].ID]; ok {
						if h.cdnURL != "" {
							items[i].Thumbnail = h.cdnURL + "/data/file/" + givingBoardSlug + "/" + fname
						} else {
							items[i].Thumbnail = "data/file/" + givingBoardSlug + "/" + fname
						}
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}
