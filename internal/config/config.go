// Package config loads lazydbx settings with precedence:
// defaults < $XDG_CONFIG_HOME/lazydbx/config.yaml < env (LAZYDBX_*) < flags.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
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
	// Skins maps profile-name globs to a highlight color name, k9s-style
	// (e.g. "PROD-*": red). Matching is case-insensitive. The color tints only
	// the profile name + host in the header; the rest of the UI keeps the
	// default accent. The in-app color picker (`c` on the profile screen)
	// writes exact-name entries here via SaveSkin.
	Skins map[string]string `koanf:"skins"`

	// path is the file config was loaded from; SaveSkin writes back here. Not
	// a koanf field — it is resolved by Load, not parsed.
	path string
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
	cfg.path = path
	return cfg, nil
}

// SetSkin updates the in-memory highlight color for an exact profile name. An
// empty color clears the entry. Persist the change with SaveSkin.
func (c *Config) SetSkin(profile, color string) {
	if color == "" {
		delete(c.Skins, profile)
		return
	}
	if c.Skins == nil {
		c.Skins = map[string]string{}
	}
	c.Skins[profile] = color
}

// SaveSkin persists a single profile→color highlight to the config file,
// touching only the `skins` map so other settings (and unknown keys) survive —
// though YAML comments are not preserved by the round-trip. An empty color
// removes the entry. The file and its parent directory are created if absent.
func (c Config) SaveSkin(profile, color string) error {
	path := c.path
	if path == "" {
		path = Path()
	}
	parser := yaml.Parser()

	root := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if root, err = parser.Unmarshal(data); err != nil {
			return fmt.Errorf("parsing config %s: %w", path, err)
		}
		if root == nil {
			root = map[string]any{}
		}
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("reading config %s: %w", path, err)
	}

	skins, _ := root["skins"].(map[string]any)
	if skins == nil {
		skins = map[string]any{}
	}
	if color == "" {
		delete(skins, profile)
	} else {
		skins[profile] = color
	}
	if len(skins) == 0 {
		delete(root, "skins")
	} else {
		root["skins"] = skins
	}

	out, err := parser.Marshal(root)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("writing config %s: %w", path, err)
	}
	return nil
}
