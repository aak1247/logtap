package query

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// API response envelope.
//
// Success:
//
//	{"code":0,"data":...}
//
// Error:
func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": data,
	})
}

func respondErr(c *gin.Context, status int, errMsg string) {
	errMsg = strings.TrimSpace(errMsg)
	if errMsg == "" {
		errMsg = http.StatusText(status)
	}
	c.JSON(status, gin.H{
		"code": status,
		"err":  errMsg,
	})
}
