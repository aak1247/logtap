package httpserver

import (
	"expvar"
	"log"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/ingest"
	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/obs"
	"github.com/aak1247/logtap/internal/openapi"
	"github.com/aak1247/logtap/internal/query"
	"github.com/aak1247/logtap/internal/queue"
	"github.com/aak1247/logtap/internal/search"
	searchpostgres "github.com/aak1247/logtap/internal/search/adapters/postgres"
	"github.com/gin-gonic/gin"
	swgui "github.com/swaggest/swgui/v3"
	"gorm.io/gorm"
)

func New(cfg config.Config, publisher queue.Publisher, db *gorm.DB, recorder *metrics.RedisRecorder, stats *obs.Stats, detectorService *detector.Service, detectorStore *detector.ResultStore) *http.Server {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(maintenanceMiddleware(cfg.MaintenanceMode))
	router.Use(errorLogMiddleware())
	router.Use(bodyLimitMiddleware())
	if stats != nil {
		router.Use(observabilityMiddleware(stats))
	}

	if cfg.EnableDebugEndpoints {
		router.GET("/debug/vars", gin.WrapH(expvar.Handler()))
		router.GET("/debug/pprof/", gin.WrapF(pprof.Index))
		router.GET("/debug/pprof/cmdline", gin.WrapF(pprof.Cmdline))
		router.GET("/debug/pprof/profile", gin.WrapF(pprof.Profile))
		router.POST("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
		router.GET("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
		router.GET("/debug/pprof/trace", gin.WrapF(pprof.Trace))
		router.GET("/debug/metrics", query.DebugMetricsHandler(stats))
	}

	router.GET("/openapi.json", func(c *gin.Context) { c.JSON(http.StatusOK, openapi.Spec()) })
	router.GET("/docs/*any", gin.WrapH(swgui.New("logtap API", "/openapi.json", "/docs")))

	router.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })

	authEnabled := db != nil && len(cfg.AuthSecret) > 0
	if db != nil && !authEnabled {
		log.Println("WARNING: AUTH_SECRET is unset; the query API is unauthenticated and any caller can read every project. Set AUTH_SECRET before exposing this service beyond localhost.")
	}
	trustedProxyEnabled := strings.TrimSpace(cfg.LogtapProxySecret) != ""

	apiRoot := router.Group("/api")
	{
		apiRoot.GET("/status", query.StatusHandler(db, cfg.MaintenanceMode, len(cfg.AuthSecret) > 0))

		apiRoot.POST("/auth/bootstrap", query.BootstrapHandler(db, cfg.AuthSecret, cfg.AuthTokenTTL))
		apiRoot.POST("/auth/login", query.LoginHandler(db, cfg.AuthSecret, cfg.AuthTokenTTL))

		// Internal-only APIs for logtap-cloud integration (guarded by LOGTAP_PROXY_SECRET).
		if trustedProxyEnabled {
			internal := apiRoot.Group("/internal")
			internal.Use(requireProxySecretMiddleware(cfg.LogtapProxySecret))
			internal.POST("/projects", query.InternalCreateProjectHandler(db))
			internal.POST("/import/projects/:projectId/apply", query.InternalImportProjectApplyHandler(db, recorder))
			internal.POST("/metrics/projects/:projectId/rebuild", query.InternalRebuildProjectMetricsHandler(db, recorder))
			internal.GET("/metrics", query.DebugMetricsHandler(stats))
		}

		migrationAPI := apiRoot.Group("/migration")
		if authEnabled {
			migrationAPI.Use(RequireUser(cfg.AuthSecret))
		}
		migrationAPI.POST("/export/preview", query.MigrationPreviewHandler(db, query.MigrationConfig{DefaultCloudURL: cfg.MigrationCloudURL}))
		migrationAPI.POST("/export/cloud", query.MigrationExportCloudHandler(db, query.MigrationConfig{DefaultCloudURL: cfg.MigrationCloudURL}))

		authed := apiRoot.Group("")
		authed.Use(requireAuthReadyMiddleware(db, cfg.AuthSecret))
		if authEnabled {
			authed.Use(RequireUser(cfg.AuthSecret))
		}
		authed.GET("/me", query.MeHandler(db))
		if !trustedProxyEnabled {
			authed.GET("/internal/metrics", query.DebugMetricsHandler(stats))
		}
		authed.GET("/projects", query.ListProjectsHandler(db))
		authed.POST("/projects", query.CreateProjectHandler(db))
		authed.GET("/projects/:projectId", query.GetProjectHandler(db))
		authed.DELETE("/projects/:projectId", query.DeleteProjectHandler(db))
		authed.GET("/projects/:projectId/keys", query.ListProjectKeysHandler(db))
		authed.POST("/projects/:projectId/keys", query.CreateProjectKeyHandler(db))
		authed.POST("/projects/:projectId/keys/:keyId/revoke", query.RevokeProjectKeyHandler(db))

		pluginAPI := apiRoot.Group("/plugins")
		pluginAPI.Use(requireAuthReadyMiddleware(db, cfg.AuthSecret))
		if authEnabled {
			if trustedProxyEnabled {
				pluginAPI.Use(acceptProxySecretMiddleware(cfg.LogtapProxySecret), RequireUserOrProxy(cfg.AuthSecret))
			} else {
				pluginAPI.Use(RequireUser(cfg.AuthSecret))
			}
		}
		pluginAPI.GET("/packages", query.ListDetectorPackagesHandler(detectorService))
		pluginAPI.GET("/views", query.ListPluginViewsHandler(detectorService))
		pluginAPI.GET("/detectors", query.ListDetectorsHandler(detectorService))
		pluginAPI.GET("/detectors/:detectorType/schema", query.GetDetectorSchemaHandler(detectorService))
		pluginAPI.GET("/detectors/:detectorType/views", query.ListDetectorViewsHandler(detectorService))
		pluginAPI.GET("/detectors/:detectorType/health", query.DetectorHealthHandler(detectorService))
		pluginAPI.GET("/detectors/:detectorType/aggregate", query.DetectorAggregateHandler(detectorService, detectorStore))
	}

	ingestAPI := router.Group("/api/:projectId")
	if trustedProxyEnabled && !authEnabled {
		ingestAPI.Use(requireProxySecretMiddleware(cfg.LogtapProxySecret))
	} else {
		ingestAPI.Use(acceptProxySecretMiddleware(cfg.LogtapProxySecret))
	}
	{
		switch {
		case authEnabled:
			ingestAPI.POST("/store/", RequireProjectKey(db), ingest.SentryStoreHandler(publisher))
			ingestAPI.POST("/envelope/", RequireProjectKey(db), ingest.SentryEnvelopeHandler(publisher))
			ingestAPI.POST("/logs/", RequireProjectKey(db), ingest.CustomLogHandler(publisher))
			ingestAPI.POST("/track/", RequireProjectKey(db), ingest.TrackEventHandler(publisher))
		default:
			ingestAPI.POST("/store/", ingest.SentryStoreHandler(publisher))
			ingestAPI.POST("/envelope/", ingest.SentryEnvelopeHandler(publisher))
			ingestAPI.POST("/logs/", ingest.CustomLogHandler(publisher))
			ingestAPI.POST("/track/", ingest.TrackEventHandler(publisher))
		}
	}

	queryAPI := router.Group("/api/:projectId")
	if trustedProxyEnabled && !authEnabled {
		queryAPI.Use(requireProxySecretMiddleware(cfg.LogtapProxySecret))
	} else {
		queryAPI.Use(acceptProxySecretMiddleware(cfg.LogtapProxySecret))
	}
	if authEnabled {
		queryAPI.Use(RequireUserOrProxy(cfg.AuthSecret), RequireProjectOwner(db))
	}
	{
		if db != nil {
			queryAPI.GET("/events/recent", query.RecentEventsHandler(db))
			queryAPI.GET("/events/:eventId", query.GetEventHandler(db))
			queryAPI.GET("/events/schema", query.ListEventDefinitionsHandler(db))
			queryAPI.POST("/events/schema", query.CreateEventDefinitionHandler(db))
			queryAPI.PUT("/events/schema/:eventName", query.UpdateEventDefinitionHandler(db))
			queryAPI.GET("/logs/search", query.SearchLogsHandler(db))
			queryAPI.GET("/logs/trend", query.LogTrendHandler(db))
			// Unified search endpoint (v1: queries logs table via adapter)
			if db != nil {
				searchEngine := search.NewEngine(searchpostgres.NewAdapter(db))
				queryAPI.GET("/search", search.SearchHandler(searchEngine))
			}
			queryAPI.DELETE("/logs/cleanup", query.CleanupLogsHandler(db))
			queryAPI.DELETE("/events/cleanup", query.CleanupEventsHandler(db))
			queryAPI.GET("/storage/estimate", query.StorageEstimateHandler(db))
			queryAPI.GET("/cleanup/policy", query.GetCleanupPolicyHandler(db))
			queryAPI.PUT("/cleanup/policy", query.UpsertCleanupPolicyHandler(db))
			queryAPI.POST("/cleanup/run", query.RunCleanupPolicyHandler(db))
			queryAPI.GET("/analytics/events/top", query.TopEventsHandler(db))
			queryAPI.GET("/analytics/users", query.UserGrowthHandler(db))
			queryAPI.GET("/analytics/funnel", query.FunnelHandler(db))
			queryAPI.POST("/analytics/custom", query.CustomAnalyticsHandler(db))
			queryAPI.GET("/analytics/views", query.ListAnalysisViewsHandler(db))
			queryAPI.POST("/analytics/views", query.CreateAnalysisViewHandler(db))
			queryAPI.GET("/analytics/views/:viewId", query.GetAnalysisViewHandler(db))
			queryAPI.DELETE("/analytics/views/:viewId", query.DeleteAnalysisViewHandler(db))

			alerts := queryAPI.Group("/alerts")
			{
				alerts.GET("/contacts", query.ListAlertContactsHandler(db))
				alerts.POST("/contacts", query.CreateAlertContactHandler(db))
				alerts.PUT("/contacts/:contactId", query.UpdateAlertContactHandler(db))
				alerts.DELETE("/contacts/:contactId", query.DeleteAlertContactHandler(db))

				alerts.GET("/contact-groups", query.ListAlertContactGroupsHandler(db))
				alerts.POST("/contact-groups", query.CreateAlertContactGroupHandler(db))
				alerts.PUT("/contact-groups/:groupId", query.UpdateAlertContactGroupHandler(db))
				alerts.DELETE("/contact-groups/:groupId", query.DeleteAlertContactGroupHandler(db))

				alerts.GET("/wecom-bots", query.ListAlertWecomBotsHandler(db))
				alerts.POST("/wecom-bots", query.CreateAlertWecomBotHandler(db))
				alerts.PUT("/wecom-bots/:botId", query.UpdateAlertWecomBotHandler(db))
				alerts.DELETE("/wecom-bots/:botId", query.DeleteAlertWecomBotHandler(db))

				alerts.GET("/webhook-endpoints", query.ListAlertWebhookEndpointsHandler(db))
				alerts.POST("/webhook-endpoints", query.CreateAlertWebhookEndpointHandler(db))
				alerts.PUT("/webhook-endpoints/:endpointId", query.UpdateAlertWebhookEndpointHandler(db))
				alerts.DELETE("/webhook-endpoints/:endpointId", query.DeleteAlertWebhookEndpointHandler(db))

				alerts.GET("/rules", query.ListAlertRulesHandler(db))
				alerts.POST("/rules", query.CreateAlertRuleHandler(db))
				alerts.POST("/rules/test", query.TestAlertRulesHandler(db))
				alerts.POST("/rules/:ruleId/test-deliveries", query.TestAlertRulesDeliveriesHandler(db))
				alerts.PUT("/rules/:ruleId", query.UpdateAlertRuleHandler(db))
				alerts.DELETE("/rules/:ruleId", query.DeleteAlertRuleHandler(db))

				alerts.GET("/deliveries", query.ListAlertDeliveriesHandler(db))
			}

			monitors := queryAPI.Group("/monitors")
			{
				monitors.GET("", query.ListMonitorsHandler(db))
				monitors.POST("", query.CreateMonitorHandler(db, detectorService))
				monitors.GET("/:monitorId", query.GetMonitorHandler(db))
				monitors.PUT("/:monitorId", query.UpdateMonitorHandler(db, detectorService))
				monitors.DELETE("/:monitorId", query.DeleteMonitorHandler(db))
				monitors.GET("/:monitorId/runs", query.ListMonitorRunsHandler(db))
				monitors.POST("/:monitorId/run", query.RunMonitorNowHandler(db))
				monitors.POST("/:monitorId/test", query.TestMonitorHandler(db, detectorService))
			}
			queryAPI.GET("/plugins/views", query.ProjectPluginViewsHandler(db, detectorService))
			queryAPI.GET("/plugins/packages/:packageId/settings", query.GetPluginPackageSettingHandler(db, detectorService))
			queryAPI.PUT("/plugins/packages/:packageId/settings", query.UpsertPluginPackageSettingHandler(db, detectorService))
			queryAPI.GET("/plugins/packages/:packageId/analysis", query.PluginPackageAnalysisHandler(db, detectorStore))
			queryAPI.GET("/plugins/detectors/:detectorType/analysis", query.DetectorAnalysisHandler(db, detectorStore))
		}
		queryAPI.GET("/metrics/today", query.MetricsTodayHandler(recorder, db))
		queryAPI.GET("/metrics/total", query.MetricsTotalHandler(recorder, db))
		queryAPI.GET("/analytics/active", query.ActiveSeriesHandler(recorder, db))
		queryAPI.GET("/analytics/dist", query.DistributionHandler(recorder))
		queryAPI.GET("/analytics/dist/series", query.DistributionSeriesHandler(recorder))
		queryAPI.GET("/analytics/retention", query.RetentionHandler(recorder, db))
		queryAPI.GET("/properties/schema", query.ListPropertyDefinitionsHandler(db))
		queryAPI.POST("/properties/schema", query.CreatePropertyDefinitionHandler(db))
		queryAPI.PUT("/properties/schema/:propertyKey", query.UpdatePropertyDefinitionHandler(db))
	}

	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      300 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

func requireAuthReadyMiddleware(db *gorm.DB, authSecret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			c.JSON(http.StatusNotImplemented, gin.H{"code": http.StatusNotImplemented, "err": "database not configured"})
			c.Abort()
			return
		}
		if len(authSecret) == 0 {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable, "err": "AUTH_SECRET not configured"})
			c.Abort()
			return
		}
		c.Next()
	}
}
