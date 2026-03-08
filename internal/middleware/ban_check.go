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
	interceptDateFormat     = "2006-01-02 15:04:05"
	interceptDateShortFormat = "20060102"
	claimBoardSlug          = "claim"
	claimWindowDays         = 15
)

// BanCheck checks if the authenticated user is banned (mb_intercept_date).
// Banned users cannot create or update posts/comments.
// Exception: banned users can CREATE (POST only) on the claim board within 15 days of discipline start.
func BanCheck(gnuDB *gorm.DB) gin.HandlerFunc {
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
		if err != nil || interceptDate == "" {
			c.Next()
			return
		}

		// Parse intercept date (end date of ban)
		banEnd, parseErr := parseInterceptDate(interceptDate)
		if parseErr != nil {
			// Unparseable date — treat as not banned
			c.Next()
			return
		}

		now := time.Now()
		if now.After(banEnd) {
			// Ban expired
			c.Next()
			return
		}

		// User is currently banned — check claim board exception
		slug := c.Param("slug")
		if slug == claimBoardSlug && c.Request.Method == http.MethodPost {
			// Claim board exception: allow POST within 15 days of discipline start
			var penaltyDateFrom string
			err := gnuDB.Raw(
				"SELECT penalty_date_from FROM g5_da_member_discipline WHERE penalty_mb_id = ? ORDER BY id DESC LIMIT 1",
				mbID,
			).Row().Scan(&penaltyDateFrom)
			if err == nil && penaltyDateFrom != "" {
				if disciplineStart, e := time.ParseInLocation(interceptDateFormat, penaltyDateFrom, time.Local); e == nil {
					if now.Sub(disciplineStart) <= time.Duration(claimWindowDays)*24*time.Hour {
						c.Next()
						return
					}
				}
			}
		}

		// Block the request
		banEndStr := banEnd.Format("2006-01-02 15:04:05")
		if banEnd.Year() >= 9999 {
			banEndStr = "영구 제재"
		}
		common.ErrorResponse(c, http.StatusForbidden,
			"이용제한 기간 중에는 해당 기능을 사용할 수 없습니다. (해제일: "+banEndStr+")", nil)
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

// parseInterceptDate parses mb_intercept_date which can be "2006-01-02 15:04:05" or "20060102" format.
func parseInterceptDate(s string) (time.Time, error) {
	if t, err := time.ParseInLocation(interceptDateFormat, s, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation(interceptDateShortFormat, s, time.Local); err == nil {
		// Short format has no time component — treat as end of day
		return t.Add(24*time.Hour - time.Second), nil
	}
	return time.Time{}, fmt.Errorf("unknown date format: %s", s)
}
