package ginutil

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// QueryInt extracts an integer from query parameters with default value
func QueryInt(c *gin.Context, key string, defaultValue int) int {
	valueStr := c.Query(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}

	return value
}

// ParamInt extracts an integer from path parameters
// Returns the parsed int and error if parsing fails
func ParamInt(c *gin.Context, key string) (int, error) {
	valueStr := c.Param(key)
	return strconv.Atoi(valueStr)
}

// ParamInt64 extracts an int64 from path parameters
// Returns the parsed int64 and error if parsing fails
func ParamInt64(c *gin.Context, key string) (int64, error) {
	valueStr := c.Param(key)
	return strconv.ParseInt(valueStr, 10, 64)
}
