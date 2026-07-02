// Package resolver provides periodic DNS resolution of domain-type routing rules
// and stores resolved IP addresses in the resolved_routes table.
package resolver

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/Traliaa/vpn-manager/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// Service выполняет фоновый DNS-резолв доменных правил маршрутизации.
type Service struct {
	q       *db.Queries
	logger  *zap.Logger
	timeout time.Duration

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
}

// NewService создаёт сервис резолвинга доменов.
func NewService(q *db.Queries, logger *zap.Logger) *Service {
	return &Service{
		q:       q,
		logger:  logger.Named("domain-resolver"),
		timeout: 5 * time.Second,
		stopCh:  make(chan struct{}),
	}
}

// Start запускает циклический резолвинг доменов с заданным интервалом.
func (s *Service) Start(ctx context.Context, interval, staleTimeout time.Duration) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	s.logger.Info("domain resolver started",
		zap.Duration("interval", interval),
		zap.Duration("stale_timeout", staleTimeout),
	)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Сразу выполняем первый проход
		s.resolveAll(ctx, staleTimeout)

		for {
			select {
			case <-ticker.C:
				s.resolveAll(ctx, staleTimeout)
			case <-s.stopCh:
				s.logger.Info("domain resolver stopped")
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Stop останавливает фоновый резолвинг.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		close(s.stopCh)
		s.running = false
	}
}

// resolveAll выполняет полный цикл: загружает правила, резолвит домены, чистит stale.
func (s *Service) resolveAll(ctx context.Context, staleTimeout time.Duration) {
	rules, err := s.q.ListEnabledDomainRules(ctx)
	if err != nil {
		s.logger.Error("list enabled domain rules", zap.Error(err))
		return
	}

	if len(rules) == 0 {
		s.cleanStale(ctx, staleTimeout)
		return
	}

	var wg sync.WaitGroup
	for _, rule := range rules {
		wg.Add(1)
		go func(r db.ListEnabledDomainRulesRow) {
			defer wg.Done()
			s.resolveRule(ctx, r)
		}(rule)
	}
	wg.Wait()

	s.cleanStale(ctx, staleTimeout)
}

// resolveRule резолвит один домен и сохраняет IP-адреса.
func (s *Service) resolveRule(ctx context.Context, rule db.ListEnabledDomainRulesRow) {
	r := &net.Resolver{
		PreferGo: true,
	}

	// Пробуем A (IPv4) и AAAA (IPv6) записи
	ips, err := r.LookupNetIP(ctx, "ip", rule.Value)
	if err != nil {
		s.logger.Debug("domain resolution failed",
			zap.String("domain", rule.Value),
			zap.Error(err),
		)
		return
	}

	if len(ips) == 0 {
		return
	}

	for _, ip := range ips {
		_, err := s.q.UpsertResolvedRoute(ctx, db.UpsertResolvedRouteParams{
			RuleID: rule.ID,
			Ip:     ip,
		})
		if err != nil {
			s.logger.Warn("failed to upsert resolved route",
				zap.String("domain", rule.Value),
				zap.Stringer("ip", ip),
				zap.Error(err),
			)
		}
	}

	s.logger.Debug("domain resolved",
		zap.String("domain", rule.Value),
		zap.Int("ips", len(ips)),
	)
}

// cleanStale удаляет записи, которые не обновлялись дольше staleTimeout.
func (s *Service) cleanStale(ctx context.Context, staleTimeout time.Duration) {
	interval := pgtype.Interval{
		Microseconds: staleTimeout.Microseconds(),
		Valid:        true,
	}

	if err := s.q.DeleteStaleResolvedRoutes(ctx, interval); err != nil {
		s.logger.Warn("failed to clean stale resolved routes", zap.Error(err))
	}
}
