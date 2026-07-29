package v2

// reaction_bridge.go — da_reaction(g5_da_reaction / g5_da_reaction_choose) 을 v2 API 로 서빙.
//
// 배경: 앱 리액션이 v2 라우트 부재로 404 였고, 표시조차 good_count 를 👍 로 보여주는 가짜였다.
// 근본 해결: 웹(damoang.net)이 쓰는 것과 "동일한 테이블·동일한 로직"을 v2 가 소유 → 앱↔웹 양방향 SSOT.
// 웹 출처: angple apps/web/src/routes/api/reactions/+server.ts, lib/server/reactions.ts.
//
// ID 규약: target_id = document:{boardSlug}:{wrID} (게시글). reaction 포맷 = category:id (예 emoji:1f44d).
// 스키마(참조: native_app_data_reference.html §6):
//   g5_da_reaction(id, parent_id, target_id, reaction, reaction_count, metadata json, UNIQUE(target_id,reaction), KEY(parent_id))
//   g5_da_reaction_choose(id, member_id, parent_id, target_id, reaction, created_at, chosen_ip, UNIQUE(member_id,target_id,reaction))

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const reactionKindLimit = 20 // 대상당 리액션 "종류" 최대 개수(비관리자). 웹 REACTION_LIMIT 동일.

// reactionCharset: 웹 VALID_REACTION_PATTERN / sanitizeId 와 동일 문자집합.
var reactionInvalidChars = regexp.MustCompile(`[^a-zA-Z0-9:_-]`)
var reactionValidPattern = regexp.MustCompile(`^[a-zA-Z0-9:_-]+$`)

func sanitizeReactionID(s string) string {
	return reactionInvalidChars.ReplaceAllString(s, "")
}

// reactionAggRow: g5_da_reaction 집계 행.
type reactionAggRow struct {
	TargetID      string `gorm:"column:target_id"`
	Reaction      string `gorm:"column:reaction"`
	ReactionCount int    `gorm:"column:reaction_count"`
}

// reactionItem: 앱/웹 공통 응답 아이템(웹 parseReaction 미러).
func reactionItem(reaction string, count int, choose bool) map[string]any {
	category := reaction
	reactionID := reaction
	if idx := strings.IndexByte(reaction, ':'); idx >= 0 {
		category = reaction[:idx]
		reactionID = reaction[idx+1:]
	}
	return map[string]any{
		"reaction":   reaction,
		"category":   category,
		"reactionId": reactionID,
		"count":      count,
		"choose":     choose,
	}
}

// fetchReactionsForTarget 는 단일 target 의 집계 + (로그인 시) 유저 선택을 합쳐 응답 result 를 만든다.
// 반환: { "<targetID>": [reactionItem...] }. 리액션 없으면 빈 배열.
func (h *V2Handler) fetchReactionsForTarget(targetID, memberID string) map[string]any {
	items := []map[string]any{}
	if h.gnuDB == nil {
		return map[string]any{targetID: items}
	}
	var rows []reactionAggRow
	h.gnuDB.Table("g5_da_reaction").
		Select("target_id, reaction, reaction_count").
		Where("target_id = ?", targetID).
		Order("id ASC").
		Scan(&rows)

	chosen := map[string]bool{}
	if memberID != "" {
		var choiceRows []struct {
			Reaction string `gorm:"column:reaction"`
		}
		h.gnuDB.Table("g5_da_reaction_choose").
			Select("reaction").
			Where("member_id = ? AND target_id = ?", memberID, targetID).
			Scan(&choiceRows)
		for _, cr := range choiceRows {
			chosen[cr.Reaction] = true
		}
	}
	for _, r := range rows {
		items = append(items, reactionItem(r.Reaction, r.ReactionCount, chosen[r.Reaction]))
	}
	return map[string]any{targetID: items}
}

// GetPostReactions handles GET /api/v2/boards/:slug/posts/:id/reactions
func (h *V2Handler) GetPostReactions(c *gin.Context) {
	slug := c.Param("slug")
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil || wrID <= 0 {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}
	targetID := postTargetID(slug, wrID)
	memberID := middleware.GetUserID(c) // 비로그인 시 "" → choose 전부 false
	c.JSON(http.StatusOK, gin.H{"status": "success", "result": h.fetchReactionsForTarget(targetID, memberID)})
}

// postReactionBody: POST 바디. reactionMode: add(항상추가)/toggle|""(있으면제거) — 웹과 동일.
type postReactionBody struct {
	Reaction     string `json:"reaction"`
	ReactionMode string `json:"reactionMode"`
}

