package migration

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
)

type ExportOptions struct {
	OwnerUserID                int64
	ProjectIDs                 []int
	Since                      *time.Time
	Until                      *time.Time
	IncludeNotificationSecrets bool
}

type Preview struct {
	CloudDefaultURL string           `json:"cloud_default_url,omitempty"`
	Projects        []PreviewProject `json:"projects"`
	TotalLogs       int64            `json:"total_logs"`
	TotalEvents     int64            `json:"total_events"`
}

type PreviewProject struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Logs   int64  `json:"logs"`
	Events int64  `json:"events"`
}

func BuildPreview(ctx context.Context, db *gorm.DB, ownerUserID int64, projectIDs []int, since *time.Time, until *time.Time, cloudDefaultURL string) (Preview, error) {
	projects, err := listExportProjects(ctx, db, ownerUserID, projectIDs)
	if err != nil {
		return Preview{}, err
	}
	out := Preview{CloudDefaultURL: strings.TrimSpace(cloudDefaultURL), Projects: make([]PreviewProject, 0, len(projects))}
	for _, p := range projects {
		logs, err := countTableByProjectAndRange(ctx, db, &model.Log{}, p.ID, "timestamp", since, until)
		if err != nil {
			return Preview{}, err
		}
		events, err := countTableByProjectAndRange(ctx, db, &model.Event{}, p.ID, "timestamp", since, until)
		if err != nil {
			return Preview{}, err
		}
		out.Projects = append(out.Projects, PreviewProject{ID: p.ID, Name: p.Name, Logs: logs, Events: events})
		out.TotalLogs += logs
		out.TotalEvents += events
	}
	return out, nil
}

func ExportBundle(ctx context.Context, db *gorm.DB, opts ExportOptions) (Bundle, error) {
	if db == nil {
		return Bundle{}, errors.New("database not configured")
	}
	projects, err := listExportProjects(ctx, db, opts.OwnerUserID, opts.ProjectIDs)
	if err != nil {
		return Bundle{}, err
	}
	b := NewBundle(opts.IncludeNotificationSecrets)
	b.Projects = make([]ProjectBundle, 0, len(projects))
	for _, p := range projects {
		pb, err := exportProject(ctx, db, p, opts)
		if err != nil {
			return Bundle{}, err
		}
		b.Projects = append(b.Projects, pb)
	}
	return b, nil
}

func listExportProjects(ctx context.Context, db *gorm.DB, ownerUserID int64, projectIDs []int) ([]model.Project, error) {
	if db == nil {
		return nil, errors.New("database not configured")
	}
	q := db.WithContext(ctx).
		Where("is_system = ?", false).
		Order("id ASC")
	if ownerUserID > 0 {
		q = q.Where("owner_user_id = ?", ownerUserID)
	}
	if len(projectIDs) > 0 {
		q = q.Where("id IN ?", positiveProjectIDs(projectIDs))
	}
	var projects []model.Project
	if err := q.Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func positiveProjectIDs(ids []int) []int {
	out := make([]int, 0, len(ids))
	seen := map[int]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func countTableByProjectAndRange(ctx context.Context, db *gorm.DB, modelValue any, projectID int, tsColumn string, since *time.Time, until *time.Time) (int64, error) {
	q := db.WithContext(ctx).Model(modelValue).Where("project_id = ?", projectID)
	if since != nil && !since.IsZero() {
		q = q.Where(tsColumn+" >= ?", since.UTC())
	}
	if until != nil && !until.IsZero() {
		q = q.Where(tsColumn+" <= ?", until.UTC())
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func exportProject(ctx context.Context, db *gorm.DB, p model.Project, opts ExportOptions) (ProjectBundle, error) {
	logCount, err := countTableByProjectAndRange(ctx, db, &model.Log{}, p.ID, "timestamp", opts.Since, opts.Until)
	if err != nil {
		return ProjectBundle{}, err
	}
	eventCount, err := countTableByProjectAndRange(ctx, db, &model.Event{}, p.ID, "timestamp", opts.Since, opts.Until)
	if err != nil {
		return ProjectBundle{}, err
	}
	pb := ProjectBundle{
		SourceProjectID: p.ID,
		Name:            p.Name,
		ExportedAt:      time.Now().UTC(),
		Counts:          ProjectCounts{Logs: logCount, Events: eventCount},
	}
	if err := exportProjectConfig(ctx, db, p.ID, opts.IncludeNotificationSecrets, &pb.Config); err != nil {
		return ProjectBundle{}, err
	}
	if err := exportLogs(ctx, db, p.ID, opts.Since, opts.Until, &pb); err != nil {
		return ProjectBundle{}, err
	}
	if err := exportEvents(ctx, db, p.ID, opts.Since, opts.Until, &pb); err != nil {
		return ProjectBundle{}, err
	}
	return pb, nil
}

func exportLogs(ctx context.Context, db *gorm.DB, projectID int, since *time.Time, until *time.Time, pb *ProjectBundle) error {
	q := db.WithContext(ctx).Where("project_id = ?", projectID)
	if since != nil && !since.IsZero() {
		q = q.Where("timestamp >= ?", since.UTC())
	}
	if until != nil && !until.IsZero() {
		q = q.Where("timestamp <= ?", until.UTC())
	}
	var rows []model.Log
	if err := q.Order("timestamp ASC, id ASC").Find(&rows).Error; err != nil {
		return err
	}
	pb.Logs = make([]LogRecord, 0, len(rows))
	for _, r := range rows {
		pb.Logs = append(pb.Logs, LogRecord{
			SourceID:   r.ID,
			Timestamp:  r.Timestamp.UTC(),
			IngestID:   r.IngestID,
			Level:      r.Level,
			DistinctID: r.DistinctID,
			DeviceID:   r.DeviceID,
			TraceID:    r.TraceID,
			SpanID:     r.SpanID,
			Message:    r.Message,
			Fields:     rawOrObject(r.Fields),
		})
	}
	return nil
}

func exportEvents(ctx context.Context, db *gorm.DB, projectID int, since *time.Time, until *time.Time, pb *ProjectBundle) error {
	q := db.WithContext(ctx).Where("project_id = ?", projectID)
	if since != nil && !since.IsZero() {
		q = q.Where("timestamp >= ?", since.UTC())
	}
	if until != nil && !until.IsZero() {
		q = q.Where("timestamp <= ?", until.UTC())
	}
	var rows []model.Event
	if err := q.Order("timestamp ASC, id ASC").Find(&rows).Error; err != nil {
		return err
	}
	pb.Events = make([]EventRecord, 0, len(rows))
	for _, r := range rows {
		pb.Events = append(pb.Events, EventRecord{
			ID:          r.ID,
			Timestamp:   r.Timestamp.UTC(),
			Level:       r.Level,
			DistinctID:  r.DistinctID,
			DeviceID:    r.DeviceID,
			OS:          r.OS,
			Platform:    r.Platform,
			ReleaseTag:  r.ReleaseTag,
			Environment: r.Environment,
			UserID:      r.UserID,
			Title:       r.Title,
			Data:        rawOrObject(r.Data),
		})
	}
	return nil
}
