package dbx

import (
	"fmt"
	"os/exec"
)

// LoginCommand builds the interactive `databricks auth login` command for a
// profile. The TUI suspends while it runs (browser-based OAuth flow), then
// resumes and refreshes. Requires the Databricks CLI on PATH.
func LoginCommand(profile string) (*exec.Cmd, error) {
	path, err := exec.LookPath("databricks")
	if err != nil {
		return nil, fmt.Errorf("databricks CLI not found on PATH — install it to log in from lazydbx")
	}
	// The command runs interactively under the TUI's control (tea.ExecProcess
	// manages its lifetime), so there is no context to attach.
	return exec.Command(path, "auth", "login", "--profile", profile), nil //nolint:noctx
}
