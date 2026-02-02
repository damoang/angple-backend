package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// V2Response v2 API 표준 응답 형식 (core-spec-v1.0.md §4.3)
type V2Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Meta    *V2Meta     `json:"meta,omitempty"`
	Error   *V2Error    `json:"error,omitempty"`
}

// V2Meta v2 페이지네이션 메타 (core-spec-v1.0.md §4.4)
type V2Meta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

// V2Error v2 에러 응답 (core-spec-v1.0.md §4.5)
type V2Error struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// NewV2Meta creates V2Meta with computed total_pages
func NewV2Meta(page, perPage int, total int64) *V2Meta {
	totalPages := total / int64(perPage)
	if total%int64(perPage) > 0 {
		totalPages++
	}
	return &V2Meta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
	}
}

// V2Success returns a v2 success response
func V2Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, V2Response{
		Success: true,
		Data:    data,
	})
}

// V2SuccessWithMeta returns a v2 success response with pagination
func V2SuccessWithMeta(c *gin.Context, data interface{}, meta *V2Meta) {
	c.JSON(http.StatusOK, V2Response{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

// V2Created returns a v2 201 Created response
func V2Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, V2Response{
		Success: true,
		Data:    data,
	})
}

// V2ErrorResponse returns a v2 error response
func V2ErrorResponse(c *gin.Context, status int, message string, err error) {
	v2Err := &V2Error{
		Code:    getErrorCode(status),
		Message: message,
	}
	if err != nil {
		v2Err.Details = err.Error()
	}
	c.JSON(status, V2Response{
		Success: false,
		Error:   v2Err,
	})
}
