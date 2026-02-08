package httpserver

import (
	"time"

	"github.com/aak1247/logtap/internal/obs"
	"github.com/gin-gonic/gin"
)

func observabilityMiddleware(stats *obs.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		stats.ObserveHTTP(c.Writer.Status(), time.Since(start))
	}
}
