package query

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type pluginPackageConfig struct {
	OverviewCardsEnabled bool `json:"overviewCardsEnabled"`
	AnalyticsTabEnabled  bool `json:"analyticsTabEnabled"`
	AnalysisHours        int  `json:"analysisHours"`
}

type pluginPackageConfigPatch struct {
	OverviewCardsEnabled *bool `json:"overviewCardsEnabled"`
	AnalyticsTabEnabled  *bool `json:"analyticsTabEnabled"`
	AnalysisHours        *int  `json:"analysisHours"`
}

type pluginPackageSettingRequest struct {
	Enabled *bool                    `json:"enabled"`
	Config  pluginPackageConfigPatch `json:"config"`
}

type pluginPackageSettingResponse struct {
	ProjectID int                 `json:"projectId"`
	PackageID string              `json:"packageId"`
	Enabled   bool                `json:"enabled"`
	Config    pluginPackageConfig `json:"config"`
	CreatedAt *time.Time          `json:"createdAt,omitempty"`
	UpdatedAt *time.Time          `json:"updatedAt,omitempty"`
}

func GetPluginPackageSettingHandler(db *gorm.DB, svc *detector.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, packageID, ok := parsePluginSettingTarget(c, svc)
		if !ok {
			return
		}
		resp, err := loadPluginPackageSetting(c.Request.Context(), db, pid, packageID)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, resp)
	}
}

func UpsertPluginPackageSettingHandler(db *gorm.DB, svc *detector.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, packageID, ok := parsePluginSettingTarget(c, svc)
		if !ok {
			return
		}
		var req pluginPackageSettingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		current, err := loadPluginPackageSetting(c.Request.Context(), db, pid, packageID)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		enabled := current.Enabled
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		cfg := mergePluginPackageConfig(current.Config, req.Config)
		cfgJSON, _ := json.Marshal(cfg)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var row model.PluginPackageSetting
		err = db.WithContext(ctx).
			Where("project_id = ? AND package_id = ?", pid, packageID).
			First(&row).Error
		now := time.Now().UTC()
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			row = model.PluginPackageSetting{
				ProjectID: pid,
				PackageID: packageID,
				Enabled:   enabled,
				Config:    datatypes.JSON(cfgJSON),
			}
			if err := db.WithContext(ctx).Create(&row).Error; err != nil {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
		case err != nil:
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		default:
			if err := db.WithContext(ctx).
				Model(&model.PluginPackageSetting{}).
				Where("id = ?", row.ID).
				Updates(map[string]any{
					"enabled":    enabled,
					"config":     datatypes.JSON(cfgJSON),
					"updated_at": now,
				}).Error; err != nil {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			if err := db.WithContext(ctx).Where("id = ?", row.ID).First(&row).Error; err != nil {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
		}

		respondOK(c, pluginPackageSettingResponse{
			ProjectID: row.ProjectID,
			PackageID: row.PackageID,
			Enabled:   row.Enabled,
			Config:    normalizePluginPackageConfig(row.Config),
			CreatedAt: &row.CreatedAt,
			UpdatedAt: &row.UpdatedAt,
		})
	}
}

func ProjectPluginViewsHandler(db *gorm.DB, svc *detector.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
			return
		}
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		items, err := svc.ListViews("")
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		settings, err := loadPluginSettingsMap(c.Request.Context(), db, pid)
		if err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		filtered := make([]detector.ViewDescriptor, 0, len(items))
		for _, item := range items {
			setting := settings[item.PackageID]
			if !pluginViewEnabled(item, setting) {
				continue
			}
			filtered = append(filtered, item)
		}
		respondOK(c, gin.H{"items": filtered})
	}
}

func parsePluginSettingTarget(c *gin.Context, svc *detector.Service) (int, string, bool) {
	if svc == nil {
		respondErr(c, http.StatusServiceUnavailable, "detector service not configured")
		return 0, "", false
	}
	pid, err := project.ParseID(c.Param("projectId"))
	if err != nil {
		respondErr(c, http.StatusBadRequest, err.Error())
		return 0, "", false
	}
	packageID := strings.TrimSpace(c.Param("packageId"))
	if packageID == "" {
		respondErr(c, http.StatusBadRequest, "packageId is required")
		return 0, "", false
	}
	packages, err := svc.ListPackages()
	if err != nil {
		respondErr(c, http.StatusServiceUnavailable, err.Error())
		return 0, "", false
	}
	for _, pkg := range packages {
		if pkg.ID == packageID {
			return pid, packageID, true
		}
	}
	respondErr(c, http.StatusNotFound, "plugin package not found")
	return 0, "", false
}

