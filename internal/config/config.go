package config

import (
	"os"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	VPN      VPNConfig
	Log      LogConfig
	Telegram TelegramConfig
}

type ServerConfig struct {
	Host            string
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

type DatabaseConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	MigrationsDir   string
}

type VPNConfig struct {
	BinDir         string
	HealthInterval time.Duration
	SingBox        SingBoxConfig
	Resolver       ResolverConfig
}

type SingBoxConfig struct {
	ConfigPath  string
	BinaryPath  string
	ServiceName string
	APIBaseURL  string
	APIEnabled  bool
}

type ResolverConfig struct {
	Interval     time.Duration
	Timeout      time.Duration
	StaleTimeout time.Duration
}

type LogConfig struct {
	Level string
}

type TelegramConfig struct {
	Token string
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            getEnv("SERVER_HOST", "0.0.0.0"),
			Port:            getEnv("SERVER_PORT", "8080"),
			ReadTimeout:     10 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 15 * time.Second,
		},
		Database: DatabaseConfig{
			DSN:             getEnv("DATABASE_DSN", "postgres://vpnmanager:vpnmanager@localhost:5432/vpnmanager?sslmode=disable"),
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
			MigrationsDir:   getEnv("MIGRATIONS_DIR", "migrations"),
		},
		VPN: VPNConfig{
			BinDir:         getEnv("VPN_BIN_DIR", "/usr/local/bin"),
			HealthInterval: 30 * time.Second,
			SingBox: SingBoxConfig{
				ConfigPath:  getEnv("SINGBOX_CONFIG_PATH", "/etc/sing-box/config.json"),
				BinaryPath:  getEnv("SINGBOX_BINARY_PATH", "sing-box"),
				ServiceName: getEnv("SINGBOX_SERVICE_NAME", "sing-box"),
				APIBaseURL:  getEnv("SINGBOX_API_URL", "http://127.0.0.1:9090"),
				APIEnabled:  getEnv("SINGBOX_API_ENABLED", "false") == "true",
			},
			Resolver: ResolverConfig{
				Interval:     5 * time.Minute,
				Timeout:      5 * time.Second,
				StaleTimeout: 30 * time.Minute,
			},
		},
		Log: LogConfig{
			Level: getEnv("LOG_LEVEL", "info"),
		},
		Telegram: TelegramConfig{
			Token: getEnv("TELEGRAM_BOT_TOKEN", ""),
		},
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
