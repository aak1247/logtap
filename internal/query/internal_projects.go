package query

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/auth"
	"github.com/aak1247/logtap/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// InternalCreateProjectHandler is a trusted upstream-only API for logtap-cloud.
// It must be protected by LOGTAP_PROXY_SECRET at the router level.
func InternalCreateProjectHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}

		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			req.Name = "Untitled"
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		uid, err := ensureCloudOwnerUser(ctx, db)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		p, err := store.CreateProject(ctx, db, uid, req.Name)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"id": p.ID, "name": p.Name})
	}
}

func ensureCloudOwnerUser(ctx context.Context, db *gorm.DB) (int64, error) {
	const email = "cloud@logtap.local"

	u, ok, err := store.GetUserByEmail(ctx, db, email)
	if err != nil {
		return 0, err
	}
	if ok {
		return u.ID, nil
	}

	hash, err := auth.HashPassword("disabled-login")
	if err != nil {
		return 0, err
	}
	uid, err := store.CreateUser(ctx, db, email, hash)
	if err != nil {
		if isUniqueViolation(err) {
			u, ok, err := store.GetUserByEmail(ctx, db, email)
			if err != nil {
				return 0, err
			}
			if ok {
				return u.ID, nil
			}
		}
		return 0, err
	}
	return uid, nil
}
