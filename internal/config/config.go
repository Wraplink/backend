package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App      AppConfig      `yaml:"app"`
	HTTP     HTTPConfig     `yaml:"http"`
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Security SecurityConfig `yaml:"security"`
	Payments PaymentsConfig `yaml:"payments"`
	Network  NetworkConfig  `yaml:"network"`
	CORS     CORSConfig     `yaml:"cors"`
}

type AppConfig struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
	FrontendURL string `yaml:"frontend-url"`
}

type HTTPConfig struct {
	Host            string        `yaml:"host"`
	Port            string        `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read-timeout"`
	WriteTimeout    time.Duration `yaml:"write-timeout"`
	IdleTimeout     time.Duration `yaml:"idle-timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown-timeout"`
}

type DatabaseConfig struct {
	URL             string        `yaml:"url"`
	MaxConns        int32         `yaml:"max-conns"`
	MinConns        int32         `yaml:"min-conns"`
	MaxConnLifetime time.Duration `yaml:"max-conn-lifetime"`
	MaxConnIdleTime time.Duration `yaml:"max-conn-idle-time"`
}

type SecurityConfig struct {
	JWTSecret           string        `yaml:"jwt-secret"`
	AccessTokenTTL      time.Duration `yaml:"access-token-ttl"`
	RefreshTokenTTL     time.Duration `yaml:"refresh-token-ttl"`
	SecureCookies       bool          `yaml:"secure-cookies"`
	BootstrapAdminEmail string `yaml:"-"`
	BootstrapAdminPassword string `yaml:"-"`
	RefreshCookieDomain string        `yaml:"refresh-cookie-domain"`
}

type PaymentsConfig struct {
	DevSettlementKey string `yaml:"dev-settlement-key"`
}

type NetworkConfig struct {
	UsageIngestKey          string             `yaml:"usage-ingest-key"`
	ControlSyncKey          string             `yaml:"control-sync-key"`
	DNSControlURL           string             `yaml:"dns-control-url"`
	DNSControlToken         string             `yaml:"dns-control-token"`
	ControlNodeOfflineAfter time.Duration      `yaml:"control-node-offline-after"`
	DNSServers              []string           `yaml:"dns-servers"`
	Proxy                   NetworkProxyConfig `yaml:"proxy"`
}

type NetworkProxyConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Protocol string `yaml:"protocol"`
}

type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed-origins"`
}

