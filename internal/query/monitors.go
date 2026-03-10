package query

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type monitorUpsertRequest struct {
	Name         string          `json:"name"`
	DetectorType string          `json:"detectorType"`
	Config       json.RawMessage `json:"config"`
	IntervalSec  *int            `json:"intervalSec,omitempty"`
	TimeoutMS    *int            `json:"timeoutMs,omitempty"`
	Enabled      *bool           `json:"enabled,omitempty"`
}

func ListMonitorsHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var items []model.MonitorDefinition
		if err := db.WithContext(ctx).Where("project_id = ?", pid).Order("id ASC").Find(&items).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

func CreateMonitorHandler(db *gorm.DB, svc *detector.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		if svc == nil {
			respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		var req monitorUpsertRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		row, err := buildMonitorFromRequest(pid, req)
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := validateMonitorConfigWithService(svc, row.DetectorType, row.Config); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
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

func GetMonitorHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		mid, err := parseMonitorID(c.Param("monitorId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var row model.MonitorDefinition
		if err := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, mid).First(&row).Error; err != nil {
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

func UpdateMonitorHandler(db *gorm.DB, svc *detector.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		if svc == nil {
			respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		mid, err := parseMonitorID(c.Param("monitorId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		var req monitorUpsertRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var cur model.MonitorDefinition
		if err := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, mid).First(&cur).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		updates := map[string]any{}
		nextDetectorType := cur.DetectorType
		nextConfig := datatypes.JSON(cur.Config)
		if strings.TrimSpace(req.Name) != "" {
			updates["name"] = normalizeMonitorName(req.Name)
		}
		if strings.TrimSpace(req.DetectorType) != "" {
			nextDetectorType = normalizeDetectorType(req.DetectorType)
			updates["detector_type"] = nextDetectorType
		}
		if len(req.Config) > 0 {
			cfg, err := normalizeMonitorConfig(req.Config)
			if err != nil {
				respondErr(c, http.StatusBadRequest, err.Error())
				return
			}
			nextConfig = cfg
			updates["config"] = nextConfig
		}
		if req.IntervalSec != nil {
			updates["interval_sec"] = normalizeIntervalSec(*req.IntervalSec)
		}
		if req.TimeoutMS != nil {
			updates["timeout_ms"] = normalizeTimeoutMS(*req.TimeoutMS)
		}
		if req.Enabled != nil {
			updates["enabled"] = *req.Enabled
			if *req.Enabled && !cur.Enabled {
				updates["next_run_at"] = time.Now().UTC()
			}
		}
		if len(updates) == 0 {
			respondOK(c, cur)
			return
		}
		if err := validateMonitorConfigWithService(svc, nextDetectorType, nextConfig); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		if err := db.WithContext(ctx).Model(&model.MonitorDefinition{}).Where("project_id = ? AND id = ?", pid, mid).Updates(updates).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if err := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, mid).First(&cur).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, cur)
	}
}

func DeleteMonitorHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		mid, err := parseMonitorID(c.Param("monitorId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("project_id = ? AND monitor_id = ?", pid, mid).Delete(&model.MonitorRun{}).Error; err != nil {
				return err
			}
			res := tx.Where("project_id = ? AND id = ?", pid, mid).Delete(&model.MonitorDefinition{})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
			return nil
		})
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"deleted": true})
	}
}

func ListMonitorRunsHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		mid, err := parseMonitorID(c.Param("monitorId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		limit := 50
		if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v <= 0 {
				respondErr(c, http.StatusBadRequest, "invalid limit")
				return
			}
			if v > 200 {
				v = 200
			}
			limit = v
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var items []model.MonitorRun
		if err := db.WithContext(ctx).
			Where("project_id = ? AND monitor_id = ?", pid, mid).
			Order("id DESC").
			Limit(limit).
			Find(&items).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

func RunMonitorNowHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		mid, err := parseMonitorID(c.Param("monitorId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		now := time.Now().UTC()
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		res := db.WithContext(ctx).
			Model(&model.MonitorDefinition{}).
			Where("project_id = ? AND id = ?", pid, mid).
			Updates(map[string]any{
				"enabled":     true,
				"next_run_at": now,
				"lease_owner": "",
				"lease_until": nil,
				"updated_at":  now,
			})
		if res.Error != nil {
			respondErr(c, http.StatusServiceUnavailable, res.Error.Error())
			return
		}
		if res.RowsAffected == 0 {
			respondErr(c, http.StatusNotFound, "not found")
			return
		}
		respondOK(c, gin.H{"queued": true})
	}
}

func TestMonitorHandler(db *gorm.DB, svc *detector.Service) gin.HandlerFunc {
	type signalSample struct {
		Source     string            `json:"source"`
		SourceType string            `json:"sourceType"`
		Severity   string            `json:"severity"`
		Status     string            `json:"status"`
		Message    string            `json:"message"`
		Labels     map[string]string `json:"labels,omitempty"`
		OccurredAt time.Time         `json:"occurredAt"`
	}
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		if svc == nil {
			respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		mid, err := parseMonitorID(c.Param("monitorId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var row model.MonitorDefinition
		if err := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, mid).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		if err := validateMonitorConfigWithService(svc, row.DetectorType, row.Config); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		timeoutMS := normalizeTimeoutMS(row.TimeoutMS)
		execCtx, execCancel := context.WithTimeout(c.Request.Context(), time.Duration(timeoutMS)*time.Millisecond)
		defer execCancel()
		signals, elapsed, err := svc.TestExecute(execCtx, row.DetectorType, detector.ExecuteRequest{
			ProjectID: row.ProjectID,
			Config:    json.RawMessage(row.Config),
			Payload: map[string]any{
				"monitor_id":   row.ID,
				"monitor_name": row.Name,
			},
			Now: time.Now().UTC(),
		})
		if err != nil {
			log.Printf("monitor test failed: project=%d monitor=%d detector=%s err=%v", row.ProjectID, row.ID, row.DetectorType, err)
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		const maxSamples = 5
		samples := make([]signalSample, 0, len(signals))
		for i := 0; i < len(signals) && i < maxSamples; i++ {
			s := signals[i]
			samples = append(samples, signalSample{
				Source:     s.Source,
				SourceType: s.SourceType,
				Severity:   s.Severity,
				Status:     s.Status,
				Message:    s.Message,
				Labels:     s.Labels,
				OccurredAt: s.OccurredAt,
			})
		}
		respondOK(c, gin.H{
			"monitorId":    row.ID,
			"detectorType": row.DetectorType,
			"signalCount":  len(signals),
			"elapsedMs":    elapsed.Milliseconds(),
			"samples":      samples,
		})
		log.Printf("monitor test ok: project=%d monitor=%d detector=%s signals=%d elapsed_ms=%d", row.ProjectID, row.ID, row.DetectorType, len(signals), elapsed.Milliseconds())
	}
}

func buildMonitorFromRequest(projectID int, req monitorUpsertRequest) (model.MonitorDefinition, error) {
	name := normalizeMonitorName(req.Name)
	if name == "" {
		return model.MonitorDefinition{}, errors.New("name is required")
	}
	detectorType := normalizeDetectorType(req.DetectorType)
	if detectorType == "" {
		return model.MonitorDefinition{}, errors.New("detectorType is required")
	}
	cfg, err := normalizeMonitorConfig(req.Config)
	if err != nil {
		return model.MonitorDefinition{}, err
	}
	intervalSec := 60
	if req.IntervalSec != nil {
		intervalSec = normalizeIntervalSec(*req.IntervalSec)
	}
	timeoutMS := 5000
	if req.TimeoutMS != nil {
		timeoutMS = normalizeTimeoutMS(*req.TimeoutMS)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	now := time.Now().UTC()
	return model.MonitorDefinition{
		ProjectID:    projectID,
		Name:         name,
		DetectorType: detectorType,
		Config:       cfg,
		IntervalSec:  intervalSec,
		TimeoutMS:    timeoutMS,
		Enabled:      enabled,
		NextRunAt:    now,
		LeaseOwner:   "",
		LeaseUntil:   nil,
	}, nil
}

func normalizeMonitorName(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 200 {
		return v[:200]
	}
	return v
}

func normalizeDetectorType(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func normalizeMonitorConfig(raw json.RawMessage) (datatypes.JSON, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return datatypes.JSON([]byte(`{}`)), nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, errors.New("config must be valid json")
	}
	return datatypes.JSON(raw), nil
}

func normalizeIntervalSec(v int) int {
	if v <= 0 {
		return 60
	}
	if v > 86400 {
		return 86400
	}
	return v
}

func normalizeTimeoutMS(v int) int {
	if v <= 0 {
		return 5000
	}
	if v > 120000 {
		return 120000
	}
	return v
}

func parseMonitorID(raw string) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return 0, errors.New("invalid monitorId")
	}
	return v, nil
}

func validateMonitorConfigWithService(svc *detector.Service, detectorType string, cfg datatypes.JSON) error {
	if svc == nil {
		return errors.New("detector service not configured")
	}
	return svc.Validate(detectorType, json.RawMessage(cfg))
}