func loadPluginPackageSetting(ctx context.Context, db *gorm.DB, projectID int, packageID string) (pluginPackageSettingResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var row model.PluginPackageSetting
	err := db.WithContext(ctx).
		Where("project_id = ? AND package_id = ?", projectID, packageID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return pluginPackageSettingResponse{
			ProjectID: projectID,
			PackageID: packageID,
			Enabled:   true,
			Config:    defaultPluginPackageConfig(),
		}, nil
	}
	if err != nil {
		return pluginPackageSettingResponse{}, err
	}
	return pluginPackageSettingResponse{
		ProjectID: row.ProjectID,
		PackageID: row.PackageID,
		Enabled:   row.Enabled,
		Config:    normalizePluginPackageConfig(row.Config),
		CreatedAt: &row.CreatedAt,
		UpdatedAt: &row.UpdatedAt,
	}, nil
}

func loadPluginSettingsMap(ctx context.Context, db *gorm.DB, projectID int) (map[string]pluginPackageSettingResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var rows []model.PluginPackageSetting
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]pluginPackageSettingResponse{}
	for _, row := range rows {
		out[row.PackageID] = pluginPackageSettingResponse{
			ProjectID: row.ProjectID,
			PackageID: row.PackageID,
			Enabled:   row.Enabled,
			Config:    normalizePluginPackageConfig(row.Config),
			CreatedAt: &row.CreatedAt,
			UpdatedAt: &row.UpdatedAt,
		}
	}
	return out, nil
}

func pluginViewEnabled(view detector.ViewDescriptor, setting pluginPackageSettingResponse) bool {
	if setting.PackageID == "" {
		setting = pluginPackageSettingResponse{
			Enabled: true,
			Config:  defaultPluginPackageConfig(),
		}
	}
	if !setting.Enabled {
		return false
	}
	switch view.Surface {
	case detector.ViewSurfaceOverviewCard:
		return setting.Config.OverviewCardsEnabled
	case detector.ViewSurfaceAnalyticsTab:
		return setting.Config.AnalyticsTabEnabled
	default:
		return true
	}
}

func defaultPluginPackageConfig() pluginPackageConfig {
	return pluginPackageConfig{
		OverviewCardsEnabled: true,
		AnalyticsTabEnabled:  true,
		AnalysisHours:        24,
	}
}

func mergePluginPackageConfig(current pluginPackageConfig, patch pluginPackageConfigPatch) pluginPackageConfig {
	out := sanitizePluginPackageConfig(current)
	if patch.OverviewCardsEnabled != nil {
		out.OverviewCardsEnabled = *patch.OverviewCardsEnabled
	}
	if patch.AnalyticsTabEnabled != nil {
		out.AnalyticsTabEnabled = *patch.AnalyticsTabEnabled
	}
	if patch.AnalysisHours != nil {
		out.AnalysisHours = *patch.AnalysisHours
	}
	return sanitizePluginPackageConfig(out)
}

func normalizePluginPackageConfig(raw datatypes.JSON) pluginPackageConfig {
	out := defaultPluginPackageConfig()
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return sanitizePluginPackageConfig(out)
}

func sanitizePluginPackageConfig(cfg pluginPackageConfig) pluginPackageConfig {
	if cfg.AnalysisHours <= 0 {
		cfg.AnalysisHours = 24
	}
	if cfg.AnalysisHours > 720 {
		cfg.AnalysisHours = 720
	}
	return cfg
}

func pluginPackageAnalysisHours(ctx context.Context, db *gorm.DB, projectID int, packageID string, fallback int) int {
	if db == nil || strings.TrimSpace(packageID) == "" {
		return fallback
	}
	setting, err := loadPluginPackageSetting(ctx, db, projectID, packageID)
	if err != nil {
		return fallback
	}
	return setting.Config.AnalysisHours
}
