package vpn

import (
	"github.com/Traliaa/vpn-manager/internal/config"
	"github.com/Traliaa/vpn-manager/internal/vpn/singbox"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var Module = fx.Module("vpn",
	fx.Provide(
		NewManager,
		NewSingBoxController,
	),
)

// ProviderParams — общие зависимости для конструкторов провайдеров.
type ProviderParams struct {
	fx.In

	Manager *Manager
	SingBox *singbox.Controller
	Logger  *zap.Logger
}

// NewSingBoxController создаёт и настраивает SingBox Controller из конфига.
func NewSingBoxController(cfg *config.Config, logger *zap.Logger) (*singbox.Controller, error) {
	return singbox.NewController(singbox.Config{
		ConfigPath:  cfg.VPN.SingBox.ConfigPath,
		BinaryPath:  cfg.VPN.SingBox.BinaryPath,
		ServiceName: cfg.VPN.SingBox.ServiceName,
		APIBaseURL:  cfg.VPN.SingBox.APIBaseURL,
		APIEnabled:  cfg.VPN.SingBox.APIEnabled,
	}, logger)
}
