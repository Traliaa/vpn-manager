package app

import (
	"context"
	"os/exec"
	"time"

	"github.com/Traliaa/vpn-manager/internal/api"
	"github.com/Traliaa/vpn-manager/internal/api/logs"
	"github.com/Traliaa/vpn-manager/internal/bot"
	"github.com/Traliaa/vpn-manager/internal/config"
	"github.com/Traliaa/vpn-manager/internal/db"
	"github.com/Traliaa/vpn-manager/internal/routing"
	"github.com/Traliaa/vpn-manager/internal/vpn"
	"github.com/Traliaa/vpn-manager/internal/vpn/health"
	"github.com/Traliaa/vpn-manager/internal/vpn/resolver"
	"github.com/Traliaa/vpn-manager/internal/vpn/service"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var Module = fx.Module("app",
	config.Module,
	db.MigrateModule,
	db.Module,
	vpn.Module,
	api.Module,

	fx.Provide(
		service.NewService,
		health.NewChecker,
		resolver.NewService,
		routing.NewActivator,
		newBot,
		func() *logs.Buffer { return logs.NewBuffer(500) },
	),

	fx.Invoke(
		startHealthChecker,
		startResolver,
		activateDefaultProfile,
		startBot,
	),

	fx.Provide(func(buf *logs.Buffer) (*zap.Logger, error) {
		logger := zap.New(zapcore.NewTee(
			zapcore.NewCore(
				zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
				zapcore.AddSync(os.Stdout),
				zap.NewAtomicLevelAt(zap.InfoLevel),
			),
			buf.ZapCore(),
		))
		return logger, nil
	}),
	fx.Decorate(func(log *zap.Logger) *zap.Logger {
		return log.With(zap.String("service", "vpn-manager"))
	}),
)

func startHealthChecker(lc fx.Lifecycle, checker *health.Checker, cfg *config.Config, logger *zap.Logger) {
	interval := cfg.VPN.HealthInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	logger.Info("configuring health checker", zap.Duration("interval", interval))

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return checker.Start(ctx, interval)
		},
		OnStop: func(ctx context.Context) error {
			checker.Stop()
			return nil
		},
	})
}

func startResolver(lc fx.Lifecycle, svc *resolver.Service, cfg *config.Config, logger *zap.Logger) {
	interval := cfg.VPN.Resolver.Interval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	staleTimeout := cfg.VPN.Resolver.StaleTimeout
	if staleTimeout <= 0 {
		staleTimeout = 30 * time.Minute
	}

	logger.Info("configuring domain resolver",
		zap.Duration("interval", interval),
		zap.Duration("stale_timeout", staleTimeout),
	)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return svc.Start(ctx, interval, staleTimeout)
		},
		OnStop: func(ctx context.Context) error {
			svc.Stop()
			return nil
		},
	})
}

func activateDefaultProfile(lc fx.Lifecycle, activator *routing.Activator, queries *db.Queries, svc *service.Service, logger *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Загружаем VPN-модули ядра (если не загружены)
			loadKernelModules(logger)

			// Синхронизируем провайдеров из БД в менеджер
			if err := svc.SyncProviders(ctx); err != nil {
				logger.Warn("failed to sync providers on startup",
					zap.Error(err),
				)
			}

			// Пробуем активировать профиль по умолчанию (если есть)
			profile, err := queries.GetDefaultProfile(ctx)
			if err != nil {
				logger.Info("no default profile found, routing stays direct")
				return nil
			}

			logger.Info("activating default profile on startup",
				zap.String("profile", profile.Name),
			)
			if err := activator.Activate(ctx, profile.ID); err != nil {
				logger.Warn("failed to activate default profile",
					zap.String("profile", profile.Name),
					zap.Error(err),
				)
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return nil
		},
	})
}

// loadKernelModules пытается загрузить VPN-модули ядра (wireguard, amneziawg).
func loadKernelModules(logger *zap.Logger) {
	modules := []string{"wireguard", "amneziawg"}
	for _, mod := range modules {
		if err := exec.Command("modprobe", mod).Run(); err != nil {
			logger.Debug("kernel module not available",
				zap.String("module", mod),
				zap.Error(err),
			)
		} else {
			logger.Info("kernel module loaded",
				zap.String("module", mod),
			)
		}
	}
}

// newBot creates a Telegram bot (no-op if token is empty).
func newBot(cfg *config.Config, manager *vpn.Manager, activator *routing.Activator,
	svc *service.Service, queries *db.Queries, logger *zap.Logger) *bot.Bot {
	return bot.New(cfg.Telegram.Token, manager, activator, svc, queries, logger)
}

// startBot starts the Telegram bot polling via fx.Lifecycle.
func startBot(lc fx.Lifecycle, b *bot.Bot, logger *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return b.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			b.Stop()
			return nil
		},
	})
}
