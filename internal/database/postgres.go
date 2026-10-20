package database

import (
	"context"
	"time"

	"backend/internal/bootstrap"
	"backend/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPostgres(
	ctx context.Context,
	cfg config.Config,
) (*pgxpool.Pool, error) {

	poolConfig, err := pgxpool.ParseConfig(
		cfg.Database.URL,
	)

	if err != nil {
		return nil, err
	}

	poolConfig.MaxConns =
		cfg.Database.MaxConns

	poolConfig.MinConns =
		cfg.Database.MinConns

	poolConfig.MaxConnLifetime =
		cfg.Database.MaxConnLifetime

	poolConfig.MaxConnIdleTime =
		cfg.Database.MaxConnIdleTime

	poolConfig.HealthCheckPeriod =
		30 * time.Second

	pool, err := pgxpool.NewWithConfig(
		ctx,
		poolConfig,
	)

	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	if err := bootstrap.EnsureRequiredData(ctx, pool, nil); err != nil {
		pool.Close()
		return nil, err
	}

	if err := bootstrap.EnsureDefaultAdmin(ctx, pool, nil); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
