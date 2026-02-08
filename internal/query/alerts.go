package query

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/alert"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ListAlertContactsHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		typ := strings.ToLower(strings.TrimSpace(c.Query("type")))
		if typ != "" && typ != "email" && typ != "sms" {
			respondErr(c, http.StatusBadRequest, "invalid type (expected email|sms)")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		q := db.WithContext(ctx).Where("project_id = ?", pid).Order("id ASC")
		if typ != "" {
			q = q.Where("type = ?", typ)
		}
		var items []model.AlertContact
		if err := q.Find(&items).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

func CreateAlertContactHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		var req struct {
			Type  string `json:"type"`
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		typ := strings.ToLower(strings.TrimSpace(req.Type))
		name := strings.TrimSpace(req.Name)
		value := strings.TrimSpace(req.Value)
		if typ != "email" && typ != "sms" {
			respondErr(c, http.StatusBadRequest, "invalid type (expected email|sms)")
			return
		}
		if value == "" {
			respondErr(c, http.StatusBadRequest, "value is required")
			return
		}
		if typ == "email" {
			value = strings.ToLower(value)
			if !looksLikeEmail(value) {
				respondErr(c, http.StatusBadRequest, "invalid email value")
				return
			}
		}
		if typ == "sms" {
			if !isE164(value) {
				respondErr(c, http.StatusBadRequest, "invalid phone value (expected E.164, e.g. +8613800138000)")
				return
			}
		}

		row := model.AlertContact{
			ProjectID: pid,
			Type:      typ,
			Name:      name,
			Value:     value,
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			if isUniqueViolation(err) {
				respondErr(c, http.StatusConflict, "contact already exists")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, row)
	}
}

func UpdateAlertContactHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		id64, err := strconv.ParseInt(strings.TrimSpace(c.Param("contactId")), 10, 32)
		if err != nil || id64 <= 0 {
			respondErr(c, http.StatusBadRequest, "invalid contactId")
			return
		}
		id := int(id64)

		var req struct {
			Name  *string `json:"name"`
			Value *string `json:"value"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var cur model.AlertContact
		if err := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, id).First(&cur).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		updates := map[string]any{}
		if req.Name != nil {
			updates["name"] = strings.TrimSpace(*req.Name)
		}
		if req.Value != nil {
			v := strings.TrimSpace(*req.Value)
			if v == "" {
				respondErr(c, http.StatusBadRequest, "value cannot be empty")
				return
			}
			if cur.Type == "email" {
				v = strings.ToLower(v)
				if !looksLikeEmail(v) {
					respondErr(c, http.StatusBadRequest, "invalid email value")
					return
				}
			}
			if cur.Type == "sms" {
				if !isE164(v) {
					respondErr(c, http.StatusBadRequest, "invalid phone value (expected E.164)")
					return
				}
			}
			updates["value"] = v
		}
		if len(updates) == 0 {
			respondOK(c, cur)
			return
		}

		if err := db.WithContext(ctx).Model(&model.AlertContact{}).Where("project_id = ? AND id = ?", pid, id).Updates(updates).Error; err != nil {
			if isUniqueViolation(err) {
				respondErr(c, http.StatusConflict, "contact already exists")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		if err := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, id).First(&cur).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, cur)
	}
}

func DeleteAlertContactHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		id64, err := strconv.ParseInt(strings.TrimSpace(c.Param("contactId")), 10, 32)
		if err != nil || id64 <= 0 {
			respondErr(c, http.StatusBadRequest, "invalid contactId")
			return
		}
		id := int(id64)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		tx := db.WithContext(ctx).Begin()
		if tx.Error != nil {
			respondErr(c, http.StatusServiceUnavailable, tx.Error.Error())
			return
		}
		defer func() { _ = tx.Rollback().Error }()

		var cur model.AlertContact
		if err := tx.Where("project_id = ? AND id = ?", pid, id).First(&cur).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		if err := tx.Where("contact_id = ?", id).Delete(&model.AlertContactGroupMember{}).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if err := tx.Where("project_id = ? AND id = ?", pid, id).Delete(&model.AlertContact{}).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if err := tx.Commit().Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"deleted": true})
	}
}

func ListAlertContactGroupsHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		typ := strings.ToLower(strings.TrimSpace(c.Query("type")))
		if typ != "" && typ != "email" && typ != "sms" {
			respondErr(c, http.StatusBadRequest, "invalid type (expected email|sms)")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		q := db.WithContext(ctx).Where("project_id = ?", pid).Order("id ASC")
		if typ != "" {
			q = q.Where("type = ?", typ)
		}
		var groups []model.AlertContactGroup
		if err := q.Find(&groups).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		groupIDs := make([]int, 0, len(groups))
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID)
		}

		membersByGroup := map[int][]int{}
		if len(groupIDs) > 0 {
			var members []model.AlertContactGroupMember
			if err := db.WithContext(ctx).Where("group_id IN ?", groupIDs).Find(&members).Error; err != nil {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			for _, m := range members {
				membersByGroup[m.GroupID] = append(membersByGroup[m.GroupID], m.ContactID)
			}
		}

		type item struct {
			model.AlertContactGroup
			MemberContactIDs []int `json:"memberContactIds"`
		}
		out := make([]item, 0, len(groups))
		for _, g := range groups {
			out = append(out, item{AlertContactGroup: g, MemberContactIDs: membersByGroup[g.ID]})
		}
		respondOK(c, gin.H{"items": out})
	}
}

func CreateAlertContactGroupHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		var req struct {
			Type             string `json:"type"`
			Name             string `json:"name"`
			MemberContactIDs []int  `json:"memberContactIds"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		typ := strings.ToLower(strings.TrimSpace(req.Type))
		name := strings.TrimSpace(req.Name)
		if typ != "email" && typ != "sms" {
			respondErr(c, http.StatusBadRequest, "invalid type (expected email|sms)")
			return
		}
		if name == "" {
			respondErr(c, http.StatusBadRequest, "name is required")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		tx := db.WithContext(ctx).Begin()
		if tx.Error != nil {
			respondErr(c, http.StatusServiceUnavailable, tx.Error.Error())
			return
		}
		defer func() { _ = tx.Rollback().Error }()

		group := model.AlertContactGroup{ProjectID: pid, Type: typ, Name: name}
		if err := tx.Create(&group).Error; err != nil {
			if isUniqueViolation(err) {
				respondErr(c, http.StatusConflict, "group already exists")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		if err := ensureContactsExist(ctx, tx, pid, typ, req.MemberContactIDs); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		if len(req.MemberContactIDs) > 0 {
			rows := make([]model.AlertContactGroupMember, 0, len(req.MemberContactIDs))
			for _, cid := range uniquePositive(req.MemberContactIDs) {
				rows = append(rows, model.AlertContactGroupMember{GroupID: group.ID, ContactID: cid})
			}
			if len(rows) > 0 {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
					respondErr(c, http.StatusServiceUnavailable, err.Error())
					return
				}
			}
		}

		if err := tx.Commit().Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"group": group, "memberContactIds": uniquePositive(req.MemberContactIDs)})
	}
}

func UpdateAlertContactGroupHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		id64, err := strconv.ParseInt(strings.TrimSpace(c.Param("groupId")), 10, 32)
		if err != nil || id64 <= 0 {
			respondErr(c, http.StatusBadRequest, "invalid groupId")
			return
		}
		gid := int(id64)

		var req struct {
			Name             *string `json:"name"`
			MemberContactIDs *[]int  `json:"memberContactIds"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		tx := db.WithContext(ctx).Begin()
		if tx.Error != nil {
			respondErr(c, http.StatusServiceUnavailable, tx.Error.Error())
			return
		}
		defer func() { _ = tx.Rollback().Error }()

		var group model.AlertContactGroup
		if err := tx.Where("project_id = ? AND id = ?", pid, gid).First(&group).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if name == "" {
				respondErr(c, http.StatusBadRequest, "name cannot be empty")
				return
			}
			if err := tx.Model(&model.AlertContactGroup{}).Where("project_id = ? AND id = ?", pid, gid).Update("name", name).Error; err != nil {
				if isUniqueViolation(err) {
					respondErr(c, http.StatusConflict, "group already exists")
					return
				}
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			group.Name = name
		}

		memberIDs := []int(nil)
		if req.MemberContactIDs != nil {
			memberIDs = uniquePositive(*req.MemberContactIDs)
			if err := ensureContactsExist(ctx, tx, pid, group.Type, memberIDs); err != nil {
				respondErr(c, http.StatusBadRequest, err.Error())
				return
			}
			if err := tx.Where("group_id = ?", gid).Delete(&model.AlertContactGroupMember{}).Error; err != nil {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			rows := make([]model.AlertContactGroupMember, 0, len(memberIDs))
			for _, cid := range memberIDs {
				rows = append(rows, model.AlertContactGroupMember{GroupID: gid, ContactID: cid})
			}
			if len(rows) > 0 {
				if err := tx.Create(&rows).Error; err != nil {
					respondErr(c, http.StatusServiceUnavailable, err.Error())
					return
				}
			}
		} else {
			var members []model.AlertContactGroupMember
			if err := tx.Where("group_id = ?", gid).Find(&members).Error; err != nil {
				respondErr(c, http.StatusServiceUnavailable, err.Error())
				return
			}
			for _, m := range members {
				memberIDs = append(memberIDs, m.ContactID)
			}
		}

		if err := tx.Commit().Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"group": group, "memberContactIds": memberIDs})
	}
}

func DeleteAlertContactGroupHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		id64, err := strconv.ParseInt(strings.TrimSpace(c.Param("groupId")), 10, 32)
		if err != nil || id64 <= 0 {
			respondErr(c, http.StatusBadRequest, "invalid groupId")
			return
		}
		gid := int(id64)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		tx := db.WithContext(ctx).Begin()
		if tx.Error != nil {
			respondErr(c, http.StatusServiceUnavailable, tx.Error.Error())
			return
		}
		defer func() { _ = tx.Rollback().Error }()

		var group model.AlertContactGroup
		if err := tx.Where("project_id = ? AND id = ?", pid, gid).First(&group).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if err := tx.Where("group_id = ?", gid).Delete(&model.AlertContactGroupMember{}).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if err := tx.Where("project_id = ? AND id = ?", pid, gid).Delete(&model.AlertContactGroup{}).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if err := tx.Commit().Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"deleted": true})
	}
}

func ListAlertWecomBotsHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var items []model.AlertWecomBot
		if err := db.WithContext(ctx).Where("project_id = ?", pid).Order("id ASC").Find(&items).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

func CreateAlertWecomBotHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		var req struct {
			Name       string `json:"name"`
			WebhookURL string `json:"webhookUrl"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		name := strings.TrimSpace(req.Name)
		u := strings.TrimSpace(req.WebhookURL)
		if name == "" || u == "" {
			respondErr(c, http.StatusBadRequest, "name and webhookUrl are required")
			return
		}
		if !looksLikeURL(u) {
			respondErr(c, http.StatusBadRequest, "invalid webhookUrl")
			return
		}

		row := model.AlertWecomBot{ProjectID: pid, Name: name, WebhookURL: u}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			if isUniqueViolation(err) {
				respondErr(c, http.StatusConflict, "wecom bot already exists")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, row)
	}
}

func UpdateAlertWecomBotHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		id64, err := strconv.ParseInt(strings.TrimSpace(c.Param("botId")), 10, 32)
		if err != nil || id64 <= 0 {
			respondErr(c, http.StatusBadRequest, "invalid botId")
			return
		}
		botID := int(id64)

		var req struct {
			Name       *string `json:"name"`
			WebhookURL *string `json:"webhookUrl"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var cur model.AlertWecomBot
		if err := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, botID).First(&cur).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		updates := map[string]any{}
		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if name == "" {
				respondErr(c, http.StatusBadRequest, "name cannot be empty")
				return
			}
			updates["name"] = name
		}
		if req.WebhookURL != nil {
			u := strings.TrimSpace(*req.WebhookURL)
			if u == "" || !looksLikeURL(u) {
				respondErr(c, http.StatusBadRequest, "invalid webhookUrl")
				return
			}
			updates["webhook_url"] = u
		}
		if len(updates) == 0 {
			respondOK(c, cur)
			return
		}

		if err := db.WithContext(ctx).Model(&model.AlertWecomBot{}).Where("project_id = ? AND id = ?", pid, botID).Updates(updates).Error; err != nil {
			if isUniqueViolation(err) {
				respondErr(c, http.StatusConflict, "wecom bot already exists")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if err := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, botID).First(&cur).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, cur)
	}
}

func DeleteAlertWecomBotHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		id64, err := strconv.ParseInt(strings.TrimSpace(c.Param("botId")), 10, 32)
		if err != nil || id64 <= 0 {
			respondErr(c, http.StatusBadRequest, "invalid botId")
			return
		}
		botID := int(id64)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		res := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, botID).Delete(&model.AlertWecomBot{})
		if res.Error != nil {
			respondErr(c, http.StatusServiceUnavailable, res.Error.Error())
			return
		}
		if res.RowsAffected == 0 {
			respondErr(c, http.StatusNotFound, "not found")
			return
		}
		respondOK(c, gin.H{"deleted": true})
	}
}

func ListAlertWebhookEndpointsHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var items []model.AlertWebhookEndpoint
		if err := db.WithContext(ctx).Where("project_id = ?", pid).Order("id ASC").Find(&items).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

func CreateAlertWebhookEndpointHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		var req struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		name := strings.TrimSpace(req.Name)
		u := strings.TrimSpace(req.URL)
		if name == "" || u == "" {
			respondErr(c, http.StatusBadRequest, "name and url are required")
			return
		}
		if !looksLikeURL(u) {
			respondErr(c, http.StatusBadRequest, "invalid url")
			return
		}

		row := model.AlertWebhookEndpoint{ProjectID: pid, Name: name, URL: u}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			if isUniqueViolation(err) {
				respondErr(c, http.StatusConflict, "webhook endpoint already exists")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, row)
	}
}

func UpdateAlertWebhookEndpointHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		id64, err := strconv.ParseInt(strings.TrimSpace(c.Param("endpointId")), 10, 32)
		if err != nil || id64 <= 0 {
			respondErr(c, http.StatusBadRequest, "invalid endpointId")
			return
		}
		eid := int(id64)

		var req struct {
			Name *string `json:"name"`
			URL  *string `json:"url"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		var cur model.AlertWebhookEndpoint
		if err := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, eid).First(&cur).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		updates := map[string]any{}
		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if name == "" {
				respondErr(c, http.StatusBadRequest, "name cannot be empty")
				return
			}
			updates["name"] = name
		}
		if req.URL != nil {
			u := strings.TrimSpace(*req.URL)
			if u == "" || !looksLikeURL(u) {
				respondErr(c, http.StatusBadRequest, "invalid url")
				return
			}
			updates["url"] = u
		}
		if len(updates) == 0 {
			respondOK(c, cur)
			return
		}

		if err := db.WithContext(ctx).Model(&model.AlertWebhookEndpoint{}).Where("project_id = ? AND id = ?", pid, eid).Updates(updates).Error; err != nil {
			if isUniqueViolation(err) {
				respondErr(c, http.StatusConflict, "webhook endpoint already exists")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if err := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, eid).First(&cur).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, cur)
	}
}

func DeleteAlertWebhookEndpointHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		id64, err := strconv.ParseInt(strings.TrimSpace(c.Param("endpointId")), 10, 32)
		if err != nil || id64 <= 0 {
			respondErr(c, http.StatusBadRequest, "invalid endpointId")
			return
		}
		eid := int(id64)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		res := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, eid).Delete(&model.AlertWebhookEndpoint{})
		if res.Error != nil {
			respondErr(c, http.StatusServiceUnavailable, res.Error.Error())
			return
		}
		if res.RowsAffected == 0 {
			respondErr(c, http.StatusNotFound, "not found")
			return
		}
		respondOK(c, gin.H{"deleted": true})
	}
}

func ListAlertRulesHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var items []model.AlertRule
		if err := db.WithContext(ctx).Where("project_id = ?", pid).Order("id ASC").Find(&items).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, gin.H{"items": items})
	}
}

func CreateAlertRuleHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		var req ruleUpsertRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		row, err := buildRuleFromRequest(c.Request.Context(), db, pid, 0, req)
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, row)
	}
}

func UpdateAlertRuleHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		id64, err := strconv.ParseInt(strings.TrimSpace(c.Param("ruleId")), 10, 32)
		if err != nil || id64 <= 0 {
			respondErr(c, http.StatusBadRequest, "invalid ruleId")
			return
		}
		rid := int(id64)

		var req ruleUpsertRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var cur model.AlertRule
		if err := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, rid).First(&cur).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondErr(c, http.StatusNotFound, "not found")
				return
			}
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		row, err := buildRuleFromRequest(c.Request.Context(), db, pid, rid, req)
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}

		updates := map[string]any{
			"name":    row.Name,
			"enabled": row.Enabled,
			"source":  row.Source,
			"match":   row.Match,
			"repeat":  row.Repeat,
			"targets": row.Targets,
		}
		if err := db.WithContext(ctx).Model(&model.AlertRule{}).Where("project_id = ? AND id = ?", pid, rid).Updates(updates).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}

		if err := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, rid).First(&cur).Error; err != nil {
			respondErr(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respondOK(c, cur)
	}
}

func DeleteAlertRuleHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			respondErr(c, http.StatusNotImplemented, "database not configured")
			return
		}
		pid, err := project.ParseID(c.Param("projectId"))
		if err != nil {
			respondErr(c, http.StatusBadRequest, err.Error())
			return
		}
		id64, err := strconv.ParseInt(strings.TrimSpace(c.Param("ruleId")), 10, 32)
		if err != nil || id64 <= 0 {
			respondErr(c, http.StatusBadRequest, "invalid ruleId")
			return
		}
		rid := int(id64)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		res := db.WithContext(ctx).Where("project_id = ? AND id = ?", pid, rid).Delete(&model.AlertRule{})
		if res.Error != nil {
			respondErr(c, http.StatusServiceUnavailable, res.Error.Error())
			return
		}
		if res.RowsAffected == 0 {
			respondErr(c, http.StatusNotFound, "not found")
			return
		}
		respondOK(c, gin.H{"deleted": true})
	}
}

type ruleUpsertRequest struct {
	Name    string          `json:"name"`
	Enabled *bool           `json:"enabled,omitempty"`
	Source  string          `json:"source"`
	Match   json.RawMessage `json:"match"`
	Repeat  json.RawMessage `json:"repeat"`
	Targets json.RawMessage `json:"targets"`
}

