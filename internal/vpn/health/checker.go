// Package health provides periodic health-checking of VPN providers.
package health

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Traliaa/vpn-manager/internal/db"
	"github.com/Traliaa/vpn-manager/internal/vpn"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// Checker выполняет фоновую проверку здоровья VPN-провайдеров.
type Checker struct {
	manager  *vpn.Manager
	q        *db.Queries
	logger   *zap.Logger
	interval time.Duration

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
}

// NewChecker создаёт health-check сервис.
func NewChecker(manager *vpn.Manager, q *db.Queries, logger *zap.Logger) *Checker {
	return &Checker{
		manager: manager,
		q:       q,
		logger:  logger.Named("health-checker"),
		stopCh:  make(chan struct{}),
	}
}

// Start запускает фоновую проверку с заданным интервалом.
func (c *Checker) Start(ctx context.Context, interval time.Duration) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return nil
	}
	c.interval = interval
	c.running = true
	c.mu.Unlock()

	c.logger.Info("health checker started", zap.Duration("interval", c.interval))

	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		// Сразу выполняем первую проверку
		c.runChecks(ctx)

		for {
			select {
			case <-ticker.C:
				c.runChecks(ctx)
			case <-c.stopCh:
				c.logger.Info("health checker stopped")
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Stop останавливает фоновую проверку.
func (c *Checker) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		close(c.stopCh)
		c.running = false
	}
}

// runChecks выполняет проверку всех зарегистрированных провайдеров.
func (c *Checker) runChecks(ctx context.Context) {
	providers := c.manager.List()
	if len(providers) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, p := range providers {
		wg.Add(1)
		go func(provider vpn.Provider) {
			defer wg.Done()
			c.checkProvider(ctx, provider)
		}(p)
	}
	wg.Wait()
}

// checkProvider проверяет одного провайдера и пишет результат в БД.
func (c *Checker) checkProvider(ctx context.Context, provider vpn.Provider) {
	start := time.Now()
	err := provider.HealthCheck(ctx)
	duration := time.Since(start)

	// Определяем статус
	success := err == nil
	status := db.CheckStatusUp
	errorMsg := ""
	if !success {
		status = db.CheckStatusDown
		errorMsg = err.Error()
	}

	// Получаем providerId из БД по имени
	providerID, err2 := c.findProviderID(ctx, provider.Name())
	if err2 != nil {
		c.logger.Warn("cannot find provider in DB",
			zap.String("name", provider.Name()),
			zap.Error(err2),
		)
		return
	}

	// Пишем результат
	if _, err := c.q.CreateHealthCheck(ctx, db.CreateHealthCheckParams{
		ProviderID: providerID,
		Status:     status,
		LatencyMs:  pgtype.Int4{Int32: int32(duration.Milliseconds()), Valid: true},
		Error:      pgtype.Text{String: errorMsg, Valid: errorMsg != ""},
	}); err != nil {
		c.logger.Warn("failed to record health check",
			zap.String("provider", provider.Name()),
			zap.Error(err),
		)
	}

	c.logger.Debug("health check completed",
		zap.String("provider", provider.Name()),
		zap.Bool("success", success),
		zap.Duration("duration", duration),
	)
}

// findProviderID ищет ID провайдера в БД по имени.
func (c *Checker) findProviderID(ctx context.Context, name string) (pgtype.UUID, error) {
	providers, err := c.q.ListProviders(ctx)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("list providers: %w", err)
	}
	for _, p := range providers {
		if p.Name == name {
			return p.ID, nil
		}
	}
	return pgtype.UUID{}, fmt.Errorf("provider %q not found in database", name)
}
