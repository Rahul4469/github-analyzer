package models

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// Database wraps the PostgreSQL connection pool.
// Using pgxpool provides:
// - Connection pooling (reuse connections)
// - Automatic reconnection on failure
// - Prepared statement caching
// - Context-aware queries with timeouts
type Database struct {
	Pool *pgxpool.Pool
}

// DatabaseConfig holds settings for the database connection.
type DatabaseConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

func DefaultDatabaseConfig(url string) DatabaseConfig {
	return DatabaseConfig{
		URL:             url,
		MaxConns:        25,               // Max connections in pool
		MinConns:        5,                // Keep at least this many open
		MaxConnLifetime: 1 * time.Hour,    // Recycle connections periodically
		MaxConnIdleTime: 30 * time.Minute, // Close idle connections
	}
}

func NewDatabase(ctx context.Context, cfg DatabaseConfig) (*Database, error) {
	// Parse the connection string and apply pool configuration
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Apply pool settings
	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime

	// Create the connection pool
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connectivity
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Database{Pool: pool}, nil
}

// Close should be called while shutting down db connection- via defer
func (db *Database) Close() {
	db.Pool.Close()
}

// to check databse connection (like ping from the good old days)
func (db *Database) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return db.Pool.Ping(ctx)
}

// Stats returns connection pool statistics.
// Useful for monitoring and debugging connection issues.
func (db *Database) Stats() *pgxpool.Stat {
	return db.Pool.Stat()
}

// BeginTx starts a transaction.
// Use this for operations that need to be atomic
func (db *Database) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return db.Pool.Begin(ctx)
}

// QueryTimeout is the default timeout for database queries.
// Individual queries can override this with their own context timeout.
const QueryTimeout = 10 * time.Second

// withTimeout creates a context with the default query timeout.
// This prevents queries from hanging indefinitely.
func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, QueryTimeout)
}

// type PostgresConfig struct {
// 	Host     string
// 	Port     string
// 	User     string
// 	Password string
// 	Database string
// 	SSLMode  string
// }

// func (cfg *PostgresConfig) String() string {
// 	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s", cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode)
// }

// // Open will open a SQL connection with the provided
// // Postgres database. Callers of Open need to ensure
// // that the connection is eventually closed via the
// // db.Close() method
// func Open(config PostgresConfig) (*sql.DB, error) {
// 	db, err := sql.Open("pgx", config.String())
// 	if err != nil {
// 		return nil, fmt.Errorf("open: %w", err)
// 	}
// 	return db, nil
// }

func Migrate(db *sql.DB, dir string) error {
	err := goose.SetDialect("postgres")
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	err = goose.Up(db, dir)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

func MigrateFS(db *sql.DB, migrationFS fs.FS, dir string) error {
	if dir == "" {
		dir = "."
	}
	goose.SetBaseFS(migrationFS)
	defer func() {
		goose.SetBaseFS(nil) //reads files from os file system if no embedded system is detected
	}()
	return Migrate(db, dir)
}