type ServerConfig struct {
	TrustedProxies []string `yaml:"trusted-proxies"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	data = []byte(os.ExpandEnv(string(data)))

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config file: %w", err)
	}
	cfg.Security.BootstrapAdminEmail = strings.ToLower(strings.TrimSpace(os.Getenv("WRAPLINK_BOOTSTRAP_ADMIN_EMAIL")))
	cfg.Security.BootstrapAdminPassword = os.Getenv("WRAPLINK_BOOTSTRAP_ADMIN_PASSWORD")
	if cfg.Network.ControlNodeOfflineAfter == 0 {
		cfg.Network.ControlNodeOfflineAfter = 45 * time.Second
	}
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validate(cfg Config) error {
	if (cfg.Security.BootstrapAdminEmail == "") != (cfg.Security.BootstrapAdminPassword == "") {
		return errors.New("bootstrap administrator email and password must be configured together")
	}
	if cfg.Security.BootstrapAdminEmail != "" && (!strings.Contains(cfg.Security.BootstrapAdminEmail, "@") || len(cfg.Security.BootstrapAdminPassword) < 16) {
		return errors.New("bootstrap administrator requires a valid email and password of at least 16 characters")
	}

	if strings.TrimSpace(cfg.App.FrontendURL) == "" {
		return errors.New("app.frontend-url is required")
	}
	if strings.TrimSpace(cfg.App.Name) == "" {
		return errors.New("app.name is required")
	}
	if strings.TrimSpace(cfg.App.Environment) == "" {
		return errors.New("app.environment is required")
	}
	if strings.TrimSpace(cfg.HTTP.Host) == "" {
		return errors.New("http.host is required")
	}
	if strings.TrimSpace(cfg.HTTP.Port) == "" {
		return errors.New("http.port is required")
	}
	if strings.TrimSpace(cfg.Database.URL) == "" {
		return errors.New("database.url is required")
	}
	if cfg.Database.MaxConns < 1 {
		return errors.New("database.max-conns must be greater than zero")
	}
	if cfg.Database.MinConns < 0 {
		return errors.New("database.min-conns cannot be negative")
	}
	if cfg.Database.MinConns > cfg.Database.MaxConns {
		return errors.New("database.min-conns cannot exceed database.max-conns")
	}
	if len(cfg.Security.JWTSecret) < 64 {
		return errors.New("security.jwt-secret must contain at least 64 characters")
	}
	if cfg.Security.AccessTokenTTL <= 0 {
		return errors.New("security.access-token-ttl must be greater than zero")
	}
	if cfg.Security.RefreshTokenTTL <= 0 {
		return errors.New("security.refresh-token-ttl must be greater than zero")
	}
	if cfg.Security.AccessTokenTTL >= cfg.Security.RefreshTokenTTL {
		return errors.New("security.access-token-ttl must be shorter than refresh-token-ttl")
	}

	devSettlementKey := strings.TrimSpace(cfg.Payments.DevSettlementKey)
	if devSettlementKey != "" && len(devSettlementKey) < 32 {
		return errors.New("payments.dev-settlement-key must contain at least 32 characters when configured")
	}

	usageIngestKey := strings.TrimSpace(cfg.Network.UsageIngestKey)
	if usageIngestKey != "" && len(usageIngestKey) < 32 {
		return errors.New("network.usage-ingest-key must contain at least 32 characters when configured")
	}

	controlSyncKey := strings.TrimSpace(cfg.Network.ControlSyncKey)
	if controlSyncKey != "" && len(controlSyncKey) < 32 {
		return errors.New("network.control-sync-key must contain at least 32 characters when configured")
	}
	if (cfg.Network.DNSControlURL == "") != (cfg.Network.DNSControlToken == "") {
		return errors.New("network.dns-control-url and network.dns-control-token must be configured together")
	}
	if cfg.Network.DNSControlURL != "" && !strings.HasPrefix(cfg.Network.DNSControlURL, "http://") && !strings.HasPrefix(cfg.Network.DNSControlURL, "https://") {
		return errors.New("network.dns-control-url must use http or https")
	}
	if cfg.Network.ControlNodeOfflineAfter <= 0 {
		return errors.New("network.control-node-offline-after must be greater than zero")
	}

	for _, server := range cfg.Network.DNSServers {
		server = strings.TrimSpace(server)
		if server == "" || net.ParseIP(server) == nil {
			return errors.New("network.dns-servers must contain valid IP addresses")
		}
	}

	proxyHost := strings.TrimSpace(cfg.Network.Proxy.Host)
	proxyProtocol := strings.TrimSpace(cfg.Network.Proxy.Protocol)
	if proxyHost == "" {
		if cfg.Network.Proxy.Port != 0 || proxyProtocol != "" {
			return errors.New("network.proxy.host is required when proxy port or protocol is configured")
		}
	} else {
		if cfg.Network.Proxy.Port < 1 || cfg.Network.Proxy.Port > 65535 {
			return errors.New("network.proxy.port must be between 1 and 65535")
		}
		if proxyProtocol == "" {
			return errors.New("network.proxy.protocol is required when proxy host is configured")
		}
	}

	if strings.EqualFold(cfg.App.Environment, "production") {
		if !cfg.Security.SecureCookies {
			return errors.New("security.secure-cookies must be enabled in production")
		}
		if devSettlementKey != "" {
			return errors.New("payments.dev-settlement-key must be empty in production")
		}
	}
	return nil
}
