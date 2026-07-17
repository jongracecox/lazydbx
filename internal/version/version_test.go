package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestString(t *testing.T) {
	// Guard against silently dropping any of the three build fields.
	got := String()
	assert.Contains(t, got, Version)
	assert.Contains(t, got, Commit)
	assert.Contains(t, got, Date)
	assert.Contains(t, got, "lazydbx")
}

func TestStringFormat(t *testing.T) {
	origV, origC, origD := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = origV, origC, origD })

	Version, Commit, Date = "1.2.3", "abc123", "2026-07-17"
	assert.Equal(t, "lazydbx 1.2.3 (commit abc123, built 2026-07-17)", String())
}
