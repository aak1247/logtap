package query

import (
	"context"
	"time"

	"github.com/aak1247/logtap/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SystemStatus string

const (
	SystemStatusUninitialized SystemStatus = "uninitialized"
	SystemStatusRunning       SystemStatus = "running"
	SystemStatusMaintenance   SystemStatus = "maintenance"
	SystemStatusException     SystemStatus = "exception"
)

func StatusHandler(db *gorm.DB, maintenanceMode bool, authEnabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maintenanceMode {
			respondOK(c, gin.H{
				"status":       SystemStatusMaintenance,
				"initialized":  true,
				"auth_enabled": authEnabled,
				"message":      "maintenance",
			})
			return
		}
		if db == nil {
			respondOK(c, gin.H{
				"status":       SystemStatusException,
				"initialized":  false,
				"auth_enabled": authEnabled,
				"message":      "database not configured",
			})
			return
		}
		if !authEnabled {
			respondOK(c, gin.H{
				"status":       SystemStatusException,
				"initialized":  false,
				"auth_enabled": false,
				"message":      "AUTH_SECRET not configured",
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		n, err := store.CountUsers(ctx, db)
		if err != nil {
			respondOK(c, gin.H{
				"status":       SystemStatusException,
				"initialized":  false,
				"auth_enabled": authEnabled,
				"message":      "database unavailable",
			})
			return
		}

		if n == 0 {
			respondOK(c, gin.H{
				"status":       SystemStatusUninitialized,
				"initialized":  false,
				"auth_enabled": authEnabled,
			})
			return
		}
		respondOK(c, gin.H{
			"status":       SystemStatusRunning,
			"initialized":  true,
			"auth_enabled": authEnabled,
		})
	}
}
