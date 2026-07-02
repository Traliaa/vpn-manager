package config

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var Module = fx.Module("config",
	fx.Provide(Load),
	fx.Decorate(func(cfg *Config, logger *zap.Logger) *Config {
		logger.Info("config loaded",
			zap.String("server", cfg.Server.Host+":"+cfg.Server.Port),
			zap.String("db_dsn", cfg.Database.DSN),
			zap.String("log_level", cfg.Log.Level),
		)
		return cfg
	}),
)
