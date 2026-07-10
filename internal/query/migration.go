package query

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/migration"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type MigrationConfig struct {
	DefaultCloudURL string
}

type migrationExportRequest struct {
	CloudAPIBase               string `json:"cloud_api_base"`
	CloudToken                 string `json:"cloud_token"`
	OrgID                      *int64 `json:"org_id,omitempty"`
	ProjectIDs                 []int  `json:"project_ids"`
	Since                      string `json:"since,omitempty"`
	Until                      string `json:"until,omitempty"`
	IncludeNotificationSecrets *bool  `json:"include_notification_secrets,omitempty"`
}

func MigrationPreviewHandler(db *gorm.DB, cfg MigrationConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		uid := userIDFromGin(c)
		var req migrationExportRequest
		if c.Request.Body != nil && c.Request.ContentLength != 0 {
			if err := c.ShouldBindJSON(&req); err != nil {
				respondErr(c, http.StatusBadRequest, err.Error())
				return
			}
		}
		since, until, err := parseMigrationTimeRange(req.Since, req.Until)
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		preview, err := migration.BuildPreview(ctx, db, uid, req.ProjectIDs, since, until, cfg.DefaultCloudURL)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, preview)
	}
}

func MigrationExportCloudHandler(db *gorm.DB, cfg MigrationConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		uid := userIDFromGin(c)
		var req migrationExportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		cloudBase := strings.TrimSpace(req.CloudAPIBase)
		if cloudBase == "" {
			cloudBase = cfg.DefaultCloudURL
		}
		cloudBase = strings.TrimRight(cloudBase, "/")
		if err := validateHTTPURL(cloudBase); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		cloudToken := strings.TrimSpace(req.CloudToken)
		if cloudToken == "" {
			respondErr(c, http.StatusBadRequest, "cloud_token required")
			return
		}
		since, until, err := parseMigrationTimeRange(req.Since, req.Until)
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		includeSecrets := true
		if req.IncludeNotificationSecrets != nil {
			includeSecrets = *req.IncludeNotificationSecrets
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
		defer cancel()
		bundle, err := migration.ExportBundle(ctx, db, migration.ExportOptions{
			OwnerUserID:                uid,
			ProjectIDs:                 req.ProjectIDs,
			Since:                      since,
			Until:                      until,
			IncludeNotificationSecrets: includeSecrets,
		})
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		resp, err := pushBundleToCloud(ctx, cloudBase, cloudToken, req.OrgID, bundle)
		if err != nil {
			respondErr(c, http.StatusBadGateway, err.Error())
			return
		}
		respondOK(c, resp)
	}
}

type cloudImportResponse struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	StatusURL string `json:"status_url,omitempty"`
}

func pushBundleToCloud(ctx context.Context, cloudBase string, token string, orgID *int64, bundle migration.Bundle) (cloudImportResponse, error) {
	body := map[string]any{"bundle": bundle}
	if orgID != nil && *orgID > 0 {
		body["org_id"] = *orgID
	}
	b, err := json.Marshal(body)
	if err != nil {
		return cloudImportResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cloudBase+"/api/imports/local-logtap", bytes.NewReader(b))
	if err != nil {
		return cloudImportResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return cloudImportResponse{}, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var env struct {
		Code  int                 `json:"code"`
		Data  cloudImportResponse `json:"data"`
		Err   string              `json:"err"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &env)
	if res.StatusCode < 200 || res.StatusCode >= 300 || env.Code != 0 {
		msg := strings.TrimSpace(env.Err)
		if msg == "" && env.Error != nil {
			msg = strings.TrimSpace(env.Error.Message)
		}
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		if msg == "" {
			msg = res.Status
		}
		return cloudImportResponse{}, errors.New(msg)
	}
	if env.Data.JobID == "" {
		return cloudImportResponse{}, errors.New("cloud import response missing job_id")
	}
	return env.Data, nil
}

func InternalImportProjectApplyHandler(db *gorm.DB, recorder *metrics.RedisRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		projectID, err := parsePositiveInt(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, "invalid projectId")
			return
		}
		var req struct {
			Project migration.ProjectBundle `json:"project"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
		defer cancel()
		result, err := migration.ApplyProjectBundle(ctx, db, projectID, req.Project)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "target project not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		rebuild, err := recorder.RebuildProjectFromDB(ctx, db, metrics.RebuildProjectOptions{
			ProjectID:  projectID,
			BatchSize:  1000,
			ResetRedis: true,
		})
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, "rebuild metrics: "+err.Error())
			return
		}
		respondOK(c, gin.H{
			"apply":           result,
			"metrics_rebuild": rebuild,
		})
	}
}

func InternalRebuildProjectMetricsHandler(db *gorm.DB, recorder *metrics.RedisRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		projectID, err := parsePositiveInt(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, "invalid projectId")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
		defer cancel()
		result, err := recorder.RebuildProjectFromDB(ctx, db, metrics.RebuildProjectOptions{
			ProjectID:  projectID,
			BatchSize:  1000,
			ResetRedis: true,
		})
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, result)
	}
}

func parseMigrationTimeRange(sinceRaw string, untilRaw string) (*time.Time, *time.Time, error) {
	var since *time.Time
	var until *time.Time
	if strings.TrimSpace(sinceRaw) != "" {
		t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(sinceRaw))
		if err != nil {
			return nil, nil, errors.New("invalid since")
		}
		u := t.UTC()
		since = &u
	}
	if strings.TrimSpace(untilRaw) != "" {
		t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(untilRaw))
		if err != nil {
			return nil, nil, errors.New("invalid until")
		}
		u := t.UTC()
		until = &u
	}
	if since != nil && until != nil && since.After(*until) {
		return nil, nil, errors.New("since must be before until")
	}
	return since, until, nil
}

func validateHTTPURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New("invalid cloud_api_base")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("cloud_api_base must use http or https")
	}
	return nil
}

func parsePositiveInt(raw string) (int, error) {
	var n int
	for _, ch := range strings.TrimSpace(raw) {
		if ch < '0' || ch > '9' {
			return 0, errors.New("invalid int")
		}
		n = n*10 + int(ch-'0')
	}
	if n <= 0 {
		return 0, errors.New("invalid int")
	}
	return n, nil
}
