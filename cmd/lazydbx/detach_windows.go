//go:build windows

package main

import "os/exec"

// detach is a no-op on Windows: the prefetch child is started without waiting,
// which is enough for the completion invocation not to block on it.
func detach(*exec.Cmd) {}
