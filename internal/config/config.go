package config

import (
	"errors"
	"fmt"
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
	CORS     CORSConfig     `yaml:"cors"`
}

type AppConfig struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
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
	JWTSecret       string        `yaml:"jwt-secret"`
	AccessTokenTTL  time.Duration `yaml:"access-token-ttl"`
	RefreshTokenTTL time.Duration `yaml:"refresh-token-ttl"`
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
		return Config{}, fmt.Errorf(
			"read config file: %w",
			err,
		)
	}

	// Environment variables are deliberately supported
	// inside YAML using ${VARIABLE} syntax.
	data = []byte(
		os.ExpandEnv(string(data)),
	)

	var cfg Config

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf(
			"parse config file: %w",
			err,
		)
	}

	if err := validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validate(cfg Config) error {
	if strings.TrimSpace(cfg.App.Name) == "" {
		return errors.New(
			"app.name is required",
		)
	}

	if strings.TrimSpace(cfg.App.Environment) == "" {
		return errors.New(
			"app.environment is required",
		)
	}

	if strings.TrimSpace(cfg.HTTP.Host) == "" {
		return errors.New(
			"http.host is required",
		)
	}

	if strings.TrimSpace(cfg.HTTP.Port) == "" {
		return errors.New(
			"http.port is required",
		)
	}

	if strings.TrimSpace(cfg.Database.URL) == "" {
		return errors.New(
			"database.url is required",
		)
	}

	if cfg.Database.MaxConns < 1 {
		return errors.New(
			"database.max-conns must be greater than zero",
		)
	}

	if cfg.Database.MinConns < 0 {
		return errors.New(
			"database.min-conns cannot be negative",
		)
	}

	if cfg.Database.MinConns > cfg.Database.MaxConns {
		return errors.New(
			"database.min-conns cannot exceed database.max-conns",
		)
	}

	if len(cfg.Security.JWTSecret) < 64 {
		return errors.New(
			"security.jwt-secret must contain at least 64 characters",
		)
	}

	if cfg.Security.AccessTokenTTL <= 0 {
		return errors.New(
			"security.access-token-ttl must be greater than zero",
		)
	}

	if cfg.Security.RefreshTokenTTL <= 0 {
		return errors.New(
			"security.refresh-token-ttl must be greater than zero",
		)
	}

	return nil
}
