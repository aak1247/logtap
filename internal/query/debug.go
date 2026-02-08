package query

import (
	"net/http"

	"github.com/aak1247/logtap/internal/obs"
	"github.com/gin-gonic/gin"
)

func DebugMetricsHandler(stats *obs.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		if stats == nil {
			respondErr(c, http.StatusNotImplemented, "stats not configured")
			return
		}
		respondOK(c, stats.Snapshot())
	}
}
