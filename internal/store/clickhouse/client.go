// Package clickhouse implements the ClickHouse storage backend: connection
// management, idempotent schema migrations, row encoders and the async batch
// writer. The schema and batching parameters follow
// docs/CLICKHOUSE_STORAGE_DESIGN.md §4-§5.
package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Clients holds the write and read connections. Production topologies point
// Write at the primary replica and Read at a read-only replica (design
// §8.1.2); single-node setups point both at the same server.
type Clients struct {
	Write driver.Conn
	Read  driver.Conn
}

// Open connects to ClickHouse. An empty readDSN reuses the write connection.
func Open(writeDSN, readDSN string) (*Clients, error) {
	w, err := openConn(writeDSN)
	if err != nil {
		return nil, fmt.Errorf("clickhouse write conn: %w", err)
	}
	r := w
	if readDSN != "" && readDSN != writeDSN {
		if r, err = openConn(readDSN); err != nil {
			_ = w.Close()
			return nil, fmt.Errorf("clickhouse read conn: %w", err)
		}
	}
	return &Clients{Write: w, Read: r}, nil
}

func openConn(dsn string) (driver.Conn, error) {
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	opts.Compression = &clickhouse.Compression{Method: clickhouse.CompressionLZ4}
	if opts.DialTimeout == 0 {
		opts.DialTimeout = 5 * time.Second
	}
	return clickhouse.Open(opts)
}

// Ping checks both connections.
func (c *Clients) Ping(ctx context.Context) error {
	if err := c.Write.Ping(ctx); err != nil {
		return fmt.Errorf("clickhouse write ping: %w", err)
	}
	if c.Read != c.Write {
		if err := c.Read.Ping(ctx); err != nil {
			return fmt.Errorf("clickhouse read ping: %w", err)
		}
	}
	return nil
}

// Close releases both connections.
func (c *Clients) Close() {
	if c.Write != nil {
		_ = c.Write.Close()
	}
	if c.Read != nil && c.Read != c.Write {
		_ = c.Read.Close()
	}
}

// WaitForClickHouse retries Open until the server accepts a connection or
// ctx expires, mirroring waitForPostgres in cmd/gateway.
func WaitForClickHouse(ctx context.Context, writeDSN, readDSN string) (*Clients, error) {
	const maxDelay = 5 * time.Second
	delay := 300 * time.Millisecond
	var lastErr error
	for {
		var clients *Clients
		clients, lastErr = Open(writeDSN, readDSN)
		if lastErr == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			lastErr = clients.Ping(pingCtx)
			cancel()
			if lastErr == nil {
				return clients, nil
			}
			clients.Close()
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("clickhouse not ready: %w (last error: %v)", ctx.Err(), lastErr)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("clickhouse not ready: %w (last error: %v)", ctx.Err(), lastErr)
		case <-timer.C:
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}
