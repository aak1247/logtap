package httpserver

import (
	"expvar"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/ingest"
	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/obs"
	"github.com/aak1247/logtap/internal/openapi"
	"github.com/aak1247/logtap/internal/query"
	"github.com/aak1247/logtap/internal/queue"
	"github.com/gin-gonic/gin"
	swgui "github.com/swaggest/swgui/v3"
	"gorm.io/gorm"
)

func New(cfg config.Config, publisher queue.Publisher, db *gorm.DB, recorder *metrics.RedisRecorder, stats *obs.Stats) *http.Server {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(maintenanceMiddleware(cfg.MaintenanceMode))
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
			internal.GET("/metrics", query.DebugMetricsHandler(stats))
		}

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
			queryAPI.GET("/logs/search", query.SearchLogsHandler(db))
			queryAPI.DELETE("/logs/cleanup", query.CleanupLogsHandler(db))
			queryAPI.DELETE("/events/cleanup", query.CleanupEventsHandler(db))
			queryAPI.GET("/storage/estimate", query.StorageEstimateHandler(db))
			queryAPI.GET("/cleanup/policy", query.GetCleanupPolicyHandler(db))
			queryAPI.PUT("/cleanup/policy", query.UpsertCleanupPolicyHandler(db))
			queryAPI.POST("/cleanup/run", query.RunCleanupPolicyHandler(db))
			queryAPI.GET("/analytics/events/top", query.TopEventsHandler(db))
			queryAPI.GET("/analytics/funnel", query.FunnelHandler(db))
		}
		if recorder != nil {
			queryAPI.GET("/metrics/today", query.MetricsTodayHandler(recorder))
			queryAPI.GET("/metrics/total", query.MetricsTotalHandler(recorder))
			queryAPI.GET("/analytics/active", query.ActiveSeriesHandler(recorder))
			queryAPI.GET("/analytics/dist", query.DistributionHandler(recorder))
			queryAPI.GET("/analytics/retention", query.RetentionHandler(recorder))
		}
	}

	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
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
