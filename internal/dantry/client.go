package dantry

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// ClickHouseClient wraps a ClickHouse connection for Dantry queries
type ClickHouseClient struct {
	conn driver.Conn
}

// ClientConfig holds ClickHouse connection parameters
type ClientConfig struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

// NewClickHouseClient creates a new ClickHouse client for error_logs
func NewClickHouseClient(cfg ClientConfig) (*ClickHouseClient, error) {
	options := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.User,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:      5 * time.Second,
		MaxOpenConns:     3,
		MaxIdleConns:     2,
		ConnMaxLifetime:  5 * time.Minute,
		ConnOpenStrategy: clickhouse.ConnOpenInOrder,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	}

	// TLS for non-private networks
	if cfg.Host != "localhost" && cfg.Host != "127.0.0.1" && cfg.Host != "host.docker.internal" &&
		!(len(cfg.Host) >= 3 && cfg.Host[:3] == "10.") &&
		!(len(cfg.Host) >= 4 && cfg.Host[:4] == "172.") &&
		!(len(cfg.Host) >= 8 && cfg.Host[:8] == "192.168.") {
		options.TLS = &tls.Config{InsecureSkipVerify: true}
	}

	conn, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("dantry: failed to open ClickHouse: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("dantry: failed to ping ClickHouse: %w", err)
	}

	return &ClickHouseClient{conn: conn}, nil
}

// Query executes a SELECT and returns rows
func (c *ClickHouseClient) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	return c.conn.Query(ctx, query, args...)
}

// QueryRow executes a query returning a single row
func (c *ClickHouseClient) QueryRow(ctx context.Context, query string, args ...interface{}) driver.Row {
	return c.conn.QueryRow(ctx, query, args...)
}

// Close closes the connection
func (c *ClickHouseClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