func buildRuleFromRequest(ctx context.Context, db *gorm.DB, projectID int, ruleID int, req ruleUpsertRequest) (model.AlertRule, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return model.AlertRule{}, errors.New("name is required")
	}
	source := strings.ToLower(strings.TrimSpace(req.Source))
	if source == "" {
		source = string(alert.SourceBoth)
	}
	switch source {
	case string(alert.SourceLogs), string(alert.SourceEvents), string(alert.SourceBoth):
	default:
		return model.AlertRule{}, errors.New("invalid source (expected logs|events|both)")
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	match := req.Match
	if len(match) == 0 {
		match = []byte(`{}`)
	}
	repeat := req.Repeat
	if len(repeat) == 0 {
		repeat = []byte(`{}`)
	}
	targets := req.Targets
	if len(targets) == 0 {
		targets = []byte(`{}`)
	}

	var m alert.RuleMatch
	if err := json.Unmarshal(match, &m); err != nil {
		return model.AlertRule{}, errors.New("invalid match json")
	}
	var r alert.RuleRepeat
	if err := json.Unmarshal(repeat, &r); err != nil {
		return model.AlertRule{}, errors.New("invalid repeat json")
	}
	var t alert.RuleTargets
	if err := json.Unmarshal(targets, &t); err != nil {
		return model.AlertRule{}, errors.New("invalid targets json")
	}
	if err := validateTargets(ctx, db, projectID, t); err != nil {
		return model.AlertRule{}, err
	}

	row := model.AlertRule{
		ID:        ruleID,
		ProjectID: projectID,
		Name:      name,
		Enabled:   enabled,
		Source:    source,
		Match:     datatypes.JSON(match),
		Repeat:    datatypes.JSON(repeat),
		Targets:   datatypes.JSON(targets),
	}
	return row, nil
}

func validateTargets(ctx context.Context, db *gorm.DB, projectID int, t alert.RuleTargets) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := validateIDsExist(ctx, db, "alert_contact_groups", projectID, "email", t.EmailGroupIDs); err != nil {
		return err
	}
	if err := validateIDsExist(ctx, db, "alert_contacts", projectID, "email", t.EmailContactIDs); err != nil {
		return err
	}
	if err := validateIDsExist(ctx, db, "alert_contact_groups", projectID, "sms", t.SMSGroupIDs); err != nil {
		return err
	}
	if err := validateIDsExist(ctx, db, "alert_contacts", projectID, "sms", t.SMSContactIDs); err != nil {
		return err
	}

	if err := validateProjectIDsExist(ctx, db, "alert_wecom_bots", projectID, t.WecomBotIDs); err != nil {
		return err
	}
	if err := validateProjectIDsExist(ctx, db, "alert_webhook_endpoints", projectID, t.WebhookEndpointIDs); err != nil {
		return err
	}
	return nil
}

func validateIDsExist(ctx context.Context, db *gorm.DB, table string, projectID int, typ string, ids []int) error {
	uniq := uniquePositive(ids)
	if len(uniq) == 0 {
		return nil
	}
	var count int64
	if err := db.WithContext(ctx).Table(table).Where("project_id = ? AND type = ? AND id IN ?", projectID, typ, uniq).Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(uniq) {
		return errors.New("targets reference missing ids")
	}
	return nil
}

func validateProjectIDsExist(ctx context.Context, db *gorm.DB, table string, projectID int, ids []int) error {
	uniq := uniquePositive(ids)
	if len(uniq) == 0 {
		return nil
	}
	var count int64
	if err := db.WithContext(ctx).Table(table).Where("project_id = ? AND id IN ?", projectID, uniq).Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(uniq) {
		return errors.New("targets reference missing ids")
	}
	return nil
}

func ensureContactsExist(ctx context.Context, db *gorm.DB, projectID int, typ string, ids []int) error {
	uniq := uniquePositive(ids)
	if len(uniq) == 0 {
		return nil
	}
	var count int64
	if err := db.WithContext(ctx).Model(&model.AlertContact{}).Where("project_id = ? AND type = ? AND id IN ?", projectID, typ, uniq).Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(uniq) {
		return errors.New("memberContactIds reference missing contacts")
	}
	return nil
}

func uniquePositive(ids []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func isE164(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "+") {
		return false
	}
	d := s[1:]
	if len(d) < 8 || len(d) > 15 {
		return false
	}
	for _, ch := range d {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func looksLikeEmail(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at >= len(s)-1 {
		return false
	}
	if strings.Contains(s, " ") {
		return false
	}
	return true
}

func looksLikeURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return strings.TrimSpace(u.Host) != ""
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "sqlstate 23505") ||
		strings.Contains(s, "unique") ||
		strings.Contains(s, "duplicate") ||
		strings.Contains(s, "重复键") ||
		strings.Contains(s, "唯一约束")
}
