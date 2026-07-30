package view

import (
	"context"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/config"
	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/theme"
)

// linkableViews builds one instance of every view that supports the `o`
// out-link, in the state where the key is live (the SQL view needs its results
// focused — with the editor focused `o` is a typed character).
func linkableViews() map[string]func() View {
	th := theme.Default()
	return map[string]func() View{
		"describe": func() View { return NewDescribe(th, "job/42", map[string]string{"id": "42"}) },
		"logview": func() View {
			return NewLogView(th, "logs", func(context.Context) (string, error) { return "line", nil }, false)
		},
		"logtable": func() View {
			return NewLogTable(th, "logs", func(context.Context) ([]LogRecord, error) { return nil, nil }, false)
		},
		"sqlview": func() View {
			clients := dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, dbx.DAOs{})
			v := NewSQLView(th, clients, config.SQLConfig{}, "select 1", false)
			v.setFocus(focusResults)
			return v
		},
	}
}

func TestViewsOpenWebLinkOnO(t *testing.T) {
	for name, build := range linkableViews() {
		t.Run(name, func(t *testing.T) {
			v := build()
			v.(WebLinkSetter).SetWebLink(WebLink{URL: "https://ws/jobs/42", Hint: "open run"})

			assert.Contains(t, hintKeys(v.Hints()), "o", "the hint appears once a link is set")
			assert.Contains(t, hintHelp(v.Hints()), "open run", "the hint carries the supplied label")

			_, cmd := v.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
			require.NotNil(t, cmd)
			assert.Equal(t, OpenURLMsg{URL: "https://ws/jobs/42"}, cmd())
		})
	}
}

func TestViewsWithoutWebLinkIgnoreO(t *testing.T) {
	for name, build := range linkableViews() {
		t.Run(name, func(t *testing.T) {
			v := build()
			assert.NotContains(t, hintKeys(v.Hints()), "o", "no link, no hint")

			_, cmd := v.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
			if cmd != nil {
				_, opened := cmd().(OpenURLMsg)
				assert.False(t, opened, "`o` must not open anything without a link")
			}
		})
	}
}

// TestWebLinkDefaultHint covers the unlabeled case — the generic label.
func TestWebLinkDefaultHint(t *testing.T) {
	d := NewDescribe(theme.Default(), "t", nil)
	d.SetWebLink(WebLink{URL: "https://ws/explore/data/main"})
	assert.Contains(t, hintHelp(d.Hints()), defaultWebHint)
}

// TestSQLViewWebLinkOnlyWithResultsFocused pins the editor exception: with the
// query focused, `o` types a letter instead of opening the browser.
func TestSQLViewWebLinkOnlyWithResultsFocused(t *testing.T) {
	clients := dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, dbx.DAOs{})
	v := NewSQLView(theme.Default(), clients, config.SQLConfig{}, "select 1", false)
	v.SetWebLink(WebLink{URL: "https://ws/explore/data/main"})
	require.Equal(t, focusEditor, v.focus, "the SQL screen lands on the editor")

	assert.NotContains(t, hintKeys(v.Hints()), "o", "no `o` hint while typing the query")
	got, _ := v.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	assert.Equal(t, "select 1o", got.(*SQLView).editor.Value(), "`o` is a typed character in the editor")
}

// hintHelp lists the descriptions of a view's hints.
func hintHelp(binds []key.Binding) []string {
	out := make([]string, len(binds))
	for i, b := range binds {
		out[i] = b.Help().Desc
	}
	return out
}
