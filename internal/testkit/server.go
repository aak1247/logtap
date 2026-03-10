package testkit

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/detector/plugins/logbasic"
	"github.com/aak1247/logtap/internal/httpserver"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const TestAuthSecret = "01234567890123456789012345678901"

type Server struct {
	DB        *gorm.DB
	Publisher *InlinePublisher
	Config    config.Config
	HTTP      *httptest.Server
}

func NewServer(t testing.TB) *Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	db := OpenTestDB(t)
	cfg := config.Config{
		HTTPAddr:     "127.0.0.1:0",
		AuthSecret:   []byte(TestAuthSecret),
		AuthTokenTTL: time.Hour,
		// Allow httptest loopback webhooks in integration tests.
		WebhookAllowLoopback: true,
	}
	publisher := &InlinePublisher{DB: db}
	reg := detector.NewRegistry()
	_ = reg.RegisterStatic(logbasic.New())
	detectorService := detector.NewService(reg)

	srv := httpserver.New(cfg, publisher, db, nil, nil, detectorService)
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)

	return &Server{
		DB:        db,
		Publisher: publisher,
		Config:    cfg,
		HTTP:      ts,
	}
}
