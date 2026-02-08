package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aak1247/logtap/internal/auth"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/project"
	"github.com/aak1247/logtap/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const ctxUserIDKey = "user_id"

func RequireUser(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.GetHeader("Authorization"))
		token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
		if token == "" {
			c.Status(http.StatusUnauthorized)
			c.Abort()
			return
		}

		claims, ok := auth.VerifyToken(secret, token, time.Now())
		if !ok {
			c.Status(http.StatusUnauthorized)
			c.Abort()
			return
		}
		c.Set(ctxUserIDKey, claims.UserID)
		c.Next()
	}
}

func RequireUserOrProxy(secret []byte) gin.HandlerFunc {
	requireUser := RequireUser(secret)
	return func(c *gin.Context) {
		if proxyOK(c) {
			c.Next()
			return
		}
		requireUser(c)
	}
}

func userIDFromContext(c *gin.Context) (int64, bool) {
	v, ok := c.Get(ctxUserIDKey)
	if !ok {
		return 0, false
	}
	id, ok := v.(int64)
	return id, ok && id > 0
}

func RequireProjectOwner(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if proxyOK(c) {
			c.Next()
			return
		}
		if db == nil {
			c.Status(http.StatusNotImplemented)
			c.Abort()
			return
		}
		uid, ok := userIDFromContext(c)
		if !ok {
			c.Status(http.StatusUnauthorized)
			c.Abort()
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			c.Status(http.StatusBadRequest)
			c.Abort()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		var n int64
		if err := db.WithContext(ctx).Model(&model.Project{}).
			Where("id = ? AND owner_user_id = ?", pid, uid).
			Count(&n).Error; err != nil || n == 0 {
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}
		c.Next()
	}
}

func RequireProjectKey(db *gorm.DB) gin.HandlerFunc {
	cache := newProjectKeyCache(10_000, 30*time.Second)
	return func(c *gin.Context) {
		if proxyOK(c) {
			c.Next()
			return
		}
		if db == nil {
			c.Status(http.StatusNotImplemented)
			c.Abort()
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			c.Status(http.StatusBadRequest)
			c.Abort()
			return
		}

		key := strings.TrimSpace(c.GetHeader("X-Project-Key"))
		if key == "" {
			// Support Authorization: Bearer pk_...
			authz := strings.TrimSpace(c.GetHeader("Authorization"))
			if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
				cand := strings.TrimSpace(authz[len("bearer "):])
				if strings.HasPrefix(cand, "pk_") {
					key = cand
				}
			}
		}
		if key == "" {
			// Support Sentry SDK auth header: "Sentry sentry_key=..., ..."
			key = sentryKeyFromHeader(c.GetHeader("X-Sentry-Auth"))
		}
		if key == "" {
			c.Status(http.StatusUnauthorized)
			c.Abort()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if v, hit := cache.Get(pid, key); hit {
			if !v {
				c.Status(http.StatusUnauthorized)
				c.Abort()
				return
			}
			c.Next()
			return
		}

		ok, err := store.ValidateProjectKey(ctx, db, pid, key)
		if err != nil || !ok {
			cache.Set(pid, key, false)
			c.Status(http.StatusUnauthorized)
			c.Abort()
			return
		}
		cache.Set(pid, key, true)
		c.Next()
	}
}

func sentryKeyFromHeader(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(h), "sentry ") {
		h = strings.TrimSpace(h[len("sentry "):])
	}
	parts := strings.Split(h, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(kv[0]))
		v := strings.Trim(strings.TrimSpace(kv[1]), "\"")
		if k == "sentry_key" && v != "" {
			return v
		}
	}
	return ""
}

type projectKeyCache struct {
	mu        sync.Mutex
	items     map[string]cacheEntry
	maxItems  int
	ttl       time.Duration
	lastPrune time.Time
}

type cacheEntry struct {
	ok    bool
	until time.Time
}

func newProjectKeyCache(maxItems int, ttl time.Duration) *projectKeyCache {
	if maxItems <= 0 {
		maxItems = 10_000
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &projectKeyCache{
		items:    map[string]cacheEntry{},
		maxItems: maxItems,
		ttl:      ttl,
	}
}

func (c *projectKeyCache) key(projectID int, key string) string {
	return fmt.Sprintf("%d:%s", projectID, key)
}

func (c *projectKeyCache) Get(projectID int, key string) (bool, bool) {
	if c == nil {
		return false, false
	}
	now := time.Now()
	k := c.key(projectID, key)

	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[k]
	if !ok {
		return false, false
	}
	if now.After(e.until) {
		delete(c.items, k)
		return false, false
	}
	return e.ok, true
}

func (c *projectKeyCache) Set(projectID int, key string, ok bool) {
	if c == nil {
		return
	}
	now := time.Now()
	k := c.key(projectID, key)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[k] = cacheEntry{ok: ok, until: now.Add(c.ttl)}

	if len(c.items) <= c.maxItems && now.Sub(c.lastPrune) < time.Minute {
		return
	}
	c.lastPrune = now
	for kk, e := range c.items {
		if now.After(e.until) {
			delete(c.items, kk)
		}
	}
	for len(c.items) > c.maxItems {
		for kk := range c.items {
			delete(c.items, kk)
			break
		}
	}
}
