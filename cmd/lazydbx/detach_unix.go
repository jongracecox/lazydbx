//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach puts the prefetch child in its own session so the shell's completion
// invocation doesn't wait on it — the child outlives the __complete process
// and refreshes the cache in the background.
func detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
