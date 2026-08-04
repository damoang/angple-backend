package v2

// reaction_comment_bridge.go — 댓글 리액션(Phase2) + 스레드 배치 조회.
//
// 게시글 리액션(reaction_bridge.go, Phase1)과 동일한 da_reaction 테이블·헬퍼를 재사용한다.
// 차이(웹 계약 .../comments/[commentId]/reactions/+server.ts):
//   - 댓글 target_id = comment:{slug}:{commentWrID}, parent_id = document:{slug}:{postWrID}
//   - 자기 댓글 리액션 금지(웹 preventWriterReaction: wr_is_comment=1 & 작성자==본인 → 403)
//   - 스레드 배치: parent_id=document:{slug}:{post} 로 글+모든 댓글 리액션을 1쿼리 조회(N+1 방지)

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

func commentTargetID(slug string, commentID int) string {
	return "comment:" + sanitizeReactionID(slug) + ":" + strconv.Itoa(commentID)
}

// GetCommentReactions handles GET /api/v2/boards/:slug/posts/:id/comments/:comment_id/reactions
func (h *V2Handler) GetCommentReactions(c *gin.Context) {
	slug := c.Param("slug")
	commentID, err := strconv.Atoi(c.Param("comment_id"))
	if err != nil || commentID <= 0 {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 댓글 ID", err)
		return
	}
	targetID := commentTargetID(slug, commentID)
	memberID := middleware.GetUserID(c)
	c.JSON(http.StatusOK, gin.H{"status": "success", "result": h.fetchReactionsForTarget(targetID, memberID)})
}

// PostCommentReaction handles POST /api/v2/boards/:slug/posts/:id/comments/:comment_id/reactions
// 가드: 로그인(auth) + 이용제한(banCheck) + 실명인증 + sanitize/정규식/20종 + 자기 댓글 방지(게시글과 달리 적용).
func (h *V2Handler) PostCommentReaction(c *gin.Context) {
	slug := c.Param("slug")
	postID, perr := strconv.Atoi(c.Param("id"))
	commentID, cerr := strconv.Atoi(c.Param("comment_id"))
	if perr != nil || cerr != nil || postID <= 0 || commentID <= 0 {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 ID", nil)
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
	if body.ReactionMode == "" {
		body.ReactionMode = "add"
	}
	reaction := sanitizeReactionID(body.Reaction)
	if reaction == "" || len(reaction) > 250 || !reactionValidPattern.MatchString(reaction) {
		common.V2ErrorResponse(c, http.StatusBadRequest, "유효하지 않은 리액션입니다", nil)
		return
	}

	if certErr := h.checkCertification(c, slug); certErr != "" {
		common.V2ErrorResponse(c, http.StatusForbidden, certErr, nil)
		return
	}

	memberID := middleware.GetUserID(c)
	if memberID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	// 자기 댓글 리액션 방지(웹과 동일 — 댓글에만 적용). g5_write_{slug} 에서 댓글 작성자 확인.
	table := "g5_write_" + sanitizeReactionID(slug)
	var authorMbID string
	h.gnuDB.Table(table).
		Select("mb_id").
		Where("wr_id = ? AND wr_is_comment = 1", commentID).
		Limit(1).
		Scan(&authorMbID)
	if authorMbID != "" && authorMbID == memberID {
		common.V2ErrorResponse(c, http.StatusForbidden, "자신의 댓글에 리액션을 할 수 없습니다", nil)
		return
	}

	targetID := commentTargetID(slug, commentID)
	parentID := postTargetID(slug, postID) // 부모는 "글" (스레드 배치 조회 키)

	exists, err := h.reactionChoiceExists(memberID, targetID, reaction)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "리액션 처리 실패", err)
		return
	}
	if !exists {
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
		if err := h.revokeReaction(memberID, reaction, targetID); err != nil {
			common.V2ErrorResponse(c, http.StatusInternalServerError, "리액션 처리 실패", err)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "result": h.fetchReactionsForTarget(targetID, memberID)})
}

// GetThreadReactions handles GET /api/v2/boards/:slug/posts/:id/reactions?scope=thread
// parent_id=document:{slug}:{post} 로 글 + 모든 댓글 리액션을 1쿼리로 조회(N+1 방지).
// 응답: { "document:slug:post":[...], "comment:slug:c1":[...], ... }.
// scope 미지정이면 기존 단건(GetPostReactions) 동작을 유지(하위호환).
func (h *V2Handler) GetPostOrThreadReactions(c *gin.Context) {
	if c.Query("scope") != "thread" {
		h.GetPostReactions(c)
		return
	}
	slug := c.Param("slug")
	postID, err := strconv.Atoi(c.Param("id"))
	if err != nil || postID <= 0 {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}
	parentID := postTargetID(slug, postID)
	memberID := middleware.GetUserID(c)
	c.JSON(http.StatusOK, gin.H{"status": "success", "result": h.fetchReactionsByParent(parentID, memberID)})
}

// fetchReactionsByParent 는 parent_id 로 글+모든 댓글 리액션을 배치 조회한다(웹 fetchReactionsByParentId).
// 반환: { "<targetID>": [reactionItem...] } (각 target 별 배열). 1~2 쿼리(집계 + 로그인 시 choose).
func (h *V2Handler) fetchReactionsByParent(parentID, memberID string) map[string]any {
	out := map[string]any{}
	if h.gnuDB == nil {
		return out
	}
	var rows []reactionAggRow
	h.gnuDB.Table("g5_da_reaction").
		Select("target_id, reaction, reaction_count").
		Where("parent_id = ?", parentID).
		Order("id ASC").
		Scan(&rows)

	// 유저 선택(choose) 도 parent_id 로 한 번에.
	chosen := map[string]map[string]bool{} // targetID -> reaction -> true
	if memberID != "" {
		var choiceRows []struct {
			TargetID string `gorm:"column:target_id"`
			Reaction string `gorm:"column:reaction"`
		}
		h.gnuDB.Table("g5_da_reaction_choose").
			Select("target_id, reaction").
			Where("member_id = ? AND parent_id = ?", memberID, parentID).
			Scan(&choiceRows)
		for _, cr := range choiceRows {
			if chosen[cr.TargetID] == nil {
				chosen[cr.TargetID] = map[string]bool{}
			}
			chosen[cr.TargetID][cr.Reaction] = true
		}
	}

	for _, r := range rows {
		arr, _ := out[r.TargetID].([]map[string]any)
		isChosen := chosen[r.TargetID] != nil && chosen[r.TargetID][r.Reaction]
		out[r.TargetID] = append(arr, reactionItem(r.Reaction, r.ReactionCount, isChosen))
	}
	return out
}
