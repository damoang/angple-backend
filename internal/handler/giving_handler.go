package handler

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	givingdomain "github.com/damoang/angple-backend/internal/domain/giving"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// givingBoardSlug is the g5_write_{slug} table suffix + g5_board_file.bo_table key.
const givingBoardSlug = "giving"

// givingContentExcerptLen limits the wr_content slice read for first-image extraction.
const givingContentExcerptLen = 1000

// givingImgRegex extracts the first <img src="..."> URL from a giving post body.
var givingImgRegex = regexp.MustCompile(`<img[^>]+src=["']([^"']+)["']`)

// GivingHandler handles giving plugin API endpoints
type GivingHandler struct {
	db              *gorm.DB
	fileRepo        *gnurepo.FileRepository
	cdnURL          string
	pointConfigRepo v2repo.PointConfigRepository
	memoRepo        gnurepo.MemoRepository
}

// NewGivingHandler creates a new GivingHandler.
// fileRepo + cdnURL 는 thumbnail enrich (g5_board_file → CDN URL) 에 사용.
// pointConfigRepo 는 유료 응모(lowest_unique) 수수료 지급 시 포인트 만료일 산정에 사용
// (nil 이면 만료 없는 기본값으로 처리).
// memoRepo 는 개표 완료 시 당첨자·주최자에게 자동 쪽지(g5_memo)를 보내는 데 사용
// (nil 이면 쪽지 발송을 건너뛴다 — 알림 g5_na_noti 는 영향받지 않는다).
func NewGivingHandler(db *gorm.DB, fileRepo *gnurepo.FileRepository, cdnURL string, pointConfigRepo v2repo.PointConfigRepository, memoRepo gnurepo.MemoRepository) *GivingHandler {
	return &GivingHandler{
		db:              db,
		fileRepo:        fileRepo,
		cdnURL:          strings.TrimRight(cdnURL, "/"),
		pointConfigRepo: pointConfigRepo,
		memoRepo:        memoRepo,
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

// givingRow holds raw columns selected from g5_write_giving.
type givingRow struct {
	WrID             int    `gorm:"column:wr_id"`
	WrSubject        string `gorm:"column:wr_subject"`
	Wr4              string `gorm:"column:wr_4"`  // start_time
	Wr5              string `gorm:"column:wr_5"`  // end_time
	Wr7              string `gorm:"column:wr_7"`  // state
	Wr10             string `gorm:"column:wr_10"` // image URL (사용자 입력)
	WrContent        string `gorm:"column:wr_content"`
	ParticipantCount int    `gorm:"column:participant_count"`
}

// extractFirstImageURL returns the first <img src> in HTML content, or "".
func extractFirstImageURL(html string) string {
	m := givingImgRegex.FindStringSubmatch(html)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// shouldKeep decides whether a row passes the tab filter.
func shouldKeep(meta givingdomain.Normalized, tab string) bool {
	if meta.Status == givingdomain.StatusNoGiving {
		// active 탭에는 noGiving (시간 미정) 도 진행중으로 포함, ended 탭에서는 제외
		return tab != "ended"
	}
	if tab == "active" && meta.Status == givingdomain.StatusEnded {
		return false
	}
	if tab == "ended" && meta.Status != givingdomain.StatusEnded {
		return false
	}
	return true
}

// givingUrgentRank orders statuses for the "urgent" sort:
// 진행중(0) → 대기중/일시정지(1) → 일정미정·기타(2).
func givingUrgentRank(status string) int {
	switch givingdomain.Status(status) {
	case givingdomain.StatusActive:
		return 0
	case givingdomain.StatusWaiting, givingdomain.StatusPaused:
		return 1
	default:
		return 2
	}
}

// givingListLess is the list ordering. Extracted for unit tests.
//
// ⛔ 종전 "urgent" 는 GivingEnd 문자열 오름차순뿐이었다. 일정이 없는 글은 GivingEnd="" 라
// 항상 맨 앞으로 와서, 진짜 진행중 글이 limit 밖으로 밀렸다 — 위젯 "나눔" 에 몇 년 전
// "(마감)" 글만 보이던 원인. 상태 서열을 먼저 두고, 같은 서열 안에서만 시각으로 비교한다.
func givingListLess(a, b GivingListItem, sortBy string) bool {
	switch sortBy {
	case "newest":
		return a.ID > b.ID
	default: // "urgent"
		ra, rb := givingUrgentRank(a.GivingStatus), givingUrgentRank(b.GivingStatus)
		if ra != rb {
			return ra < rb
		}
		switch ra {
		case 0: // 진행중: 종료 임박순
			return a.GivingEnd < b.GivingEnd
		case 1: // 대기중·일시정지: 시작 임박순
			return a.GivingStart < b.GivingStart
		default: // 일정미정·기타: 최신 글 우선
			return a.ID > b.ID
		}
	}
}

// enrichThumbnails fills Thumbnail when Extra10 is empty.
// Priority: extra_10 (already set) > 본문 첫 <img> > g5_board_file 첫 이미지.
func (h *GivingHandler) enrichThumbnails(items []GivingListItem, contentByID map[int]string) {
	needFileLookup := make([]int, 0, len(items))
	for i := range items {
		if items[i].Extra10 != "" || items[i].Thumbnail != "" {
			continue
		}
		if c := contentByID[items[i].ID]; c != "" {
			if url := extractFirstImageURL(c); url != "" {
				items[i].Thumbnail = url
				continue
			}
		}
		needFileLookup = append(needFileLookup, items[i].ID)
	}

	if h.fileRepo == nil || len(needFileLookup) == 0 {
		return
	}
	files, err := h.fileRepo.GetFirstImagesByPostIDs(givingBoardSlug, needFileLookup)
	if err != nil || len(files) == 0 {
		return
	}
	for i := range items {
		if items[i].Extra10 != "" || items[i].Thumbnail != "" {
			continue
		}
		fname, ok := files[items[i].ID]
		if !ok {
			continue
		}
		if h.cdnURL != "" {
			items[i].Thumbnail = h.cdnURL + "/data/file/" + givingBoardSlug + "/" + fname
		} else {
			items[i].Thumbnail = "data/file/" + givingBoardSlug + "/" + fname
		}
	}
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

	contentExcerpt := "LEFT(g.wr_content, " + strconv.Itoa(givingContentExcerptLen) + ") AS wr_content"
	query := h.db.Table("g5_write_giving AS g").
		Select(`g.wr_id, g.wr_subject, g.wr_4, g.wr_5, g.wr_7, g.wr_10, ` + contentExcerpt + `,
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
	contentByID := make(map[int]string, len(rows))
	for _, r := range rows {
		meta := givingdomain.Normalize(now, givingdomain.Meta{
			StartRaw:         r.Wr4,
			EndRaw:           r.Wr5,
			StateRaw:         r.Wr7,
			ParticipantCount: r.ParticipantCount,
		})

		if !shouldKeep(meta, tab) {
			continue
		}

		contentByID[r.WrID] = r.WrContent
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
		return givingListLess(items[i], items[j], sortBy)
	})
	if len(items) > limit {
		items = items[:limit]
	}

	// Thumbnail enrich (extra_10 → 본문 첫 <img> → g5_board_file).
	// frontend giving-card 의 우선순위 (extra_10 → thumbnail → images[0]) 와 일관.
	h.enrichThumbnails(items, contentByID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}
