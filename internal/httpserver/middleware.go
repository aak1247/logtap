package httpserver

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Encoding, X-Sentry-Auth, X-Project-Key, X-Logtap-Proxy-Secret, X-Requested-With, sentry-trace, baggage, Authorization")

		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next()
	}
}

const ctxProxyOKKey = "proxy_ok"

func proxyOK(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(ctxProxyOKKey)
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func acceptProxySecretMiddleware(secret string) gin.HandlerFunc {
	secret = strings.TrimSpace(secret)
	return func(c *gin.Context) {
		if secret == "" {
			c.Next()
			return
		}
		got := strings.TrimSpace(c.GetHeader("X-Logtap-Proxy-Secret"))
		if got == "" {
			c.Next()
			return
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") || c.Request.URL.Path == "/api" {
				c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "err": "unauthorized"})
			} else {
				c.Status(http.StatusUnauthorized)
			}
			c.Abort()
			return
		}
		c.Set(ctxProxyOKKey, true)
		c.Next()
	}
}

func requireProxySecretMiddleware(secret string) gin.HandlerFunc {
	secret = strings.TrimSpace(secret)
	return func(c *gin.Context) {
		if secret == "" {
			c.Next()
			return
		}
		got := strings.TrimSpace(c.GetHeader("X-Logtap-Proxy-Secret"))
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") || c.Request.URL.Path == "/api" {
				c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "err": "unauthorized"})
			} else {
				c.Status(http.StatusUnauthorized)
			}
			c.Abort()
			return
		}
		c.Set(ctxProxyOKKey, true)
		c.Next()
	}
}

func maintenanceMiddleware(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}
		switch c.Request.URL.Path {
		case "/healthz", "/api/status":
			c.Next()
			return
		default:
			if strings.HasPrefix(c.Request.URL.Path, "/api/") || c.Request.URL.Path == "/api" {
				c.JSON(http.StatusServiceUnavailable, gin.H{"code": http.StatusServiceUnavailable, "err": "maintenance"})
			} else {
				c.String(http.StatusServiceUnavailable, "maintenance")
			}
			c.Abort()
			return
		}
	}
}
