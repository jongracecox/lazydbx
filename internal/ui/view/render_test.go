package view

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jongracecox/lazydbx/internal/theme"
)

func TestRenderDetailMultilineStrings(t *testing.T) {
	type table struct {
		Name    string `yaml:"name"`
		Comment string `yaml:"comment"`
	}
	obj := table{
		Name:    "gold_accounts",
		Comment: "# 🥇 Gold Accounts\n\n## Details\n\n • **Table Type**: 📐 Dimension Table",
	}

	out := renderDetail(theme.Default(), obj)

	assert.Contains(t, out, "gold_accounts")
	assert.Contains(t, out, "  # 🥇 Gold Accounts\n", "multi-line comment renders as an indented raw block")
	assert.Contains(t, out, "   • **Table Type**: 📐 Dimension Table\n")
	assert.NotContains(t, out, `\n`, "no literal escape sequences in output")
}

func TestRenderDetailNested(t *testing.T) {
	obj := map[string]any{
		"columns": []map[string]any{
			{"name": "id", "type": "bigint"},
		},
	}

	out := renderDetail(theme.Default(), obj)
	lines := strings.Split(out, "\n")

	assert.Contains(t, out, "columns:")
	assert.Contains(t, out, "id")
	// The sequence item is indented under its key.
	var dashLine string
	for _, l := range lines {
		if strings.Contains(l, "-") {
			dashLine = l
			break
		}
	}
	assert.True(t, strings.HasPrefix(dashLine, "  "), "sequence items indent under the key: %q", dashLine)
}
