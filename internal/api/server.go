package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Traliaa/vpn-manager/internal/api/logs"
	"github.com/Traliaa/vpn-manager/internal/config"
	"github.com/Traliaa/vpn-manager/internal/vpn/gateway"
	"github.com/go-chi/chi/v5"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type ServerParams struct {
	fx.In

	Router    *chi.Mux
	Config    *config.Config
	Logger    *zap.Logger
	Lifecycle fx.Lifecycle
}

func RunServer(p ServerParams) {
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%s", p.Config.Server.Host, p.Config.Server.Port),
		Handler:      p.Router,
		ReadTimeout:  p.Config.Server.ReadTimeout,
		WriteTimeout: p.Config.Server.WriteTimeout,
	}

	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				p.Logger.Info("server listening", zap.String("addr", srv.Addr))
				if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					p.Logger.Fatal("server error", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			p.Logger.Info("shutting down server...")
			shutdownCtx, cancel := context.WithTimeout(ctx, p.Config.Server.ShutdownTimeout)
			defer cancel()
			return srv.Shutdown(shutdownCtx)
		},
	})
}

var Module = fx.Module("api",
	fx.Provide(NewHandlers, NewVpnHandlers, NewRoutingHandlers, NewHaHandlers, NewGatewayHandlers, gateway.NewManager, NewRouter, logs.NewHTTPHandler),
	fx.Invoke(RunServer),
)
