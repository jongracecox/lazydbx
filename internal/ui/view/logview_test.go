package view

import (
	"context"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/adrg/xdg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/theme"
)

func logKey(k string) tea.KeyPressMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "ctrl+r":
		return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
}

func updateLog(v View, msg tea.Msg) (*LogView, tea.Cmd) {
	got, cmd := v.Update(msg)
	return got.(*LogView), cmd
}

// typeSearch feeds each rune of q through the (open) search prompt.
func typeSearch(v *LogView, q string) *LogView {
	for _, r := range q {
		v, _ = updateLog(v, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return v
}

// ansiPrefix returns the leading SGR sequence a styled string emits.
func ansiPrefix(t *testing.T, styled string) string {
	t.Helper()
	i := strings.IndexByte(styled, 'm')
	require.GreaterOrEqual(t, i, 0)
	return styled[:i+1]
}

func newTestLog(content string, follow bool) (*LogView, *int) {
	calls := 0
	fetch := func(context.Context) (string, error) {
		calls++
		return content, nil
	}
	return NewLogView(theme.Default(), "task run 42", fetch, follow), &calls
}

func loadContent(t *testing.T, v *LogView) *LogView {
	t.Helper()
	msg := runCmd(t, v.fetchCmd())
	v, _ = updateLog(v, msg)
	return v
}

func TestLogLoadedContentVisible(t *testing.T) {
	v, _ := newTestLog("line one\nline two\nline three", false)
	assert.Contains(t, v.Render(80, 20), "loading", "shows loading before first content")

	v = loadContent(t, v)
	out := v.Render(80, 20)
	assert.Contains(t, out, "line one")
	assert.Contains(t, out, "line three")
}

func TestLogErrorPath(t *testing.T) {
	fetch := func(context.Context) (string, error) {
		return "", assert.AnError
	}
	v := NewLogView(theme.Default(), "job", fetch, false)
	msg := runCmd(t, v.fetchCmd())
	v, _ = updateLog(v, msg)

	out := v.Render(80, 20)
	assert.Contains(t, out, "error:")
	assert.Contains(t, out, "ctrl+r to retry")
}

func TestLogStaleFetchDropped(t *testing.T) {
	v, _ := newTestLog("fresh", false)
	v.gen = 5
	v, _ = updateLog(v, logLoadedMsg{content: "stale", gen: 3})
	assert.False(t, v.loaded, "older generation result is ignored")
}

func TestLogFollowTickRefetches(t *testing.T) {
	v, calls := newTestLog("body", true)
	require.True(t, v.follow)

	// A tick for the active follow session schedules a refetch + reschedule.
	v, cmd := updateLog(v, followTickMsg{gen: v.followGen})
	require.NotNil(t, cmd)
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok, "follow tick returns a batch")
	require.NotEmpty(t, batch)
	msg := batch[0]() // first cmd is the fetch; the second is the (blocking) tick
	loaded, ok := msg.(logLoadedMsg)
	require.True(t, ok, "first batched cmd is the refetch")
	assert.Equal(t, 1, *calls)

	v, _ = updateLog(v, loaded)
	assert.True(t, v.jumpBottom, "new content while following jumps to bottom")

	// Turning follow off stops the loop: a stale-gen tick is a no-op.
	v, _ = updateLog(v, logKey("f"))
	assert.False(t, v.follow)
	staleGen := v.followGen - 1
	_, cmd = updateLog(v, followTickMsg{gen: staleGen})
	assert.Nil(t, cmd, "tick from a previous follow session is dropped")
}

func TestLogSearchHighlightAndNavigate(t *testing.T) {
	th := theme.Default()
	v, _ := newTestLog("alpha error\nbeta ok\ngamma error\ndelta ok", false)
	v = loadContent(t, v)

	// Open search, type "error", submit.
	v, _ = updateLog(v, logKey("/"))
	assert.True(t, v.CapturesKeys(), "search prompt captures keys")
	v = typeSearch(v, "error")
	v, _ = updateLog(v, logKey("enter"))
	assert.False(t, v.CapturesKeys())

	out := v.Render(80, 20)
	hlPrefix := ansiPrefix(t, th.KeyHint.Reverse(true).Render("x"))
	assert.Contains(t, out, hlPrefix, "matches are highlighted")
	require.Equal(t, []int{0, 2}, v.matchLines, "both matching lines recorded")
	assert.Equal(t, 0, v.matchIdx, "jumps to first match")

	// n advances, N goes back and wraps.
	v, _ = updateLog(v, logKey("n"))
	v.Render(80, 20)
	assert.Equal(t, 1, v.matchIdx)

	v, _ = updateLog(v, logKey("N"))
	v.Render(80, 20)
	assert.Equal(t, 0, v.matchIdx)

	v, _ = updateLog(v, logKey("N"))
	v.Render(80, 20)
	assert.Equal(t, 1, v.matchIdx, "N wraps to the last match")
}

func TestLogEscSemantics(t *testing.T) {
	v, _ := newTestLog("has error here", false)
	v = loadContent(t, v)

	// esc inside the prompt cancels without setting a query.
	v, _ = updateLog(v, logKey("/"))
	require.True(t, v.searchOpen)
	v, _ = updateLog(v, logKey("esc"))
	assert.False(t, v.searchOpen)
	assert.Empty(t, v.searchQuery)

	// Establish an active search.
	v, _ = updateLog(v, logKey("/"))
	v = typeSearch(v, "error")
	v, _ = updateLog(v, logKey("enter"))
	v.Render(80, 20)
	require.NotEmpty(t, v.matchLines)

	// esc clears the active search first.
	v, _ = updateLog(v, logKey("esc"))
	assert.Empty(t, v.searchQuery)
	assert.Empty(t, v.matchLines)

	// esc with nothing active pops the view.
	_, cmd := updateLog(v, logKey("esc"))
	require.NotNil(t, cmd)
	_, ok := cmd().(PopMsg)
	assert.True(t, ok, "esc pops when no search is active")
}

func TestLogCapturesKeysTransitions(t *testing.T) {
	v, _ := newTestLog("x", false)
	assert.False(t, v.CapturesKeys())
	v, _ = updateLog(v, logKey("/"))
	assert.True(t, v.CapturesKeys())
	v, _ = updateLog(v, logKey("esc"))
	assert.False(t, v.CapturesKeys())
}

func TestLogSaveWritesFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)
	xdg.Reload()

	v, _ := newTestLog("saved body\nsecond line", false)
	v = loadContent(t, v)

	cmd := v.save()
	require.NotNil(t, cmd)
	msg := cmd().(logSavedMsg)
	require.NoError(t, msg.err)
	require.NotEmpty(t, msg.path)
	assert.Contains(t, msg.path, "lazydbx")
	assert.Contains(t, msg.path, "dumps")

	data, err := os.ReadFile(msg.path)
	require.NoError(t, err)
	assert.Equal(t, "saved body\nsecond line", string(data))

	// The save outcome flashes the path.
	_, fcmd := updateLog(v, msg)
	require.NotNil(t, fcmd)
	flashMsg, ok := fcmd().(FlashMsg)
	require.True(t, ok)
	assert.Contains(t, flashMsg.Text, msg.path)
}

func TestLogWrapToggleResetsXOffset(t *testing.T) {
	v, _ := newTestLog(strings.Repeat("wide ", 60), false)
	v = loadContent(t, v)
	v.Render(40, 10)
	v, _ = updateLog(v, logKey("l")) // scroll right (no-wrap)
	assert.Equal(t, 1, v.xoff)
	v, _ = updateLog(v, logKey("w")) // toggle wrap
	assert.True(t, v.wrap)
	assert.Equal(t, 0, v.xoff, "wrap resets horizontal offset")
}
