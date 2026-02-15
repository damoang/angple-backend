package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIResponse standard API response structure
type APIResponse struct {
	Data  interface{} `json:"data"`
	Meta  *Meta       `json:"meta,omitempty"`
	Error *ErrorInfo  `json:"error,omitempty"`
}

// Meta pagination and additional metadata
type Meta struct {
	BoardID string `json:"board_id,omitempty"`
	Page    int    `json:"page,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Total   int64  `json:"total,omitempty"`
}

// ErrorInfo error details
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// SuccessResponse returns a successful JSON response
func SuccessResponse(c *gin.Context, data interface{}, meta *Meta) {
	c.JSON(http.StatusOK, APIResponse{
		Data: data,
		Meta: meta,
	})
}

// ErrorResponse returns an error JSON response
func ErrorResponse(c *gin.Context, status int, message string, err error) {
	errInfo := &ErrorInfo{
		Code:    getErrorCode(status),
		Message: message,
	}

	c.JSON(status, gin.H{
		"error": errInfo,
	})
}

// getErrorCode generates error code from HTTP status
func getErrorCode(status int) string {
	switch status {
	case 400:
		return "BAD_REQUEST"
	case 401:
		return "UNAUTHORIZED"
	case 403:
		return "FORBIDDEN"
	case 404:
		return "NOT_FOUND"
	case 409:
		return "CONFLICT"
	case 500:
		return "INTERNAL_SERVER_ERROR"
	default:
		return "ERROR"
	}
}
