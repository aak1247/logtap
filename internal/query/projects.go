package query

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/project"
	"github.com/aak1247/logtap/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ListProjectsHandler(db *gorm.DB) gin.HandlerFunc {
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
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		items, err := store.ListProjectsByOwner(ctx, db, uid)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

func CreateProjectHandler(db *gorm.DB) gin.HandlerFunc {
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
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		p, err := store.CreateProject(ctx, db, uid, req.Name)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, p)
	}
}

func GetProjectHandler(db *gorm.DB) gin.HandlerFunc {
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
		id, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		p, ok, err := store.GetProjectByID(ctx, db, id)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if !ok || p.OwnerUserID != uid {
			respondErr(c, http.StatusNotFound, "not found")
			return
		}
		respondOK(c, p)
	}
}

func ListProjectKeysHandler(db *gorm.DB) gin.HandlerFunc {
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
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		p, ok, err := store.GetProjectByID(ctx, db, projectID)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if !ok || p.OwnerUserID != uid {
			respondErr(c, http.StatusNotFound, "not found")
			return
		}
		keys, err := store.ListProjectKeys(ctx, db, projectID)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": keys})
	}
}

func CreateProjectKeyHandler(db *gorm.DB) gin.HandlerFunc {
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
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		p, ok, err := store.GetProjectByID(ctx, db, projectID)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if !ok || p.OwnerUserID != uid {
			respondErr(c, http.StatusNotFound, "not found")
			return
		}

		key, err := store.CreateProjectKey(ctx, db, projectID, req.Name)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, key)
	}
}

func RevokeProjectKeyHandler(db *gorm.DB) gin.HandlerFunc {
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
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		keyID64, err := strconv.ParseInt(strings.TrimSpace(c.Param("keyId")), 10, 32)
		if err != nil || keyID64 <= 0 {
			respondErr(c, http.StatusBadRequest, "invalid keyId")
			return
		}
		keyID := int(keyID64)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		p, ok, err := store.GetProjectByID(ctx, db, projectID)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if !ok || p.OwnerUserID != uid {
			respondErr(c, http.StatusNotFound, "not found")
			return
		}

		revoked, err := store.RevokeProjectKey(ctx, db, projectID, keyID, time.Now())
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"revoked": revoked})
	}
}
