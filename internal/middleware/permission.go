package middleware

import (
	"errors"
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/gin-gonic/gin"
)

// PermissionAction represents the type of action being performed
type PermissionAction string

const (
	ActionList     PermissionAction = "list"
	ActionRead     PermissionAction = "read"
	ActionWrite    PermissionAction = "write"
	ActionReply    PermissionAction = "reply"
	ActionComment  PermissionAction = "comment"
	ActionUpload   PermissionAction = "upload"
	ActionDownload PermissionAction = "download"
)

// BoardPermissionChecker interface for checking board permissions
// This avoids circular dependency with service package
type BoardPermissionChecker interface {
	CanList(boardID string, memberLevel int) (bool, error)
	CanRead(boardID string, memberLevel int) (bool, error)
	CanWrite(boardID string, memberLevel int) (bool, error)
	CanComment(boardID string, memberLevel int) (bool, error)
	GetRequiredLevel(boardID string, action string) int
}

// BoardPermission returns a middleware that checks user's permission level for a board
// It requires JWTAuth middleware to be applied first
func BoardPermission(checker BoardPermissionChecker, action PermissionAction) gin.HandlerFunc {
	return func(c *gin.Context) {
		boardID := c.Param("board_id")
		if boardID == "" {
			common.ErrorResponse(c, http.StatusBadRequest, "게시판 ID가 필요합니다", nil)
			c.Abort()
			return
		}

		// Get member level from context (set by JWTAuth)
		memberLevel := getMemberLevel(c)

		// Check permission based on action
		var canAccess bool
		var err error
		var requiredLevel int

		switch action {
		case ActionList:
			canAccess, err = checker.CanList(boardID, memberLevel)
			requiredLevel = checker.GetRequiredLevel(boardID, "list")
		case ActionRead:
			canAccess, err = checker.CanRead(boardID, memberLevel)
			requiredLevel = checker.GetRequiredLevel(boardID, "read")
		case ActionWrite:
			canAccess, err = checker.CanWrite(boardID, memberLevel)
			requiredLevel = checker.GetRequiredLevel(boardID, "write")
		case ActionComment:
			canAccess, err = checker.CanComment(boardID, memberLevel)
			requiredLevel = checker.GetRequiredLevel(boardID, "comment")
		default:
			// For unsupported actions, allow by default
			c.Next()
			return
		}

		if err != nil {
			// Board not found or other error
			if errors.Is(err, common.ErrNotFound) {
				common.ErrorResponse(c, http.StatusNotFound, "게시판을 찾을 수 없습니다", err)
			} else {
				common.ErrorResponse(c, http.StatusInternalServerError, "권한 확인 중 오류가 발생했습니다", err)
			}
			c.Abort()
			return
		}

		if !canAccess {
			common.ErrorResponse(c, http.StatusForbidden, formatPermissionError(action, requiredLevel, memberLevel), nil)
			c.Abort()
			return
		}

		// Store permission info in context for later use
		c.Set("board_permission_checked", true)
		c.Set("member_level", memberLevel)

		c.Next()
	}
}

// RequireWrite is a convenience function for write permission check
func RequireWrite(checker BoardPermissionChecker) gin.HandlerFunc {
	return BoardPermission(checker, ActionWrite)
}

// RequireComment is a convenience function for comment permission check
func RequireComment(checker BoardPermissionChecker) gin.HandlerFunc {
	return BoardPermission(checker, ActionComment)
}

// RequireRead is a convenience function for read permission check
func RequireRead(checker BoardPermissionChecker) gin.HandlerFunc {
	return BoardPermission(checker, ActionRead)
}

// getMemberLevel extracts member level from context
func getMemberLevel(c *gin.Context) int {
	if level := GetUserLevel(c); level > 0 {
		return level
	}
	return 1
}

// formatPermissionError generates a user-friendly error message
func formatPermissionError(action PermissionAction, requiredLevel, currentLevel int) string {
	actionName := getActionName(action)
	if currentLevel == 1 {
		return actionName + " 권한이 없습니다. 로그인이 필요하거나 레벨 " + itoa(requiredLevel) + " 이상이 필요합니다."
	}
	return actionName + " 권한이 없습니다. 레벨 " + itoa(requiredLevel) + " 이상이 필요합니다. (현재 레벨: " + itoa(currentLevel) + ")"
}

// getActionName returns Korean name for the action
func getActionName(action PermissionAction) string {
	switch action {
	case ActionList:
		return "목록 보기"
	case ActionRead:
		return "글 읽기"
	case ActionWrite:
		return "글쓰기"
	case ActionReply:
		return "답글 작성"
	case ActionComment:
		return "댓글 작성"
	case ActionUpload:
		return "파일 업로드"
	case ActionDownload:
		return "파일 다운로드"
	default:
		return "해당 작업"
	}
}

// itoa is a simple int to string converter
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
