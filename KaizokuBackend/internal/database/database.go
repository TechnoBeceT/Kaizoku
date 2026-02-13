package database

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/config"
	"github.com/technobecet/kaizoku-go/internal/ent"
)

const (
	maxRetries   = 10
	initialDelay = 2 * time.Second
	maxDelay     = 30 * time.Second
)

// Open creates a new Ent client connected to PostgreSQL.
// It retries the initial connection to handle DNS/network delays in Docker.
func Open(cfg config.DatabaseConfig) (*ent.Client, error) {
	dsn := cfg.DSN()
	log.Info().
		Str("host", cfg.Host).
		Int("port", cfg.Port).
		Str("dbname", cfg.DBName).
		Msg("connecting to database")

	drv, err := sql.Open(dialect.Postgres, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// sql.Open is lazy — ping to verify the connection is reachable.
	// Retry with backoff for Docker environments where DNS may not be
	// immediately available.
	delay := initialDelay
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := drv.DB().Ping(); err == nil {
			return ent.NewClient(ent.Driver(drv)), nil
		} else if attempt == maxRetries {
			return nil, fmt.Errorf("database unreachable after %d attempts: %w", maxRetries, err)
		} else {
			log.Warn().Err(err).Int("attempt", attempt).Dur("retry_in", delay).Msg("database not ready, retrying")
			time.Sleep(delay)
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}

	// unreachable, but satisfies the compiler
	return ent.NewClient(ent.Driver(drv)), nil
}

// Migrate runs auto-migration on the database schema.
func Migrate(ctx context.Context, client *ent.Client) error {
	log.Info().Msg("running database migrations")
	if err := client.Schema.Create(ctx); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	log.Info().Msg("database migrations complete")
	return nil
}
