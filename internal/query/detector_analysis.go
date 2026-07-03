package query

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type detectorAnalysisResponse struct {
	ProjectID    int                     `json:"projectId"`
	DetectorType string                  `json:"detectorType"`
	Start        time.Time               `json:"start"`
	End          time.Time               `json:"end"`
	Summary      detectorAnalysisSummary `json:"summary"`
	Monitors     []monitorAnalysisItem   `json:"monitors"`
	View         json.RawMessage         `json:"view"`
	UpdatedAt    time.Time               `json:"updatedAt"`
}

type detectorAnalysisSummary struct {
	Status            string   `json:"status"`
	MonitorCount      int      `json:"monitorCount"`
	OKCount           int      `json:"okCount"`
	FailingCount      int      `json:"failingCount"`
	UnknownCount      int      `json:"unknownCount"`
	SuccessRate       *float64 `json:"successRate,omitempty"`
	AvgElapsedMS      *float64 `json:"avgElapsedMs,omitempty"`
	WorstCertDays     *float64 `json:"worstCertDays,omitempty"`
	LastCheckedAt     *string  `json:"lastCheckedAt,omitempty"`
	LastError         string   `json:"lastError,omitempty"`
	PrimaryMetric     string   `json:"primaryMetric"`
	PrimaryMetricUnit string   `json:"primaryMetricUnit,omitempty"`
}

type monitorAnalysisItem struct {
	ID            int            `json:"id"`
	Name          string         `json:"name"`
	DetectorType  string         `json:"detectorType"`
	Enabled       bool           `json:"enabled"`
	Target        string         `json:"target"`
	Status        string         `json:"status"`
	SuccessRate   *float64       `json:"successRate,omitempty"`
	AvgElapsedMS  *float64       `json:"avgElapsedMs,omitempty"`
	LastElapsedMS *float64       `json:"lastElapsedMs,omitempty"`
	LastCheckedAt string         `json:"lastCheckedAt,omitempty"`
	LastError     string         `json:"lastError,omitempty"`
	LastData      map[string]any `json:"lastData,omitempty"`
}

func DetectorAnalysisHandler(db *gorm.DB, store *detector.ResultStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		if store == nil {
			respondErr(c, http.StatusNotImplemented, "detector result store not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		detectorType := strings.ToLower(strings.TrimSpace(c.Param("detectorType")))
		if detectorType == "" {
			respondErr(c, http.StatusBadRequest, "detectorType is required")
			return
		}
		hours := parseAnalysisHours(c.Query("hours"), pluginPackageAnalysisHours(c.Request.Context(), db, pid, packageIDForDetector(detectorType), 24))
		end := time.Now().UTC()
		start := end.Add(-time.Duration(hours) * time.Hour)
		monitorID := 0
		if raw := strings.TrimSpace(c.Query("monitor_id")); raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v <= 0 {
				respondErr(c, http.StatusBadRequest, "invalid monitor_id")
				return
			}
			monitorID = v
		}

		resp, err := buildDetectorAnalysis(c.Request.Context(), db, store, pid, detectorType, monitorID, start, end)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, resp)
	}
}

