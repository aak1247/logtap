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

type UserDTO struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

func BootstrapHandler(db *gorm.DB, authSecret []byte, tokenTTL time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		if len(authSecret) == 0 {
			respondErr(c, http.StatusServiceUnavailable, "AUTH_SECRET not configured")
			return
		}

		var req struct {
			Email       string `json:"email"`
			Password    string `json:"password"`
			ProjectName string `json:"project_name"`
			KeyName     string `json:"key_name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		req.Email = strings.TrimSpace(req.Email)
		req.ProjectName = strings.TrimSpace(req.ProjectName)
		req.KeyName = strings.TrimSpace(req.KeyName)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		n, err := store.CountUsers(ctx, db)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if n > 0 {
			respondErr(c, http.StatusConflict, "already initialized")
			return
		}

		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		uid, err := store.CreateUser(ctx, db, req.Email, hash)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		project, err := store.CreateProject(ctx, db, uid, firstNonEmpty(req.ProjectName, "Default"))
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		key, err := store.CreateProjectKey(ctx, db, project.ID, firstNonEmpty(req.KeyName, "default"))
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		token, err := auth.SignToken(authSecret, uid, time.Now().Add(tokenTTL))
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		respondOK(c, gin.H{
			"token": token,
			"user":  UserDTO{ID: uid, Email: strings.ToLower(req.Email)},
			"project": gin.H{
				"id":   project.ID,
				"name": project.Name,
			},
			"key": gin.H{
				"id":   key.ID,
				"name": key.Name,
				"key":  key.Key,
			},
		})
	}
}

func LoginHandler(db *gorm.DB, authSecret []byte, tokenTTL time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		if len(authSecret) == 0 {
			respondErr(c, http.StatusServiceUnavailable, "AUTH_SECRET not configured")
			return
		}

		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		req.Email = strings.TrimSpace(req.Email)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		u, ok, err := store.GetUserByEmail(ctx, db, req.Email)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if !ok || !auth.CheckPassword(u.PasswordHash, req.Password) {
			respondErr(c, http.StatusUnauthorized, "invalid credentials")
			return
		}

		token, err := auth.SignToken(authSecret, u.ID, time.Now().Add(tokenTTL))
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{
			"token": token,
			"user":  UserDTO{ID: u.ID, Email: u.Email},
		})
	}
}

func MeHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		uid := userIDFromGin(c)
		if uid <= 0 {
			respondErr(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		u, ok, err := store.GetUserByID(ctx, db, uid)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if !ok {
			respondErr(c, http.StatusUnauthorized, "unauthorized")
			return
		}
		respondOK(c, gin.H{"user": UserDTO{ID: u.ID, Email: u.Email}})
	}
}

func userIDFromGin(c *gin.Context) int64 {
	v, ok := c.Get("user_id")
	if !ok {
		return 0
	}
	id, ok := v.(int64)
	if !ok || id <= 0 {
		return 0
	}
	return id
}

func firstNonEmpty(s string, def string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	return s
}
