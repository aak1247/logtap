package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

type StorageEstimateTable struct {
	Count       int64 `json:"count"`
	SampleSize  int   `json:"sample_size"`
	AvgRowBytes int64 `json:"avg_row_bytes"`
	EstBytes    int64 `json:"est_bytes"`
}

type StorageEstimate struct {
	ProjectID   int                  `json:"project_id"`
	Logs        StorageEstimateTable `json:"logs"`
	Events      StorageEstimateTable `json:"events"`
	TotalBytes  int64                `json:"total_bytes"`
	EstimatedAt time.Time            `json:"estimated_at"`
}

func EstimateProjectStorage(ctx context.Context, db *gorm.DB, projectID int, sampleSize int) (StorageEstimate, error) {
	if db == nil || projectID <= 0 {
		return StorageEstimate{}, gorm.ErrInvalidDB
	}
	if sampleSize <= 0 {
		sampleSize = 200
	}
	if sampleSize > 2000 {
		sampleSize = 2000
	}

	now := time.Now().UTC()

	logsCount, err := countByProject(ctx, db, "logs", projectID)
	if err != nil {
		return StorageEstimate{}, err
	}
	eventsCount, err := countByProject(ctx, db, "events", projectID)
	if err != nil {
		return StorageEstimate{}, err
	}

	logsAvg, logsSample, err := avgLogRowBytes(ctx, db, projectID, sampleSize)
	if err != nil {
		return StorageEstimate{}, err
	}
	eventsAvg, eventsSample, err := avgEventRowBytes(ctx, db, projectID, sampleSize)
	if err != nil {
		return StorageEstimate{}, err
	}

	logsEst := safeMul(logsCount, logsAvg)
	eventsEst := safeMul(eventsCount, eventsAvg)

	return StorageEstimate{
		ProjectID: projectID,
		Logs: StorageEstimateTable{
			Count:       logsCount,
			SampleSize:  logsSample,
			AvgRowBytes: logsAvg,
			EstBytes:    logsEst,
		},
		Events: StorageEstimateTable{
			Count:       eventsCount,
			SampleSize:  eventsSample,
			AvgRowBytes: eventsAvg,
			EstBytes:    eventsEst,
		},
		TotalBytes:  logsEst + eventsEst,
		EstimatedAt: now,
	}, nil
}

func countByProject(ctx context.Context, db *gorm.DB, table string, projectID int) (int64, error) {
	var out int64
	err := db.WithContext(ctx).Raw(
		fmt.Sprintf("SELECT COUNT(1) FROM %s WHERE project_id = ?", table),
		projectID,
	).Scan(&out).Error
	return out, err
}

func avgLogRowBytes(ctx context.Context, db *gorm.DB, projectID int, sampleSize int) (avgBytes int64, sampled int, err error) {
	return avgRowBytes(
		ctx,
		db,
		projectID,
		sampleSize,
		"logs",
		"log",
	)
}

func avgEventRowBytes(ctx context.Context, db *gorm.DB, projectID int, sampleSize int) (avgBytes int64, sampled int, err error) {
	return avgRowBytes(
		ctx,
		db,
		projectID,
		sampleSize,
		"events",
		"event",
	)
}

func avgRowBytes(ctx context.Context, db *gorm.DB, projectID int, sampleSize int, table string, kind string) (avgBytes int64, sampled int, err error) {
	dialect := strings.ToLower(strings.TrimSpace(db.Dialector.Name()))
	lenFn := "length"
	if dialect == "postgres" {
		lenFn = "octet_length"
	}

	var payloadExpr string
	switch kind {
	case "log":
		fieldsExpr := "COALESCE(CAST(fields AS TEXT), '')"
		if dialect == "postgres" {
			fieldsExpr = "COALESCE(fields::text, '')"
		}
		payloadExpr = fmt.Sprintf("%s(COALESCE(message, '')) + %s(%s)", lenFn, lenFn, fieldsExpr)
	case "event":
		dataExpr := "COALESCE(CAST(data AS TEXT), '')"
		if dialect == "postgres" {
			dataExpr = "COALESCE(data::text, '')"
		}
		payloadExpr = fmt.Sprintf("%s(%s)", lenFn, dataExpr)
	default:
		return 0, 0, fmt.Errorf("unknown kind: %s", kind)
	}

	baseOverhead := int64(120)
	if kind == "event" {
		baseOverhead = 96
	}

	var avg sql.NullFloat64
	q := fmt.Sprintf(
		`SELECT AVG(sz) FROM (
			SELECT (%s + %d) AS sz
			FROM %s
			WHERE project_id = ?
			ORDER BY timestamp DESC
			LIMIT ?
		) t`,
		payloadExpr,
		baseOverhead,
		table,
	)
	if err := db.WithContext(ctx).Raw(q, projectID, sampleSize).Scan(&avg).Error; err != nil {
		return 0, 0, err
	}
	if !avg.Valid {
		return 0, 0, nil
	}
	avgBytes = int64(math.Round(avg.Float64))

	// Try to report the real sample size.
	var n int64
	nq := fmt.Sprintf(`SELECT COUNT(1) FROM (SELECT 1 FROM %s WHERE project_id = ? LIMIT ?) t`, table)
	if err := db.WithContext(ctx).Raw(nq, projectID, sampleSize).Scan(&n).Error; err == nil {
		sampled = int(n)
	}
	return avgBytes, sampled, nil
}

func safeMul(a, b int64) int64 {
	const maxInt64 = int64(^uint64(0) >> 1)
	if a == 0 || b == 0 {
		return 0
	}
	if a > maxInt64/b {
		return maxInt64
	}
	return a * b
}