func PluginPackageAnalysisHandler(db *gorm.DB, store *detector.ResultStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		if store == nil {
			respondErr(c, http.StatusNotImplemented, "detector result store not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		packageID := strings.TrimSpace(c.Param("packageId"))
		if packageID != "builtin.connectivity" {
			respondErr(c, http.StatusNotFound, "package analysis not found")
			return
		}
		hours := parseAnalysisHours(c.Query("hours"), pluginPackageAnalysisHours(c.Request.Context(), db, pid, packageID, 24))
		end := time.Now().UTC()
		start := end.Add(-time.Duration(hours) * time.Hour)
		detectors := []string{"tcp_check", "dns_check", "ssl_check"}
		items := map[string]detectorAnalysisResponse{}
		for _, typ := range detectors {
			resp, err := buildDetectorAnalysis(c.Request.Context(), db, store, pid, typ, 0, start, end)
			if err != nil {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			items[typ] = resp
		}
		respondOK(c, gin.H{
			"projectId": pid,
			"packageId": packageID,
			"start":     start,
			"end":       end,
			"detectors": items,
			"view":      connectivityPackageView(),
			"updatedAt": end,
		})
	}
}

func buildDetectorAnalysis(ctx context.Context, db *gorm.DB, store *detector.ResultStore, projectID int, detectorType string, monitorID int, start time.Time, end time.Time) (detectorAnalysisResponse, error) {
	var monitors []model.MonitorDefinition
	tx := db.WithContext(ctx).
		Where("project_id = ? AND detector_type = ?", projectID, detectorType).
		Order("id ASC")
	if monitorID > 0 {
		tx = tx.Where("id = ?", monitorID)
	}
	if err := tx.Find(&monitors).Error; err != nil {
		return detectorAnalysisResponse{}, err
	}

	resp := detectorAnalysisResponse{
		ProjectID:    projectID,
		DetectorType: detectorType,
		Start:        start,
		End:          end,
		View:         detectorAnalysisView(detectorType),
		UpdatedAt:    end,
	}

	for _, m := range monitors {
		results, err := store.Query(ctx, detectorType, detector.ResultQuery{
			ProjectID: projectID,
			MonitorID: m.ID,
			StartTime: start,
			EndTime:   end,
			Limit:     500,
		})
		if err != nil {
			return detectorAnalysisResponse{}, err
		}
		resp.Monitors = append(resp.Monitors, summarizeMonitor(m, results))
	}
	resp.Summary = summarizeDetector(detectorType, resp.Monitors)
	return resp, nil
}

func summarizeMonitor(m model.MonitorDefinition, results []detector.TypedResult) monitorAnalysisItem {
	item := monitorAnalysisItem{
		ID:           m.ID,
		Name:         m.Name,
		DetectorType: m.DetectorType,
		Enabled:      m.Enabled,
		Target:       monitorTarget(m),
		Status:       "unknown",
	}
	if len(results) == 0 {
		return item
	}

	successTotal := 0
	successCount := 0
	elapsedTotal := 0.0
	elapsedCount := 0
	var lastData map[string]any
	for i, r := range results {
		data := map[string]any{}
		_ = json.Unmarshal(r.Data, &data)
		if i == 0 {
			lastData = data
			item.LastData = data
			item.LastCheckedAt = r.Timestamp.UTC().Format(time.RFC3339)
			item.LastError = stringValue(data["error"])
			if v, ok := floatValue(data["elapsed_ms"]); ok {
				item.LastElapsedMS = ptrFloat(v)
			}
		}
		if ok, found := boolValue(data["success"]); found {
			successTotal++
			if ok {
				successCount++
			}
		}
		if v, ok := floatValue(data["elapsed_ms"]); ok {
			elapsedTotal += v
			elapsedCount++
		}
	}
	if successTotal > 0 {
		item.SuccessRate = ptrFloat(round1(float64(successCount) * 100 / float64(successTotal)))
	}
	if elapsedCount > 0 {
		item.AvgElapsedMS = ptrFloat(round1(elapsedTotal / float64(elapsedCount)))
	}

	if ok, found := boolValue(lastData["success"]); found {
		if ok {
			item.Status = "ok"
		} else {
			item.Status = "failing"
		}
	}
	if !m.Enabled && item.Status == "unknown" {
		item.Status = "disabled"
	}
	return item
}

func summarizeDetector(detectorType string, items []monitorAnalysisItem) detectorAnalysisSummary {
	s := detectorAnalysisSummary{
		Status:            "unknown",
		MonitorCount:      len(items),
		PrimaryMetric:     "成功率",
		PrimaryMetricUnit: "%",
	}
	successTotal := 0.0
	successCount := 0
	elapsedTotal := 0.0
	elapsedCount := 0
	worstCert := math.Inf(1)
	for _, item := range items {
		switch item.Status {
		case "ok":
			s.OKCount++
		case "failing":
			s.FailingCount++
		default:
			s.UnknownCount++
		}
		if item.SuccessRate != nil {
			successTotal += *item.SuccessRate
			successCount++
		}
		if item.AvgElapsedMS != nil {
			elapsedTotal += *item.AvgElapsedMS
			elapsedCount++
		}
		if item.LastCheckedAt != "" && s.LastCheckedAt == nil {
			ts := item.LastCheckedAt
			s.LastCheckedAt = &ts
		}
		if s.LastError == "" && item.LastError != "" {
			s.LastError = item.LastError
		}
		if detectorType == "ssl_check" {
			if v, ok := floatValue(item.LastData["cert_days_left"]); ok && v < worstCert {
				worstCert = v
			}
		}
	}
	if successCount > 0 {
		s.SuccessRate = ptrFloat(round1(successTotal / float64(successCount)))
	}
	if elapsedCount > 0 {
		s.AvgElapsedMS = ptrFloat(round1(elapsedTotal / float64(elapsedCount)))
	}
	if detectorType == "ssl_check" && !math.IsInf(worstCert, 1) {
		s.WorstCertDays = ptrFloat(round1(worstCert))
		s.PrimaryMetric = "最短剩余"
		s.PrimaryMetricUnit = "天"
	}
	switch {
	case s.MonitorCount == 0:
		s.Status = "unknown"
	case s.FailingCount > 0:
		s.Status = "failing"
	case s.OKCount > 0 && s.UnknownCount == 0:
		s.Status = "ok"
	default:
		s.Status = "unknown"
	}
	return s
}

func parseAnalysisHours(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		if fallback <= 0 {
			return 24
		}
		return fallback
	}
	if v > 720 {
		return 720
	}
	return v
}

func packageIDForDetector(detectorType string) string {
	switch strings.ToLower(strings.TrimSpace(detectorType)) {
	case "tcp_check", "dns_check", "ssl_check":
		return "builtin.connectivity"
	default:
		return ""
	}
}

func monitorTarget(m model.MonitorDefinition) string {
	cfg := map[string]any{}
	_ = json.Unmarshal(m.Config, &cfg)
	switch m.DetectorType {
	case "tcp_check", "ssl_check":
		host := stringValue(cfg["host"])
		port := intValue(cfg["port"])
		if port > 0 {
			return host + ":" + strconv.Itoa(port)
		}
		return host
	case "dns_check":
		rt := stringValue(cfg["recordType"])
		if rt == "" {
			rt = "A"
		}
		return rt + " " + stringValue(cfg["domain"])
	default:
		return m.Name
	}
}

func boolValue(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		if strings.EqualFold(x, "true") {
			return true, true
		}
		if strings.EqualFold(x, "false") {
			return false, true
		}
	}
	return false, false
}

