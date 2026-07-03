package db

import (
	"context"
	"fmt"
	"time"

	"github.com/Traliaa/vpn-manager/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// ConnectParams для подключения к БД
type ConnectParams struct {
	fx.In

	Config    *config.Config
	Logger    *zap.Logger
	Lifecycle fx.Lifecycle
}

// ConnectResult подключения к БД
type ConnectResult struct {
	fx.Out

	Pool    *pgxpool.Pool
	Queries *Queries
}

func Connect(p ConnectParams) (ConnectResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, p.Config.Database.DSN)
	if err != nil {
		return ConnectResult{}, fmt.Errorf("connect to database: %w", err)
	}

	pool.Config().MaxConns = int32(p.Config.Database.MaxOpenConns)
	pool.Config().MinConns = int32(p.Config.Database.MaxIdleConns)

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return ConnectResult{}, fmt.Errorf("ping database: %w", err)
	}

	p.Lifecycle.Append(fx.Hook{
		OnStop: func(context.Context) error {
			p.Logger.Info("closing database pool")
			pool.Close()
			return nil
		},
	})

	return ConnectResult{
		Pool:    pool,
		Queries: New(pool),
	}, nil
}

var Module = fx.Module("database",
	fx.Provide(Connect),
)
