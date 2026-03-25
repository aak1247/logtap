package query

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var allowedPropertyTypes = map[string]bool{
	"string": true,
	"enum":   true,
	"number": true,
}

// ListPropertyDefinitionsHandler returns all property definitions for a project.
// GET /api/:projectId/properties/schema
func ListPropertyDefinitionsHandler(db *gorm.DB) gin.HandlerFunc {
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
		if status := strings.TrimSpace(c.Query("status")); status != "" {
			q = q.Where("status = ?", status)
		}
		if typ := strings.TrimSpace(c.Query("type")); typ != "" {
			q = q.Where("type = ?", typ)
		}
		if search := strings.TrimSpace(c.Query("q")); search != "" {
			like := "%" + strings.ToLower(search) + "%"
			q = q.Where("LOWER(key) LIKE ? OR LOWER(display_name) LIKE ?", like, like)
		}

		var items []model.PropertyDefinition
		if err := q.Order("key ASC").Find(&items).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

// CreatePropertyDefinitionHandler creates a new property definition.
// POST /api/:projectId/properties/schema
func CreatePropertyDefinitionHandler(db *gorm.DB) gin.HandlerFunc {
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
			Key          string   `json:"key"`
			DisplayName  string   `json:"display_name"`
			Type         string   `json:"type"`
			Description  string   `json:"description"`
			Status       string   `json:"status"`
			EnumValues   []string `json:"enum_values"`
			ExampleValues []string `json:"example_values"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		key := strings.TrimSpace(req.Key)
		if key == "" {
			respondErr(c, http.StatusBadRequest, "key required")
			return
		}
		if len(key) > 255 {
			key = key[:255]
		}
		displayName := strings.TrimSpace(req.DisplayName)
		if displayName == "" {
			displayName = key
		}
		if len(displayName) > 255 {
			displayName = displayName[:255]
		}
		pt := strings.ToLower(strings.TrimSpace(req.Type))
		if pt == "" {
			pt = "string"
		}
		if !allowedPropertyTypes[pt] {
			respondErr(c, http.StatusBadRequest, "invalid type (expected string|enum|number)")
			return
		}
		status := strings.TrimSpace(req.Status)
		if status == "" {
			status = "active"
		}

		var enumJSON datatypes.JSON
		if pt == "enum" {
			if len(req.EnumValues) == 0 {
				respondErr(c, http.StatusBadRequest, "enum_values required for type=enum")
				return
			}
			b, err := json.Marshal(req.EnumValues)
			if err != nil {
				respondErr(c, http.StatusBadRequest, "invalid enum_values")
				return
			}
			enumJSON = datatypes.JSON(b)
		}

		var exampleJSON datatypes.JSON
		if len(req.ExampleValues) > 0 {
			b, err := json.Marshal(req.ExampleValues)
			if err != nil {
				respondErr(c, http.StatusBadRequest, "invalid example_values")
				return
			}
			exampleJSON = datatypes.JSON(b)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		// Ensure no duplicate (project_id, key).
		var existing model.PropertyDefinition
		if err := db.WithContext(ctx).
			Where("project_id = ? AND key = ?", projectID, key).
			First(&existing).Error; err == nil {
			respondErr(c, http.StatusConflict, "property already defined")
			return
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		row := model.PropertyDefinition{
			ProjectID:    projectID,
			Key:          key,
			DisplayName:  displayName,
			Type:         pt,
			Description:  req.Description,
			Status:       status,
			EnumValues:   enumJSON,
			ExampleValues: exampleJSON,
		}
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, row)
	}
}

// UpdatePropertyDefinitionHandler updates an existing property definition identified by key.
// PUT /api/:projectId/properties/schema/:propertyKey
func UpdatePropertyDefinitionHandler(db *gorm.DB) gin.HandlerFunc {
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
		key := strings.TrimSpace(c.Param("propertyKey"))
		if key == "" {
			respondErr(c, http.StatusBadRequest, "propertyKey required")
			return
		}

		var req struct {
			DisplayName  string   `json:"display_name"`
			Type         string   `json:"type"`
			Description  string   `json:"description"`
			Status       string   `json:"status"`
			EnumValues   []string `json:"enum_values"`
			ExampleValues []string `json:"example_values"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var row model.PropertyDefinition
		if err := db.WithContext(ctx).
			Where("project_id = ? AND key = ?", projectID, key).
			First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "property definition not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		displayName := strings.TrimSpace(req.DisplayName)
		if displayName == "" {
			displayName = row.DisplayName
		}
		if len(displayName) > 255 {
			displayName = displayName[:255]
		}
		pt := strings.ToLower(strings.TrimSpace(req.Type))
		if pt == "" {
			pt = row.Type
		}
		if !allowedPropertyTypes[pt] {
			respondErr(c, http.StatusBadRequest, "invalid type (expected string|enum|number)")
			return
		}
		status := strings.TrimSpace(req.Status)
		if status == "" {
			status = row.Status
		}

		var enumJSON datatypes.JSON
		if pt == "enum" {
			if len(req.EnumValues) == 0 && len(row.EnumValues) == 0 {
				respondErr(c, http.StatusBadRequest, "enum_values required for type=enum")
				return
			}
			// Prefer request values if provided, otherwise keep existing.
			if len(req.EnumValues) > 0 {
				b, err := json.Marshal(req.EnumValues)
				if err != nil {
					respondErr(c, http.StatusBadRequest, "invalid enum_values")
					return
				}
				enumJSON = datatypes.JSON(b)
			} else {
				enumJSON = row.EnumValues
			}
		}

		var exampleJSON datatypes.JSON
		if len(req.ExampleValues) > 0 {
			b, err := json.Marshal(req.ExampleValues)
			if err != nil {
				respondErr(c, http.StatusBadRequest, "invalid example_values")
				return
			}
			exampleJSON = datatypes.JSON(b)
		} else {
			exampleJSON = row.ExampleValues
		}

		row.DisplayName = displayName
		row.Type = pt
		row.Description = req.Description
		row.Status = status
		if pt == "enum" {
			row.EnumValues = enumJSON
		} else {
			row.EnumValues = nil
		}
		row.ExampleValues = exampleJSON

		if err := db.WithContext(ctx).Save(&row).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, row)
	}
}

