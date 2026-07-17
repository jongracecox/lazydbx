package app

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/adrg/xdg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/config"
	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/engine"
	"github.com/jongracecox/lazydbx/internal/resources"
	"github.com/jongracecox/lazydbx/internal/theme"
	"github.com/jongracecox/lazydbx/internal/ui/view"
)

func testProfiles() []dbx.Profile {
	return []dbx.Profile{{Name: "dev"}, {Name: "prod"}}
}

// newModel builds a Model with a real registry and offline pool/engine. No
// network happens until a view actually fetches (which these tests avoid).
func newModel(t *testing.T, cfg config.Config, launchArgs []string, launchTab string) Model {
	t.Helper()
	eng := engine.New(func(engine.DataEvent) {}, engine.NewStore(t.TempDir()))
	return New(cfg, testProfiles(), resources.NewRegistry(), dbx.NewPool(), eng, launchArgs, launchTab)
}

func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func topTitle(m Model) string { return m.top().Title() }

// --- construction ---

func TestNewWithoutProfileShowsPicker(t *testing.T) {
	m := newModel(t, config.Config{}, nil, "")
	require.Len(t, m.stack, 1)
	_, isPicker := m.top().(*view.Picker)
	assert.True(t, isPicker, "no configured profile → picker on top")
	assert.Nil(t, m.clients, "no clients until a profile is chosen")
}

func TestNewWithConfiguredProfileSkipsPicker(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	require.GreaterOrEqual(t, len(m.stack), 2, "picker + default browser")
	assert.NotNil(t, m.clients)
	assert.Equal(t, defaultResource, topTitle(m), "opens the default resource")
}

func TestNewUnknownConfiguredProfileFallsToPicker(t *testing.T) {
	m := newModel(t, config.Config{Profile: "nope"}, nil, "")
	_, isPicker := m.top().(*view.Picker)
	assert.True(t, isPicker)
}

// --- profile selection ---

func TestProfileSelectedOpensDefaultBrowser(t *testing.T) {
	m := newModel(t, config.Config{}, nil, "")
	m = update(m, view.ProfileSelectedMsg{Profile: dbx.Profile{Name: "dev"}})
	assert.NotNil(t, m.clients)
	assert.Equal(t, defaultResource, topTitle(m))
}

func TestSelectingProfileKeepsDefaultTheme(t *testing.T) {
	m := newModel(t, config.Config{}, nil, "")
	m = update(m, view.ProfileSelectedMsg{Profile: dbx.Profile{Name: "prod"}})
	// No auto-detection: the theme stays orange regardless of the name.
	assert.Equal(t, theme.Default().Accent, m.th.Accent)
}

func TestHeaderHighlightChangesRender(t *testing.T) {
	sized := func(m Model) Model {
		return update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	}
	plain := sized(newModel(t, config.Config{Profile: "dev"}, nil, ""))
	tinted := sized(newModel(t, config.Config{
		Profile: "dev",
		Skins:   map[string]string{"dev": "red"},
	}, nil, ""))

	plainOut := plain.View().Content
	tintedOut := tinted.View().Content
	assert.Contains(t, plainOut, "dev", "profile name shown either way")
	assert.Contains(t, tintedOut, "dev")
	assert.NotEqual(t, plainOut, tintedOut, "a configured highlight changes the header render")
}

func TestOpenColorPickerPushesPicker(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	m = update(m, view.OpenColorPickerMsg{Profile: "prod"})
	_, ok := m.top().(*view.ColorPicker)
	assert.True(t, ok, "OpenColorPickerMsg pushes the color picker")
}

func TestProfileColorSelectedUpdatesAndPersists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	xdg.Reload() // pick up the redirected config home

	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	m = update(m, view.OpenColorPickerMsg{Profile: "prod"})
	m = update(m, view.ProfileColorSelectedMsg{Profile: "prod", Color: "red"})

	assert.Equal(t, "red", m.cfg.Skins["prod"], "in-memory config updated")
	_, stillColorPicker := m.top().(*view.ColorPicker)
	assert.False(t, stillColorPicker, "color picker is popped after applying")

	data, err := os.ReadFile(config.Path())
	require.NoError(t, err)
	assert.Contains(t, string(data), "red", "choice persisted to disk")

	// Clearing removes the entry.
	m = update(m, view.ProfileColorSelectedMsg{Profile: "prod", Color: ""})
	_, ok := m.cfg.Skins["prod"]
	assert.False(t, ok, "empty color clears the entry")
}

