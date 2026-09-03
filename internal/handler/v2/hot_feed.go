package v2

// hot_feed.go — 크로스보드 핫(공감) 피드 (GET /api/v2/feed/hot).
//
// 앱 공감 탭의 v1 empathy API 는 g5_write_empathy 테이블이 없어 항상 빈 목록을
// 반환했다(전 사용자 빈 화면). 웹 공감글은 cron JSON 파일이라 재사용할 수 없어
// 라이브 SQL 집계(FindHotAcrossBoards)로 서빙한다. 응답 아이템은 /feed 와 동일한
// V2Post 형태(+excerpt) — 앱이 같은 렌더러를 재사용할 수 있다.

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/gin-gonic/gin"
)

const (
	hotFeedDefaultLimit = 20
	hotFeedMaxLimit     = 30
)

// hotFeedLimit 은 ?limit= 값을 정규화한다. 기본 20, 허용 1..30(밖이면 기본값).
// /feed(ListRecentFeed)와 같은 규칙 — 초과값을 30 으로 조용히 늘리지 않는다.
func hotFeedLimit(raw string) int {
	if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= hotFeedMaxLimit {
		return v
	}
	return hotFeedDefaultLimit
}

// hotFeedPage 는 ?page= 값을 정규화한다. 기본 1, 1 미만·비숫자는 1.
// 후보 풀이 바운디드(보드당 2000행 캡)라 offset 페이지네이션을 허용한다.
func hotFeedPage(raw string) int {
	if v, err := strconv.Atoi(raw); err == nil && v > 0 {
		return v
	}
	return 1
}

// ListHotFeed handles GET /api/v2/feed/hot — 최근 hours 시간 내 공감글 공감순 피드.
// 쿼리: hours(6/12/24/72, 기본 24) · limit(기본 20 최대 30) · page(기본 1).
// 차단 사용자 글은 SQL 에서 제외(OptionalJWTAuth + getBlockedMbIDs, /feed 와 동일).
func (h *V2Handler) ListHotFeed(c *gin.Context) {
	if h.feedRepo == nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "피드를 사용할 수 없습니다", nil)
		return
	}
	rawHours, _ := strconv.Atoi(c.Query("hours"))
	hours := gnurepo.NormalizeHotHours(rawHours)
	limit := hotFeedLimit(c.Query("limit"))
	page := hotFeedPage(c.Query("page"))

	blockedMbIDs, blockErr := h.getBlockedMbIDs(c, middleware.GetUserID(c))
	if blockErr != nil {
		// ⛔ 차단 목록을 모르는 채로 내보내면 차단한 사람의 글이 보인다. fail-closed.
		common.V2ErrorResponse(c, http.StatusInternalServerError, "목록을 불러오지 못했습니다", blockErr)
		return
	}

	rows, hasMore, err := h.feedRepo.FindHotAcrossBoards(hours, limit, (page-1)*limit, blockedMbIDs)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "피드 조회 실패", err)
		return
	}

	// 게시판 이름 + 작성자 해석 (ListRecentFeed 와 동일 패턴)
	boardNames := map[string]string{}
	mbIDs := make([]string, 0, len(rows))
	for i := range rows {
		slug := rows[i].BoardID
		if _, ok := boardNames[slug]; !ok {
			boardNames[slug] = slug
			if h.gnuBoardRepo != nil {
				if gb, e := h.gnuBoardRepo.FindByID(slug); e == nil {
					boardNames[slug] = gb.BoSubject
				}
			}
		}
		mbIDs = append(mbIDs, rows[i].MbID)
	}
	authors := h.resolveLiveAuthors(mbIDs)

	items := make([]map[string]any, len(rows))
	for i := range rows {
		slug := rows[i].BoardID
		items[i] = h.toV2Post(&rows[i].G5Write, 0, slug, boardNames[slug], false, authors, false)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
		"meta":    gin.H{"page": page, "has_more": hasMore, "hours": hours},
	})
}
