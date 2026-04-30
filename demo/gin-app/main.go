package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aak1247/logtap/sdks/go/logtap"
	"github.com/gin-gonic/gin"
)

func getenv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func getenvBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func getenvInt64(key string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func main() {
	baseURL := getenv("LOGTAP_BASE_URL", "http://localhost:8080")
	projectID := getenvInt64("LOGTAP_PROJECT_ID", 1)
	projectKey := getenv("LOGTAP_PROJECT_KEY", "")
	gzip := getenvBool("LOGTAP_GZIP", true)

	httpAddr := getenv("HTTP_ADDR", ":8090")

	// Create one client and reuse it across handlers.
	client, err := logtap.NewClient(logtap.ClientOptions{
		BaseURL:    baseURL,
		ProjectID:  projectID,
		ProjectKey: projectKey,
		Gzip:       gzip,
		GlobalTags: map[string]string{"env": "demo", "runtime": "go"},
		GlobalContexts: map[string]any{
			"demo": map[string]any{"kind": "gin"},
		},
	})
	if err != nil {
		panic(err)
	}

	client.Info("gin demo start", map[string]any{
		"addr":       httpAddr,
		"project_id": projectID,
	}, nil)
	client.Track("demo_init", map[string]any{"kind": "gin"}, nil)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Optional: request logging middleware.
	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)

		uid := strings.TrimSpace(c.Query("user"))
		var u *logtap.User
		if uid != "" {
			u = &logtap.User{ID: uid}
		}

		client.Info("gin request", map[string]any{
			"method":     c.Request.Method,
			"path":       c.FullPath(),
			"raw_path":   c.Request.URL.Path,
			"status":     c.Writer.Status(),
			"latency_ms": latency.Milliseconds(),
			"client_ip":  c.ClientIP(),
		}, &logtap.LogOptions{
			User: u,
			Tags: map[string]string{"source": "middleware"},
		})
	})

	// Optional: report panics before returning 500.
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		uid := strings.TrimSpace(c.Query("user"))
		var u *logtap.User
		if uid != "" {
			u = &logtap.User{ID: uid}
		}

		client.Fatal("gin panic", map[string]any{
			"panic": fmt.Sprint(recovered),
			"path":  c.Request.URL.Path,
			"stack": string(debug.Stack()),
		}, &logtap.LogOptions{User: u})

		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "panic"})
	}))

	r.GET("/", func(c *gin.Context) {
		client.Info("hello from gin", map[string]any{
			"hint": "try /track?name=signup or /panic",
		}, nil)
		c.JSON(http.StatusOK, gin.H{
			"ok": true,
		})
	})

	r.GET("/track", func(c *gin.Context) {
		name := strings.TrimSpace(c.Query("name"))
		if name == "" {
			name = "signup"
		}
		uid := strings.TrimSpace(c.Query("user"))
		var u *logtap.User
		if uid != "" {
			u = &logtap.User{ID: uid}
		}

		client.Track(name, map[string]any{"from": "gin-demo"}, &logtap.TrackOptions{User: u})
		c.JSON(http.StatusOK, gin.H{"queued": true, "name": name})
	})

	r.GET("/panic", func(c *gin.Context) {
		panic("demo panic")
	})

	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			client.Error("gin server error", map[string]any{"err": err.Error()}, nil)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = srv.Shutdown(shutdownCtx)
	cancel()

	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = client.Close(closeCtx)
	closeCancel()
}
