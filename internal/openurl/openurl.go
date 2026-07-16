// Package openurl launches the system web browser for a URL. It is a leaf:
// no other lazydbx package depends on its internals, and it imports nothing
// beyond the standard library.
package openurl

import (
	"os/exec"
	"runtime"
)

// Command returns the OS-specific command that opens url in the default web
// browser. The caller starts it; stdout/stderr are deliberately left unset so
// the opener (open/xdg-open) can never leak text into the running TUI.
func Command(url string) *exec.Cmd {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{url}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		name, args = "xdg-open", []string{url}
	}
	// The opener is fire-and-forget; a context would only risk killing the
	// spawned browser, so noctx is intentional (matches dbx.LoginCommand).
	return exec.Command(name, args...) //nolint:noctx
}