func floatValue(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	return 0, false
}

func intValue(v any) int {
	if f, ok := floatValue(v); ok {
		return int(f)
	}
	return 0
}

func stringValue(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	default:
		return ""
	}
}

func ptrFloat(v float64) *float64 { return &v }

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func detectorAnalysisView(detectorType string) json.RawMessage {
	primaryPath := "summary.successRate"
	primaryUnit := "%"
	if detectorType == "ssl_check" {
		primaryPath = "summary.worstCertDays"
		primaryUnit = "天"
	}
	return json.RawMessage(`{
		"type": "stack",
		"gap": "md",
		"children": [
			{"type": "grid", "columns": 4, "children": [
				{"type": "metric", "label": "状态", "valuePath": "summary.status"},
				{"type": "metric", "label": "监控数", "valuePath": "summary.monitorCount"},
				{"type": "metric", "label": "成功率", "valuePath": "summary.successRate", "unit": "%"},
				{"type": "metric", "label": "平均延迟", "valuePath": "summary.avgElapsedMs", "unit": "ms"}
			]},
			{"type": "table", "valuePath": "monitors", "columns": [
				{"label": "名称", "valuePath": "name"},
				{"label": "目标", "valuePath": "target"},
				{"label": "状态", "valuePath": "status"},
				{"label": "成功率", "valuePath": "successRate", "unit": "%"},
				{"label": "延迟", "valuePath": "lastElapsedMs", "unit": "ms"},
				{"label": "最近检查", "valuePath": "lastCheckedAt"},
				{"label": "错误", "valuePath": "lastError"}
			]}
		],
		"card": {"primaryPath": "` + primaryPath + `", "primaryUnit": "` + primaryUnit + `"}
	}`)
}

func connectivityPackageView() json.RawMessage {
	return json.RawMessage(`{
		"type": "stack",
		"gap": "md",
		"children": [
			{"type": "grid", "columns": 3, "children": [
				{"type": "metric", "label": "TCP 成功率", "valuePath": "detectors.tcp_check.summary.successRate", "unit": "%"},
				{"type": "metric", "label": "DNS 成功率", "valuePath": "detectors.dns_check.summary.successRate", "unit": "%"},
				{"type": "metric", "label": "SSL 最短剩余", "valuePath": "detectors.ssl_check.summary.worstCertDays", "unit": "天"}
			]},
			{"type": "table", "valuePath": "detectors.tcp_check.monitors", "title": "TCP 检查", "columns": [
				{"label": "名称", "valuePath": "name"},
				{"label": "目标", "valuePath": "target"},
				{"label": "状态", "valuePath": "status"},
				{"label": "成功率", "valuePath": "successRate", "unit": "%"},
				{"label": "延迟", "valuePath": "lastElapsedMs", "unit": "ms"}
			]},
			{"type": "table", "valuePath": "detectors.dns_check.monitors", "title": "DNS 检查", "columns": [
				{"label": "名称", "valuePath": "name"},
				{"label": "目标", "valuePath": "target"},
				{"label": "状态", "valuePath": "status"},
				{"label": "成功率", "valuePath": "successRate", "unit": "%"},
				{"label": "结果", "valuePath": "lastData.results"}
			]},
			{"type": "table", "valuePath": "detectors.ssl_check.monitors", "title": "SSL 检查", "columns": [
				{"label": "名称", "valuePath": "name"},
				{"label": "目标", "valuePath": "target"},
				{"label": "状态", "valuePath": "status"},
				{"label": "剩余天数", "valuePath": "lastData.cert_days_left", "unit": "天"},
				{"label": "过期时间", "valuePath": "lastData.not_after"}
			]}
		]
	}`)
}
