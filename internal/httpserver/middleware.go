package httpserver

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Encoding, X-Sentry-Auth, X-Project-Key, X-Requested-With, sentry-trace, baggage, Authorization")

		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next()
	}
}

func maintenanceMiddleware(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}
		switch c.Request.URL.Path {
		case "/healthz", "/api/status":
			c.Next()
			return
		default:
			if strings.HasPrefix(c.Request.URL.Path, "/api/") || c.Request.URL.Path == "/api" {
				c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable, "err": "maintenance"})
			} else {
				c.String(http.StatusServiceUnavailable, "maintenance")
			}
			c.Abort()
			return
		}
	}
}