// PostPostReaction handles POST /api/v2/boards/:slug/posts/:id/reactions
// 가드: 로그인(라우트 auth) + 이용제한(라우트 banCheck) + 실명인증(checkCertification) +
//       리액션 sanitize/정규식/길이 + 종류 20개 제한(비관리자). 자기글 방지는 댓글 전용이라 게시글엔 미적용.
func (h *V2Handler) PostPostReaction(c *gin.Context) {
	slug := c.Param("slug")
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil || wrID <= 0 {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}
	if h.gnuDB == nil {
		common.V2ErrorResponse(c, http.StatusServiceUnavailable, "리액션을 사용할 수 없습니다", nil)
		return
	}

	var body postReactionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}
	// 웹과 동일: reactionMode 기본값은 "add"(있으면 유지=no-op). 토글하려면 클라이언트가 "toggle" 명시.
	if body.ReactionMode == "" {
		body.ReactionMode = "add"
	}
	reaction := sanitizeReactionID(body.Reaction)
	if reaction == "" || len(reaction) > 250 || !reactionValidPattern.MatchString(reaction) {
		common.V2ErrorResponse(c, http.StatusBadRequest, "유효하지 않은 리액션입니다", nil)
		return
	}

	// 실명인증 게이트(관리자 바이패스 포함) — 기존 헬퍼 재사용.
	if certErr := h.checkCertification(c, slug); certErr != "" {
		common.V2ErrorResponse(c, http.StatusForbidden, certErr, nil)
		return
	}

	memberID := middleware.GetUserID(c)
	if memberID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}
	targetID := postTargetID(slug, wrID)
	parentID := targetID // 게시글은 target=parent

	// 기존 선택 여부
	exists, err := h.reactionChoiceExists(memberID, targetID, reaction)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "리액션 처리 실패", err)
		return
	}

	if !exists {
		// 리액션 종류 20개 제한(비관리자, add 시). 이미 존재 리액션이면 통과(위 exists=false 이므로 신규).
		if middleware.GetUserLevel(c) < 10 {
			var kinds int64
			h.gnuDB.Table("g5_da_reaction").Where("target_id = ?", targetID).Count(&kinds)
			if kinds >= reactionKindLimit {
				common.V2ErrorResponse(c, http.StatusBadRequest, "리액션은 최대 20개까지 가능합니다", nil)
				return
			}
		}
		if err := h.addReaction(memberID, reaction, targetID, parentID, clientIPForReaction(c)); err != nil {
			common.V2ErrorResponse(c, http.StatusInternalServerError, "리액션 처리 실패", err)
			return
		}
	} else if body.ReactionMode != "add" {
		// toggle/default: 있으면 취소
		if err := h.revokeReaction(memberID, reaction, targetID); err != nil {
			common.V2ErrorResponse(c, http.StatusInternalServerError, "리액션 처리 실패", err)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "result": h.fetchReactionsForTarget(targetID, memberID)})
}

// postTargetID 는 게시글 target_id 를 만든다. board slug 는 sanitize 하여 주입값 오염을 막는다.
func postTargetID(slug string, wrID int) string {
	return "document:" + sanitizeReactionID(slug) + ":" + strconv.Itoa(wrID)
}

// clientIPForReaction: chosen_ip 기록용. ⚠️ 앱 응답엔 절대 싣지 않음(내부 전용).
// Phase1 은 평문 IP. superadmin/지정멤버 IP 치환(웹 protectClientIp)은 Phase2 감사 보완.
func clientIPForReaction(c *gin.Context) string {
	return c.ClientIP()
}

func (h *V2Handler) reactionChoiceExists(memberID, targetID, reaction string) (bool, error) {
	var cnt int64
	err := h.gnuDB.Table("g5_da_reaction_choose").
		Where("member_id = ? AND target_id = ? AND reaction = ?", memberID, targetID, reaction).
		Count(&cnt).Error
	return cnt > 0, err
}

// addReaction: 웹 addReaction 복제 — (트랜잭션) choose INSERT + 집계 UPSERT.
func (h *V2Handler) addReaction(memberID, reaction, targetID, parentID, clientIP string) error {
	return h.gnuDB.Transaction(func(tx *gorm.DB) error {
		var ip any
		if clientIP != "" {
			ip = clientIP
		}
		if err := tx.Exec(
			`INSERT INTO g5_da_reaction_choose
			 (member_id, reaction, target_id, parent_id, created_at, chosen_ip)
			 VALUES (?, ?, ?, ?, CONVERT_TZ(UTC_TIMESTAMP(), '+00:00', '+09:00'), ?)`,
			memberID, reaction, targetID, parentID, ip,
		).Error; err != nil {
			return err
		}
		return tx.Exec(
			`INSERT INTO g5_da_reaction (reaction, target_id, parent_id, reaction_count)
			 VALUES (?, ?, ?, 1)
			 ON DUPLICATE KEY UPDATE reaction_count = reaction_count + 1`,
			reaction, targetID, parentID,
		).Error
	})
}

// revokeReaction: 웹 revokeReaction 복제 — (트랜잭션) choose DELETE + 집계 감소 + 0이하면 행 삭제.
func (h *V2Handler) revokeReaction(memberID, reaction, targetID string) error {
	return h.gnuDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`DELETE FROM g5_da_reaction_choose
			 WHERE member_id = ? AND target_id = ? AND reaction = ?`,
			memberID, targetID, reaction,
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`UPDATE g5_da_reaction SET reaction_count = reaction_count - 1
			 WHERE target_id = ? AND reaction = ?`,
			targetID, reaction,
		).Error; err != nil {
			return err
		}
		return tx.Exec(
			`DELETE FROM g5_da_reaction
			 WHERE target_id = ? AND reaction = ? AND reaction_count <= 0`,
			targetID, reaction,
		).Error
	})
}
