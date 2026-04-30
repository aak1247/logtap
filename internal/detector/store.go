package detector

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// DetectorResult is the GORM model for persisted detector results.
type DetectorResult struct {
	ID           int64          `gorm:"primaryKey;autoIncrement;column:id"`
	DetectorType string         `gorm:"type:varchar(64);not null;index:idx_dr_type_project_time,priority:1;column:detector_type"`
	ProjectID    int            `gorm:"not null;index:idx_dr_type_project_time,priority:2;column:project_id"`
	MonitorID    int            `gorm:"not null;index;column:monitor_id"`
	Timestamp    time.Time      `gorm:"not null;index:idx_dr_type_project_time,priority:3;column:timestamp"`
	Data         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}';column:data"`
	Tags         datatypes.JSON `gorm:"type:jsonb;not null;default:'{}';column:tags"`
	CreatedAt    time.Time      `gorm:"not null;autoCreateTime;column:created_at"`
}

func (DetectorResult) TableName() string { return "detector_results" }

// ResultStore provides database-backed storage for detector results.
type ResultStore struct {
	db *gorm.DB
}

func NewResultStore(db *gorm.DB) *ResultStore {
	return &ResultStore{db: db}
}

// AutoMigrate creates the detector_results table.
func (s *ResultStore) AutoMigrate(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	if err := s.db.WithContext(ctx).AutoMigrate(&DetectorResult{}); err != nil {
		return fmt.Errorf("auto migrate detector_results: %w", err)
	}
	// Try to make it a hypertable if TimescaleDB is available (ignore errors).
	s.db.Exec(`SELECT create_hypertable('detector_results', 'timestamp', if_not_exists => TRUE, migrate_data => TRUE)`)
	return nil
}

// Store persists a batch of TypedResult entries.
func (s *ResultStore) Store(ctx context.Context, results []TypedResult) error {
	if s.db == nil || len(results) == 0 {
		return nil
	}
	rows := make([]*DetectorResult, 0, len(results))
	for i := range results {
		r := &results[i]
		tagsJSON, _ := json.Marshal(r.Tags)
		dataJSON := r.Data
		if len(dataJSON) == 0 {
			dataJSON = json.RawMessage(`{}`)
		}
		rows = append(rows, &DetectorResult{
			DetectorType: r.DetectorType,
			ProjectID:    r.ProjectID,
			MonitorID:    r.MonitorID,
			Timestamp:    r.Timestamp.UTC(),
			Data:         datatypes.JSON(dataJSON),
			Tags:         datatypes.JSON(tagsJSON),
		})
	}
	return s.db.WithContext(ctx).CreateInBatches(rows, 100).Error
}

// Query retrieves TypedResult entries matching the query parameters.
func (s *ResultStore) Query(ctx context.Context, detectorType string, q ResultQuery) ([]TypedResult, error) {
	if s.db == nil {
		return nil, nil
	}
	tx := s.db.WithContext(ctx).
		Where("detector_type = ?", detectorType).
		Where("project_id = ?", q.ProjectID)
	if q.MonitorID > 0 {
		tx = tx.Where("monitor_id = ?", q.MonitorID)
	}
	if !q.StartTime.IsZero() {
		tx = tx.Where("timestamp >= ?", q.StartTime)
	}
	if !q.EndTime.IsZero() {
		tx = tx.Where("timestamp <= ?", q.EndTime)
	}
	tx = tx.Order("timestamp DESC")
	if q.Limit > 0 {
		tx = tx.Limit(q.Limit)
	} else {
		tx = tx.Limit(1000)
	}

	var rows []DetectorResult
	if err := tx.Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]TypedResult, 0, len(rows))
	for _, r := range rows {
		tags := map[string]string{}
		_ = json.Unmarshal(r.Tags, &tags)
		out = append(out, TypedResult{
			DetectorType: r.DetectorType,
			ProjectID:    r.ProjectID,
			MonitorID:    r.MonitorID,
			Timestamp:    r.Timestamp,
			Data:         json.RawMessage(r.Data),
			Tags:         tags,
		})
	}
	return out, nil
}

// AggregateAvgFloat queries detector_results and computes the average of a JSONB
// field within a time range, bucketed by the given interval.
func (s *ResultStore) AggregateAvgFloat(ctx context.Context, detectorType string, projectID int, fieldName string, tr TimeRange, interval AggregateInterval) ([]MetricPoint, error) {
	if s.db == nil {
		return nil, nil
	}

	pgInterval := aggregateToPGInterval(interval)
	if pgInterval == "" {
		pgInterval = "1 hour"
	}

	query := fmt.Sprintf(`
		SELECT
			time_bucket('%s', timestamp) AS bucket,
			AVG((data->>'%s')::float8) AS avg_val
		FROM detector_results
		WHERE detector_type = ? AND project_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY bucket
		ORDER BY bucket ASC
	`, pgInterval, fieldName)

	type row struct {
		Bucket  time.Time
		AvgVal  float64
	}
	var rows []row
	if err := s.db.WithContext(ctx).Raw(query, detectorType, projectID, tr.Start, tr.End).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]MetricPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, MetricPoint{
			Timestamp: r.Bucket,
			Value:     r.AvgVal,
			Labels:    map[string]string{"metric": fieldName, "detector_type": detectorType},
		})
	}
	return out, nil
}

// AggregateSuccessRate computes the success rate (percentage of records where
// data->>'success' = 'true') bucketed by interval.
func (s *ResultStore) AggregateSuccessRate(ctx context.Context, detectorType string, projectID int, tr TimeRange, interval AggregateInterval) ([]MetricPoint, error) {
	if s.db == nil {
		return nil, nil
	}

	pgInterval := aggregateToPGInterval(interval)
	if pgInterval == "" {
		pgInterval = "1 hour"
	}

	query := fmt.Sprintf(`
		SELECT
			time_bucket('%s', timestamp) AS bucket,
			COUNT(*) FILTER (WHERE (data->>'success')::boolean = true) * 100.0 / NULLIF(COUNT(*), 0) AS rate
		FROM detector_results
		WHERE detector_type = ? AND project_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY bucket
		ORDER BY bucket ASC
	`, pgInterval)

	type row struct {
		Bucket time.Time
		Rate   float64
	}
	var rows []row
	if err := s.db.WithContext(ctx).Raw(query, detectorType, projectID, tr.Start, tr.End).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]MetricPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, MetricPoint{
			Timestamp: r.Bucket,
			Value:     r.Rate,
			Labels:    map[string]string{"metric": "success_rate", "detector_type": detectorType},
		})
	}
	return out, nil
}

func aggregateToPGInterval(interval AggregateInterval) string {
	switch interval {
	case IntervalMinute:
		return "1 minute"
	case IntervalHour:
		return "1 hour"
	case IntervalDay:
		return "1 day"
	default:
		return "1 hour"
	}
}