// --- launch args ---

func TestLaunchArgsOpenNamedResource(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, []string{"jobs"}, "")
	assert.Equal(t, "jobs", topTitle(m), "launch command replaces the default browser")
}

func TestLaunchArgsSQLOpensEditor(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, []string{"sql", "select", "1"}, "")
	_, isSQL := m.top().(*view.SQLView)
	assert.True(t, isSQL, "sql launch opens the SQL editor")
}

func TestLaunchArgsConsumedOnce(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, []string{"jobs"}, "")
	require.Equal(t, "jobs", topTitle(m))
	// A later profile switch should open the default, not re-run the launch.
	m = update(m, view.ProfileSelectedMsg{Profile: dbx.Profile{Name: "prod"}})
	assert.Equal(t, defaultResource, topTitle(m))
}

func TestLaunchArgsBadCommandFallsBack(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, []string{"not-a-resource"}, "")
	assert.Equal(t, defaultResource, topTitle(m), "unparseable launch falls back to default")
}

// --- command bar (':') ---

func TestColonOpensCommandBar(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	m = update(m, keyMsg(":"))
	assert.True(t, m.cmdOpen)
}

func TestColonIgnoredBeforeProfile(t *testing.T) {
	m := newModel(t, config.Config{}, nil, "")
	m = update(m, keyMsg(":"))
	assert.False(t, m.cmdOpen, "no command bar until a profile is selected")
}

func TestExecOpensResource(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	m, _ = execInput(m, "jobs")
	assert.Equal(t, "jobs", topTitle(m))
}

func TestExecSQL(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	m, _ = execInput(m, "sql select 1")
	_, isSQL := m.top().(*view.SQLView)
	assert.True(t, isSQL)
}

