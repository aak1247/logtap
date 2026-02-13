package httpserver

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func errorLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		if c.Request == nil || c.Request.Method == http.MethodOptions {
			return
		}
		path := c.Request.URL.Path
		if path == "" || !strings.HasPrefix(path, "/api") {
			return
		}
		if path == "/healthz" || path == "/api/status" {
			return
		}

		status := c.Writer.Status()
		if status < 400 {
			return
		}

		errStr := strings.TrimSpace(c.Errors.String())
		lat := time.Since(start).Truncate(time.Millisecond)
		if errStr != "" {
			log.Printf("http error: status=%d method=%s path=%s latency=%s err=%q", status, c.Request.Method, path, lat, errStr)
			return
		}
		log.Printf("http error: status=%d method=%s path=%s latency=%s", status, c.Request.Method, path, lat)
	}
}
