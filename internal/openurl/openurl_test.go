package openurl

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandIncludesURL(t *testing.T) {
	const u = "https://acme.cloud.databricks.com/jobs/123"
	cmd := Command(u)
	require.NotNil(t, cmd)
	assert.Contains(t, cmd.Args, u, "the URL must be passed to the opener")

	switch runtime.GOOS {
	case "darwin":
		assert.Equal(t, "open", cmd.Args[0])
	case "windows":
		assert.Equal(t, "rundll32", cmd.Args[0])
	default:
		assert.Equal(t, "xdg-open", cmd.Args[0])
	}
}