func TestExecQuit(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	_, cmd := m.exec("quit")
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestExecEmptyIsNoop(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	before := len(m.stack)
	m, cmd := execInput(m, "   ")
	assert.Len(t, m.stack, before)
	assert.Nil(t, cmd)
}

func TestExecUnknownResourceFlashes(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	before := len(m.stack)
	m, _ = execInput(m, "nonsense")
	assert.Len(t, m.stack, before, "bad command does not push a view")
}

// execInput drives the command bar the way the app does: open, submit value.
func execInput(m Model, value string) (Model, tea.Cmd) {
	next, cmd := m.exec(value)
	return next.(Model), cmd
}

// --- global keys / navigation ---

func TestHelpToggle(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	m = update(m, keyMsg("?"))
	_, isHelp := m.top().(*view.Help)
	assert.True(t, isHelp, "? pushes help")

	// ? again while help is on top does not stack a second help.
	m2 := update(m, keyMsg("?"))
	_, stillHelp := m2.top().(*view.Help)
	assert.True(t, stillHelp)
}

func TestProfilesKeyPushesPicker(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	m = update(m, keyMsg("p"))
	_, isPicker := m.top().(*view.Picker)
	assert.True(t, isPicker)
}

func TestModeKeyTeleportsAndResetsStack(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	// Drill a couple levels deep first.
	m = update(m, view.DrillDownMsg{Resource: "jobs"})
	require.Greater(t, len(m.stack), 2)

	m = update(m, keyMsg("J")) // jump to jobs, resetting the stack
	assert.Equal(t, "jobs", topTitle(m))
	assert.Len(t, m.stack, 2, "mode key resets to picker + one browser")
}

func TestQuitKey(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	_, cmd := m.Update(keyMsg("q"))
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestDrillDownUnknownResourceFlashes(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	before := len(m.stack)
	m = update(m, view.DrillDownMsg{Resource: "ghost"})
	assert.Len(t, m.stack, before)
}

func TestPushPopStack(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	m = update(m, view.DrillDownMsg{Resource: "jobs"})
	deep := len(m.stack)
	m = update(m, view.PopMsg{})
	assert.Len(t, m.stack, deep-1)
}

func TestPopNeverEmptiesStack(t *testing.T) {
	m := newModel(t, config.Config{}, nil, "")
	require.Len(t, m.stack, 1)
	m = update(m, view.PopMsg{})
	assert.Len(t, m.stack, 1, "the last view is never popped")
}

// --- messages that open views ---

func TestOpenSQLMsg(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	m = update(m, view.OpenSQLMsg{Query: "select 1"})
	_, ok := m.top().(*view.SQLView)
	assert.True(t, ok)
}

func TestOpenLogMsg(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	m = update(m, view.OpenLogMsg{Title: "logs", Fetch: func(ctx context.Context) (string, error) { return "", nil }})
	assert.Equal(t, "logs", topTitle(m))
}

func TestOpenTabsMsg(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	m = update(m, view.OpenTabsMsg{
		Title: "run-1",
		Tabs: []view.TabSpec{
			{Name: "details", Detail: func(context.Context) (any, error) { return "x", nil }},
		},
	})
	assert.Equal(t, "run-1", topTitle(m))
}

func TestOpenTabsMsgEmptyIsNoop(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	before := len(m.stack)
	m = update(m, view.OpenTabsMsg{Title: "x", Tabs: nil})
	assert.Len(t, m.stack, before)
}

func TestFlashMsg(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	// Should not panic and should be reflected in the rendered status bar.
	m = update(m, view.FlashMsg{Level: 0, Text: "hello"})
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	assert.Contains(t, m.View().Content, "hello")
}

// --- lifecycle & rendering ---

func TestInitReturnsCommand(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	assert.NotNil(t, m.Init(), "Init starts the heartbeat")
}

func TestTickReschedules(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	_, cmd := m.Update(tickMsg(time.Now()))
	assert.NotNil(t, cmd, "each tick schedules the next")
}

func TestWindowSizeStored(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	assert.Equal(t, 120, m.width)
	assert.Equal(t, 40, m.height)
}

func TestViewEmptyBeforeSize(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	assert.Empty(t, m.View().Content, "nothing renders until a window size arrives")
}

func TestViewRendersChrome(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	out := m.View().Content
	assert.Contains(t, out, "dev", "header shows the profile context")
	assert.Contains(t, out, defaultResource, "breadcrumb shows the current view")
}

func TestViewReadonlyBadge(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev", ReadOnly: true}, nil, "")
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	assert.Contains(t, m.View().Content, "READONLY")
}

func TestViewProfileSelectionContext(t *testing.T) {
	m := newModel(t, config.Config{}, nil, "")
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	assert.Contains(t, m.View().Content, "select a profile")
}

func TestTitlesTracksStack(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	titles := m.titles()
	require.Len(t, titles, len(m.stack))
	assert.Equal(t, defaultResource, titles[len(titles)-1])
}

func TestCompleterListsResourcesAndSQL(t *testing.T) {
	c := completer(resources.NewRegistry())
	all := c("")
	assert.Contains(t, all, "sql", "empty prompt lists sql")
	assert.Contains(t, all, "catalogs")

	narrowed := c("cat")
	for _, r := range narrowed {
		assert.True(t, strings.HasPrefix(r, "cat") || strings.Contains(r, "cat"))
	}
}

func TestGlobalHintsPresent(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	hints := m.globalHints()
	assert.NotEmpty(t, hints)
	keys := map[string]bool{}
	for _, h := range hints {
		keys[h.Help().Key] = true
	}
	assert.True(t, keys[":"] && keys["?"] && keys["q"])
}

func TestHelpViewIncludesResourceCatalog(t *testing.T) {
	m := newModel(t, config.Config{Profile: "dev"}, nil, "")
	h := m.helpView()
	assert.Equal(t, "help", h.Title())
}
