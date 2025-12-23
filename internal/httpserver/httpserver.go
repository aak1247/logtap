package httpserver

import (
	"net/http"
	"time"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/ingest"
	"github.com/aak1247/logtap/internal/metrics"
	"github.com/aak1247/logtap/internal/openapi"
	"github.com/aak1247/logtap/internal/query"
	"github.com/aak1247/logtap/internal/queue"
	"github.com/gin-gonic/gin"
	swgui "github.com/swaggest/swgui/v3"
	"gorm.io/gorm"
)

func New(cfg config.Config, publisher queue.Publisher, db *gorm.DB, recorder *metrics.RedisRecorder) *http.Server {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(maintenanceMiddleware(cfg.MaintenanceMode))

	router.GET("/openapi.json", func(c *gin.Context) { c.JSON(http.StatusOK, openapi.Spec()) })
	router.GET("/docs/*any", gin.WrapH(swgui.New("logtap API", "/openapi.json", "/docs")))

	router.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })

	authEnabled := db != nil && len(cfg.AuthSecret) > 0

	apiRoot := router.Group("/api")
	{
		apiRoot.GET("/status", query.StatusHandler(db, cfg.MaintenanceMode, len(cfg.AuthSecret) > 0))
		if db != nil {
			apiRoot.POST("/auth/bootstrap", query.BootstrapHandler(db, cfg.AuthSecret, cfg.AuthTokenTTL))
			apiRoot.POST("/auth/login", query.LoginHandler(db, cfg.AuthSecret, cfg.AuthTokenTTL))
			if authEnabled {
				authed := apiRoot.Group("")
				authed.Use(RequireUser(cfg.AuthSecret))
				authed.GET("/me", query.MeHandler(db))
				authed.GET("/projects", query.ListProjectsHandler(db))
				authed.POST("/projects", query.CreateProjectHandler(db))
				authed.GET("/projects/:projectId", query.GetProjectHandler(db))
				authed.GET("/projects/:projectId/keys", query.ListProjectKeysHandler(db))
				authed.POST("/projects/:projectId/keys", query.CreateProjectKeyHandler(db))
				authed.POST("/projects/:projectId/keys/:keyId/revoke", query.RevokeProjectKeyHandler(db))
			}
		}
	}

	ingestAPI := router.Group("/api/:projectId")
	{
		if authEnabled {
			ingestAPI.POST("/store/", RequireProjectKey(db), ingest.SentryStoreHandler(publisher))
			ingestAPI.POST("/envelope/", RequireProjectKey(db), ingest.SentryEnvelopeHandler(publisher))
			ingestAPI.POST("/logs/", RequireProjectKey(db), ingest.CustomLogHandler(publisher))
			ingestAPI.POST("/track/", RequireProjectKey(db), ingest.TrackEventHandler(publisher))
		} else {
			ingestAPI.POST("/store/", ingest.SentryStoreHandler(publisher))
			ingestAPI.POST("/envelope/", ingest.SentryEnvelopeHandler(publisher))
			ingestAPI.POST("/logs/", ingest.CustomLogHandler(publisher))
			ingestAPI.POST("/track/", ingest.TrackEventHandler(publisher))
		}
	}

	queryAPI := router.Group("/api/:projectId")
	if authEnabled {
		queryAPI.Use(RequireUser(cfg.AuthSecret), RequireProjectOwner(db))
	}
	{
		if db != nil {
			queryAPI.GET("/events/recent", query.RecentEventsHandler(db))
			queryAPI.GET("/events/:eventId", query.GetEventHandler(db))
			queryAPI.GET("/logs/search", query.SearchLogsHandler(db))
			queryAPI.DELETE("/logs/cleanup", query.CleanupLogsHandler(db))
			queryAPI.DELETE("/events/cleanup", query.CleanupEventsHandler(db))
			queryAPI.GET("/analytics/events/top", query.TopEventsHandler(db))
			queryAPI.GET("/analytics/funnel", query.FunnelHandler(db))
		}
		if recorder != nil {
			queryAPI.GET("/metrics/today", query.MetricsTodayHandler(recorder))
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
