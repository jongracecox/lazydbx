package dbx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeCfg(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".databrickscfg")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoadProfiles(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
		want func(t *testing.T, profiles []Profile)
	}{
		{
			name: "workspace and account profiles",
			cfg: `[DEFAULT]
host = https://dbc-111.cloud.databricks.com
token = dapi-secret

[dev]
host = https://adb-222.azuredatabricks.net/
auth_type = databricks-cli
account_id = abc-123

[acct]
host = https://accounts.cloud.databricks.com
account_id = abc-123
`,
			want: func(t *testing.T, profiles []Profile) {
				require.Len(t, profiles, 3)
				assert.Equal(t, "DEFAULT", profiles[0].Name, "DEFAULT sorts first")
				assert.False(t, profiles[0].IsAccount())

				acct := profiles[1]
				assert.Equal(t, "acct", acct.Name)
				assert.True(t, acct.IsAccount())
				assert.Equal(t, "abc-123", acct.AccountID)

				dev := profiles[2]
				assert.Equal(t, "https://adb-222.azuredatabricks.net", dev.Host, "trailing slash stripped")
				assert.Equal(t, "adb-222.azuredatabricks.net", dev.ShortHost())
				assert.Equal(t, "databricks-cli", dev.AuthType)
				assert.False(t, dev.IsAccount(), "workspace host with account_id key is still a workspace profile")
			},
		},
		{
			name: "skips settings sections and hostless profiles",
			cfg: `[__settings__]
usage_tracking = true

[broken]
token = dapi-orphan

[real]
host = https://dbc-333.cloud.databricks.com
`,
			want: func(t *testing.T, profiles []Profile) {
				require.Len(t, profiles, 1)
				assert.Equal(t, "real", profiles[0].Name)
			},
		},
		{
			name: "no DEFAULT section defined",
			cfg: `[only]
host = https://dbc-444.cloud.databricks.com
`,
			want: func(t *testing.T, profiles []Profile) {
				require.Len(t, profiles, 1)
				assert.Equal(t, "only", profiles[0].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profiles, err := LoadProfiles(writeCfg(t, tt.cfg))
			require.NoError(t, err)
			tt.want(t, profiles)
		})
	}
}

func TestLoadProfilesMissingFile(t *testing.T) {
	_, err := LoadProfiles(filepath.Join(t.TempDir(), "nope"))
	assert.Error(t, err)
}

func TestConfigPathEnvOverride(t *testing.T) {
	t.Setenv("DATABRICKS_CONFIG_FILE", "/tmp/custom.cfg")
	path, err := ConfigPath()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/custom.cfg", path)
}
