package testkit

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/config"
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
	}
	publisher := &InlinePublisher{DB: db}

	srv := httpserver.New(cfg, publisher, db, nil, nil)
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)

	return &Server{
		DB:        db,
		Publisher: publisher,
		Config:    cfg,
		HTTP:      ts,
	}
}
