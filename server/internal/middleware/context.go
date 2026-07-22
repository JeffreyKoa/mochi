package middleware

import (
	"github.com/gin-gonic/gin"
)

func UserID(c *gin.Context) uint64 {
	v, exists := c.Get("userID")
	if !exists {
		return 0
	}
	id, ok := v.(uint64)
	if !ok {
		return 0
	}
	return id
}
