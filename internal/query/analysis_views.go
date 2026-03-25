package query

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ListAnalysisViewsHandler lists saved analysis views for a project.
// GET /api/:projectId/analytics/views
func ListAnalysisViewsHandler(db *gorm.DB) gin.HandlerFunc {
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

		analysisType := strings.ToLower(strings.TrimSpace(c.Query("analysis_type")))
		if analysisType != "" && analysisType != "event" && analysisType != "property" {
			respondErr(c, http.StatusBadRequest, "invalid analysis_type (expected event|property)")
			return
		}
		search := strings.TrimSpace(c.Query("q"))

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		q := db.WithContext(ctx).Where("project_id = ?", projectID)
		if analysisType != "" {
			q = q.Where("analysis_type = ?", analysisType)
		}
		if search != "" {
			like := "%" + strings.ToLower(search) + "%"
			q = q.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", like, like)
		}

		var items []model.AnalysisView
		if err := q.Order("id DESC").Find(&items).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

// CreateAnalysisViewHandler creates a new analysis view for a project.
// POST /api/:projectId/analytics/views
func CreateAnalysisViewHandler(db *gorm.DB) gin.HandlerFunc {
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
		uid := userIDFromGin(c)
		if uid <= 0 {
			respondErr(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req struct {
			Name         string          `json:"name"`
			Description  string          `json:"description"`
			AnalysisType string          `json:"analysis_type"`
			Query        json.RawMessage `json:"query"`
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
		analysisType := strings.ToLower(strings.TrimSpace(req.AnalysisType))
		if analysisType != "event" && analysisType != "property" {
			respondErr(c, http.StatusBadRequest, "invalid analysis_type (expected event|property)")
			return
		}
		if len(req.Query) == 0 {
			respondErr(c, http.StatusBadRequest, "query required")
			return
		}

		// Ensure query is valid JSON to avoid storing invalid payloads.
		var tmp any
		if err := json.Unmarshal(req.Query, &tmp); err != nil {
			respondErr(c, http.StatusBadRequest, "invalid query JSON")
			return
		}

		row := model.AnalysisView{
			ProjectID:    projectID,
			Name:         name,
			Description:  strings.TrimSpace(req.Description),
			AnalysisType: analysisType,
			Query:        datatypes.JSON(req.Query),
			OwnerUserID:  uid,
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, row)
	}
}

// GetAnalysisViewHandler returns a single analysis view by id.
// GET /api/:projectId/analytics/views/:viewId
func GetAnalysisViewHandler(db *gorm.DB) gin.HandlerFunc {
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
		idStr := strings.TrimSpace(c.Param("viewId"))
		id64, err := strconv.ParseInt(idStr, 10, 32)
		if err != nil || id64 <= 0 {
			respondErr(c, http.StatusBadRequest, "invalid viewId")
			return
		}
		id := int(id64)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var row model.AnalysisView
		if err := db.WithContext(ctx).
			Where("project_id = ? AND id = ?", projectID, id).
			First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, row)
	}
}

// DeleteAnalysisViewHandler deletes an analysis view by id.
// DELETE /api/:projectId/analytics/views/:viewId
func DeleteAnalysisViewHandler(db *gorm.DB) gin.HandlerFunc {
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
		idStr := strings.TrimSpace(c.Param("viewId"))
		id64, err := strconv.ParseInt(idStr, 10, 32)
		if err != nil || id64 <= 0 {
			respondErr(c, http.StatusBadRequest, "invalid viewId")
			return
		}
		id := int(id64)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		res := db.WithContext(ctx).
			Where("project_id = ? AND id = ?", projectID, id).
			Delete(&model.AnalysisView{})
		if res.Error != nil {
			respondErr(c, http.StatusServiceUnavailable, res.Error.Error())
			return
		}
		if res.RowsAffected == 0 {
			respondErr(c, http.StatusNotFound, "not found")
			return
		}
		respondOK(c, gin.H{"deleted": true})
	}
}
