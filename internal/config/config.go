// Package config loads lazydbx settings with precedence:
// defaults < $XDG_CONFIG_HOME/lazydbx/config.yaml < env (LAZYDBX_*) < flags.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config is the fully resolved application configuration.
type Config struct {
	Profile  string    `koanf:"profile"`
	ReadOnly bool      `koanf:"read_only"`
	LogLevel string    `koanf:"log_level"`
	Refresh  Refresh   `koanf:"refresh"`
	SQL      SQLConfig `koanf:"sql"`
	// Skins maps profile-name globs to accent color names, k9s-style
	// (e.g. "PROD-*": red). Matching is case-insensitive.
	Skins map[string]string `koanf:"skins"`
}

// Refresh controls poll cadence (seconds) for resource views.
type Refresh struct {
	IntervalSeconds int `koanf:"interval_seconds"`
}

// SQLConfig controls statement execution.
type SQLConfig struct {
	// WarehouseID overrides warehouse auto-selection (serverless-first).
	WarehouseID string `koanf:"warehouse_id"`
	RowLimit    int    `koanf:"row_limit"`
}

// Flags carries command-line overrides; zero values mean "not set".
type Flags struct {
	Profile    string
	ReadOnly   bool
	LogLevel   string
	ConfigFile string
}

func defaults() map[string]any {
	return map[string]any{
		"profile":                  "",
		"read_only":                false,
		"log_level":                "info",
		"refresh.interval_seconds": 5,
		"sql.warehouse_id":         "",
		"sql.row_limit":            10000,
	}
}

// Path returns the default config file location.
func Path() string {
	return filepath.Join(xdg.ConfigHome, "lazydbx", "config.yaml")
}

// Load resolves configuration from all sources.
func Load(flags Flags) (Config, error) {
	k := koanf.New(".")

	for key, val := range defaults() {
		if err := k.Set(key, val); err != nil {
			return Config{}, fmt.Errorf("setting default %s: %w", key, err)
		}
	}

	path := flags.ConfigFile
	explicit := path != ""
	if !explicit {
		path = Path()
	}
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		// A missing default config file is fine; an explicit one must exist.
		if explicit || !errors.Is(err, fs.ErrNotExist) {
			return Config{}, fmt.Errorf("loading config file %s: %w", path, err)
		}
	}

	// LAZYDBX_SQL__WAREHOUSE_ID → sql.warehouse_id ("__" nests, "_" stays).
	err := k.Load(env.Provider("LAZYDBX_", ".", func(s string) string {
		s = strings.TrimPrefix(s, "LAZYDBX_")
		return strings.ReplaceAll(strings.ToLower(s), "__", ".")
	}), nil)
	if err != nil {
		return Config{}, fmt.Errorf("loading env config: %w", err)
	}

	if flags.Profile != "" {
		_ = k.Set("profile", flags.Profile)
	}
	if flags.ReadOnly {
		_ = k.Set("read_only", true)
	}
	if flags.LogLevel != "" {
		_ = k.Set("log_level", flags.LogLevel)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshaling config: %w", err)
	}
	return cfg, nil
}
