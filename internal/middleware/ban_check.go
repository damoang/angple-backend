package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	interceptDateFormat      = "2006-01-02 15:04:05"
	interceptDateDashFormat  = "2006-01-02"
	interceptDateShortFormat = "20060102"
	promotionBoardSlug       = "promotion"
	claimBoardSlug           = "claim"
)

// banCheckKST 는 이용제한 종료 시각을 읽을 때 쓰는 로케이션이다.
//
// ⛔ 런타임의 time.Local 에 기대지 않는다. 컨테이너에 TZ 와 zoneinfo 가 모두 있는데도
// 실측상 시각 판정이 정확히 9시간 어긋났다(2026-09-04). mb_intercept_date 는 KST
// 벽시계로 기록되므로, 환경 설정에 의존하지 않도록 로케이션을 코드에 고정한다.
var banCheckKST = time.FixedZone("KST", 9*60*60)

// BanCheck checks if the authenticated user is banned (mb_intercept_date).
// Banned users cannot create or update posts/comments.
// Exception: banned users can only write/comment on the promotion board.
// This checks all restriction scopes (equivalent to BanCheckScoped("all")).
func BanCheck(gnuDB *gorm.DB) gin.HandlerFunc {
	return BanCheckScoped(gnuDB, "all")
}

// BanCheckScoped checks if the authenticated user is banned for a specific scope.
// scope: "all" (any restriction), "write", "comment", "reaction"
func BanCheckScoped(gnuDB *gorm.DB, scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		mbID := GetUserID(c)
		if mbID == "" {
			c.Next()
			return
		}

		// PK lookup: get intercept date
		var interceptDate string
		err := gnuDB.Table("g5_member").
			Select("mb_intercept_date").
			Where("mb_id = ?", mbID).
			Row().Scan(&interceptDate)
		// mb_intercept_date가 비어있거나, 파싱 실패하거나, 만료된 경우 → fallback으로 discipline 테이블 재확인
		needsFallback := (err != nil || interceptDate == "")
		if !needsFallback {
			banEnd, parseErr := parseInterceptDate(interceptDate)
			if parseErr != nil {
				needsFallback = true
			} else if time.Now().After(banEnd) {
				needsFallback = true
			}
		}

		// ⭐주의(penalty_period = 0)는 이용제한이 아니다.
		//
		// 회원에게는 「주의(이용제한 없음)」로 통보되는데, mb_intercept_date 에 날짜가
		// 들어가 있으면 parseInterceptDate 가 그 날 23:59:59 까지로 읽어(varchar(8) 은
		// 시각을 담지 못한다) 통보 내용과 달리 글쓰기가 막힌다.
		//
		// 정본은 g5_da_member_discipline 이다. 정본에 행이 있는데 활성 제재가 하나도
		// 없으면(주의뿐이거나 이미 만료) 파생값을 믿지 않는다. 만료된 제재가 크론 주기
		// 사이에 남아 있는 경우도 여기서 함께 걸러진다.
		//
		// ⛔정본 행이 아예 없는 경우는 종전대로 둔다 — 정본 없이 mb_intercept_date 만
		//    설정하는 경로가 있고, 그것까지 무시하면 제재가 풀린다.
		if !needsFallback {
			var rowCount, activeCount int64
			if scanErr := gnuDB.Raw(`
				SELECT COUNT(*),
				       IFNULL(SUM(CASE
					       WHEN penalty_period = -1
					         OR (penalty_period > 0
					             AND DATE_ADD(penalty_date_from, INTERVAL penalty_period DAY) > NOW())
					       THEN 1 ELSE 0 END), 0)
				  FROM g5_da_member_discipline
				 WHERE penalty_mb_id = ?`, mbID,
			).Row().Scan(&rowCount, &activeCount); scanErr == nil &&
				rowCount > 0 && activeCount == 0 {
				c.Next()
				return
			}
		}

		if needsFallback {
			// scope별 매칭: "all" scope는 모든 restriction_scope에 매칭,
			// 특정 scope는 "all" 또는 해당 scope에 매칭
			scopeCondition := "AND restriction_scope IN ('all', ?)"

			var penaltyEndDate string
			fallbackErr := gnuDB.Raw(
				`SELECT CASE
						WHEN penalty_period = -1 THEN '99991231'
						ELSE DATE_FORMAT(DATE_ADD(penalty_date_from, INTERVAL penalty_period DAY), '%Y%m%d')
					END
				 FROM g5_da_member_discipline
				 WHERE penalty_mb_id = ?
				   AND penalty_type IN ('intercept', 'both', 'all', 'level')
				   AND (
						penalty_period = -1
						OR (penalty_period > 0 AND DATE_ADD(penalty_date_from, INTERVAL penalty_period DAY) > NOW())
				   )
				   `+scopeCondition+`
				 ORDER BY id DESC LIMIT 1`, mbID, scope,
			).Row().Scan(&penaltyEndDate)
			if fallbackErr != nil || penaltyEndDate == "" {
				c.Next()
				return
			}
			// 활성 제재 발견 — mb_intercept_date 자동 복구(backfill, datetime 형식)
			gnuDB.Exec("UPDATE g5_member SET mb_intercept_date = ? WHERE mb_id = ?", penaltyEndDate, mbID)
			interceptDate = penaltyEndDate
		}

		// Parse intercept date (end date of ban)
		banEnd, parseErr := parseInterceptDate(interceptDate)
		if parseErr != nil {
			// Unparseable date — treat as not banned
			c.Next()
			return
		}

		if time.Now().After(banEnd) {
			// Ban expired (and no active discipline found in fallback)
			c.Next()
			return
		}

		// scope가 "all"이 아닌 경우, discipline 테이블에서 해당 scope 제한이 실제 있는지 확인
		if scope != "all" {
			var scopeCount int64
			gnuDB.Raw(`
				SELECT COUNT(*) FROM g5_da_member_discipline
				WHERE penalty_mb_id = ?
				  AND restriction_scope IN ('all', ?)
				  AND (
					penalty_period = -1
					OR (penalty_period > 0 AND DATE_ADD(penalty_date_from, INTERVAL penalty_period DAY) > NOW())
				  )`, mbID, scope,
			).Scan(&scopeCount)
			if scopeCount == 0 {
				c.Next()
				return
			}
		}

		// User is currently banned — only promotion and claim boards are allowed
		slug := c.Param("slug")
		if slug != "" && (slug == promotionBoardSlug || slug == claimBoardSlug) {
			c.Next()
			return
		}

		// Block the request
		banEndStr := banEnd.Format("2006-01-02 15:04:05")
		if banEnd.Year() >= 9999 {
			banEndStr = "영구 이용제한"
		}

		scopeLabel := ""
		switch scope {
		case "comment":
			scopeLabel = "댓글 "
		case "reaction":
			scopeLabel = "공감/추천 "
		case "write":
			scopeLabel = "글쓰기 "
		}

		common.ErrorResponse(c, http.StatusForbidden,
			scopeLabel+"이용제한 기간 중에는 해당 기능을 사용할 수 없습니다. (해제일: "+banEndStr+")", nil)
		c.Abort()
	}
}

