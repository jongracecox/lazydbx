package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		env   map[string]string
		flags Flags
		want  func(t *testing.T, cfg Config)
	}{
		{
			name: "defaults",
			want: func(t *testing.T, cfg Config) {
				assert.Equal(t, "info", cfg.LogLevel)
				assert.Equal(t, 5, cfg.Refresh.IntervalSeconds)
				assert.Equal(t, 10000, cfg.SQL.RowLimit)
				assert.False(t, cfg.ReadOnly)
			},
		},
		{
			name: "file overrides defaults",
			yaml: "log_level: debug\nsql:\n  warehouse_id: wh-123\n",
			want: func(t *testing.T, cfg Config) {
				assert.Equal(t, "debug", cfg.LogLevel)
				assert.Equal(t, "wh-123", cfg.SQL.WarehouseID)
			},
		},
		{
			name: "env overrides file",
			yaml: "log_level: debug\n",
			env:  map[string]string{"LAZYDBX_LOG_LEVEL": "warn", "LAZYDBX_SQL__WAREHOUSE_ID": "wh-env"},
			want: func(t *testing.T, cfg Config) {
				assert.Equal(t, "warn", cfg.LogLevel)
				assert.Equal(t, "wh-env", cfg.SQL.WarehouseID)
			},
		},
		{
			name:  "flags override everything",
			yaml:  "profile: from-file\nlog_level: debug\n",
			env:   map[string]string{"LAZYDBX_LOG_LEVEL": "warn"},
			flags: Flags{Profile: "from-flag", LogLevel: "error", ReadOnly: true},
			want: func(t *testing.T, cfg Config) {
				assert.Equal(t, "from-flag", cfg.Profile)
				assert.Equal(t, "error", cfg.LogLevel)
				assert.True(t, cfg.ReadOnly)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			flags := tt.flags
			if tt.yaml != "" {
				flags.ConfigFile = writeConfig(t, tt.yaml)
			} else {
				// Point at a nonexistent file so a real ~/.config is not picked up.
				flags.ConfigFile = ""
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			}
			cfg, err := Load(flags)
			require.NoError(t, err)
			tt.want(t, cfg)
		})
	}
}

func TestLoadExplicitMissingFile(t *testing.T) {
	_, err := Load(Flags{ConfigFile: filepath.Join(t.TempDir(), "nope.yaml")})
	assert.Error(t, err)
}
