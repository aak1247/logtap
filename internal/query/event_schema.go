package query

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ListEventDefinitionsHandler returns all event definitions for a project.
// GET /api/:projectId/events/schema
func ListEventDefinitionsHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		q := db.WithContext(ctx).Where("project_id = ?", projectID)
		status := strings.TrimSpace(c.Query("status"))
		if status != "" {
			q = q.Where("status = ?", status)
		}
		if search := strings.TrimSpace(c.Query("q")); search != "" {
			like := "%" + strings.ToLower(search) + "%"
			q = q.Where("LOWER(name) LIKE ? OR LOWER(display_name) LIKE ?", like, like)
		}

		var items []model.EventDefinition
		if err := q.Order("name ASC").Find(&items).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

// CreateEventDefinitionHandler creates a new event definition for the project.
// POST /api/:projectId/events/schema
func CreateEventDefinitionHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		var req struct {
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
			Category    string `json:"category"`
			Description string `json:"description"`
			Status      string `json:"status"`
			Owner       string `json:"owner"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			respondErr(c, http.StatusBadRequest, "name required")
			return
		}
		if len(name) > 255 {
			name = name[:255]
		}
		displayName := strings.TrimSpace(req.DisplayName)
		if displayName == "" {
			displayName = name
		}
		if len(displayName) > 255 {
			displayName = displayName[:255]
		}
		category := strings.TrimSpace(req.Category)
		if len(category) > 100 {
			category = category[:100]
		}
		status := strings.TrimSpace(req.Status)
		if status == "" {
			status = "active"
		}
		owner := strings.TrimSpace(req.Owner)
		if len(owner) > 255 {
			owner = owner[:255]
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		// Ensure no duplicate (project_id, name).
		var existing model.EventDefinition
		if err := db.WithContext(ctx).
			Where("project_id = ? AND name = ?", projectID, name).
			First(&existing).Error; err == nil {
			respondErr(c, http.StatusConflict, "event already defined")
			return
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		row := model.EventDefinition{
			ProjectID:   projectID,
			Name:        name,
			DisplayName: displayName,
			Category:    category,
			Description: req.Description,
			Status:      status,
			Owner:       owner,
		}
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, row)
	}
}

// UpdateEventDefinitionHandler updates an existing event definition identified by name.
// PUT /api/:projectId/events/schema/:eventName
func UpdateEventDefinitionHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		projectID, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		name := strings.TrimSpace(c.Param("eventName"))
		if name == "" {
			respondErr(c, http.StatusBadRequest, "eventName required")
			return
		}

		var req struct {
			DisplayName string `json:"display_name"`
			Category    string `json:"category"`
			Description string `json:"description"`
			Status      string `json:"status"`
			Owner       string `json:"owner"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var row model.EventDefinition
		if err := db.WithContext(ctx).
			Where("project_id = ? AND name = ?", projectID, name).
			First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "event definition not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		displayName := strings.TrimSpace(req.DisplayName)
		if displayName == "" {
			displayName = row.Name
		}
		if len(displayName) > 255 {
			displayName = displayName[:255]
		}
		category := strings.TrimSpace(req.Category)
		if len(category) > 100 {
			category = category[:100]
		}
		status := strings.TrimSpace(req.Status)
		if status == "" {
			status = row.Status
		}
		owner := strings.TrimSpace(req.Owner)
		if owner == "" {
			owner = row.Owner
		}
		if len(owner) > 255 {
			owner = owner[:255]
		}

		row.DisplayName = displayName
		row.Category = category
		row.Description = req.Description
		row.Status = status
		row.Owner = owner

		if err := db.WithContext(ctx).Save(&row).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, row)
	}
}