// archiveBoards is the set of board slugs that are read-only archives.
var archiveBoards = map[string]bool{
	"truthroom": true,
}

// ArchiveBoardCheck blocks PUT, PATCH, DELETE on archive boards (read-only).
// POST (new posts/comments) is still allowed; only modification/deletion is blocked.
func ArchiveBoardCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("slug")
		if !archiveBoards[slug] {
			c.Next()
			return
		}

		switch c.Request.Method {
		case http.MethodPut, http.MethodPatch, http.MethodDelete:
			common.ErrorResponse(c, http.StatusForbidden,
				"아카이브 게시판에서는 수정/삭제가 불가능합니다.", nil)
			c.Abort()
			return
		}

		c.Next()
	}
}

// parseInterceptDate parses mb_intercept_date which can be:
//   - "2006-01-02 15:04:05" (datetime)
//   - "20060102" (short date, varchar(8) native)
//   - "2006-01-" (truncated YYYY-MM-DD stored in varchar(8))
func parseInterceptDate(s string) (time.Time, error) {
	if t, err := time.ParseInLocation(interceptDateFormat, s, banCheckKST); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation(interceptDateDashFormat, s, banCheckKST); err == nil {
		// YYYY-MM-DD format (no time component) — treat as end of day
		return t.Add(24*time.Hour - time.Second), nil
	}
	if t, err := time.ParseInLocation(interceptDateShortFormat, s, banCheckKST); err == nil {
		// Short format has no time component — treat as end of day
		return t.Add(24*time.Hour - time.Second), nil
	}
	// Handle truncated "YYYY-MM-" format (varchar(8) truncation of "YYYY-MM-DD")
	if len(s) == 8 && s[4] == '-' && s[7] == '-' {
		if t, err := time.ParseInLocation("2006-01-", s, time.Local); err == nil {
			// Truncated — treat as last day of that month (conservative: assume banned)
			lastDay := t.AddDate(0, 1, -1)
			return lastDay.Add(24*time.Hour - time.Second), nil
		}
	}
	return time.Time{}, fmt.Errorf("unknown date format: %s", s)
}
