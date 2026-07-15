// Package dbx is the only package that imports the Databricks SDK. It owns
// profile discovery (~/.databrickscfg), per-profile client construction, and
// the narrow DAO interfaces the rest of the app consumes.
package dbx

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/ini.v1"
)

// Profile is one entry in ~/.databrickscfg. The SDK offers no way to
// enumerate profiles, so we parse the INI ourselves — read-only.
type Profile struct {
	Name      string
	Host      string
	AccountID string
	AuthType  string
}

// IsAccount reports whether this profile targets the accounts API rather
// than a single workspace. Only the host decides: workspace profiles often
// carry an account_id key too, so its presence proves nothing.
func (p Profile) IsAccount() bool {
	return strings.HasPrefix(p.Host, "https://accounts.")
}

// ShortHost is the host without scheme, for compact header display.
func (p Profile) ShortHost() string {
	return strings.TrimPrefix(strings.TrimPrefix(p.Host, "https://"), "http://")
}

// ConfigPath resolves the databricks config file location, honoring
// DATABRICKS_CONFIG_FILE like the SDK does.
func ConfigPath() (string, error) {
	if p := os.Getenv("DATABRICKS_CONFIG_FILE"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".databrickscfg"), nil
}

// LoadProfiles parses the config file at path and returns its profiles,
// sorted by name with DEFAULT first. Internal sections like [__settings__]
// are skipped.
func LoadProfiles(path string) ([]Profile, error) {
	f, err := ini.Load(path)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", path, err)
	}

	var profiles []Profile
	for _, section := range f.Sections() {
		name := section.Name()
		if skipSection(name, section) {
			continue
		}
		profiles = append(profiles, Profile{
			Name:      name,
			Host:      strings.TrimRight(section.Key("host").String(), "/"),
			AccountID: section.Key("account_id").String(),
			AuthType:  section.Key("auth_type").String(),
		})
	}

	sort.Slice(profiles, func(i, j int) bool {
		if (profiles[i].Name == "DEFAULT") != (profiles[j].Name == "DEFAULT") {
			return profiles[i].Name == "DEFAULT"
		}
		return profiles[i].Name < profiles[j].Name
	})
	return profiles, nil
}

func skipSection(name string, section *ini.Section) bool {
	// ini.v1 always materializes a DEFAULT section; only keep it when the
	// file actually defines one with content.
	if name == ini.DefaultSection && len(section.Keys()) == 0 {
		return true
	}
	// Non-profile sections used by other tooling (e.g. [__settings__]).
	if strings.HasPrefix(name, "__") {
		return true
	}
	// A profile without a host cannot be connected to; hide it rather than
	// offering a picker entry that can only fail.
	return section.Key("host").String() == ""
}
