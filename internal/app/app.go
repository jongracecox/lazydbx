// Package app hosts the root Bubble Tea model: the view stack, global keys,
// overlays (command bar), and message routing. Views own the body region;
// the app owns all other chrome.
package app

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongracecox/lazydbx/internal/config"
	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/engine"
	"github.com/jongracecox/lazydbx/internal/favorites"
	"github.com/jongracecox/lazydbx/internal/openurl"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/theme"
	"github.com/jongracecox/lazydbx/internal/ui/component"
	"github.com/jongracecox/lazydbx/internal/ui/view"
)

// Chrome geometry: the status bar line and the command bar's two lines when
// open. The header's height is dynamic (banner) — measured at render.
const (
	statusHeight = 1
	cmdBarHeight = 2
)

// defaultResource is the view opened right after profile selection.
const defaultResource = "catalogs"

// Model is the root application model.
type Model struct {
	cfg      config.Config
	th       theme.Theme
	registry *resource.Registry
	pool     *dbx.Pool
	eng      *engine.Engine
	profiles []dbx.Profile
	clients  *dbx.Clients // nil until a profile is chosen

	favs *favorites.Store

	stack     []view.View
	cmdbar    component.CmdBar
	cmdOpen   bool
	statusbar component.StatusBar

	// launch is an optional CLI command (e.g. "tables main.silver") applied to
	// the first profile selection in place of the default resource; consumed
	// after use so later profile switches open the default view.
	launch string

	width, height int
}

// New builds the root model. When cfg.Profile is set the picker is skipped.
// launch is an optional command (same syntax as the ':' bar) opened in place
// of the default resource on the first profile selection; "" opens the default.
func New(cfg config.Config, profiles []dbx.Profile, registry *resource.Registry, pool *dbx.Pool, eng *engine.Engine, launch string) Model {
	m := Model{
		cfg:      cfg,
		th:       theme.Default(),
		registry: registry,
		pool:     pool,
		eng:      eng,
		profiles: profiles,
		favs:     favorites.NewDefault(),
		cmdbar:   component.NewCmdBar(completer(registry)),
		launch:   strings.TrimSpace(launch),
	}

	if cfg.Profile != "" {
		for _, p := range profiles {
			if p.Name == cfg.Profile {
				m.selectProfile(p)
				return m
			}
		}
	}
	m.stack = []view.View{view.NewPicker(m.th, profiles)}
	return m
}

// selectProfile switches clients/theme and resets the stack to the profile
// picker (the permanent top level — esc from the default browser lands
// there) with the default resource browser above it.
func (m *Model) selectProfile(p dbx.Profile) {
	m.clients = m.pool.Get(p)
	m.th = theme.ForProfile(p.Name, m.cfg.Skins)
	m.cmdbar = component.NewCmdBar(completer(m.registry))

	for _, v := range m.stack {
		v.Close()
	}
	m.stack = []view.View{view.NewPicker(m.th, m.profiles)}
	if v, ok := m.launchView(); ok {
		m.stack = append(m.stack, v)
	} else if def, ok := m.registry.Get(defaultResource); ok {
		m.stack = append(m.stack, m.newBrowser(def, resource.Scope{}, ""))
	}
}

// launchView builds the initial view from the CLI launch command, mirroring
// exec's routing. It consumes m.launch so it only ever affects the first
// profile selection; a parse error flashes and falls back to the default view.
// Keep in sync with validateLaunch in cmd/lazydbx (which pre-checks at startup).
func (m *Model) launchView() (view.View, bool) {
	input := m.launch
	m.launch = ""
	if input == "" {
		return nil, false
	}
	if input == "sql" || strings.HasPrefix(input, "sql ") {
		query := strings.TrimSpace(strings.TrimPrefix(input, "sql"))
		return view.NewSQLView(m.th, m.clients, m.cfg.SQL, query, false), true
	}
	cmd, err := m.registry.Parse(input)
	if err != nil {
		m.statusbar.Flash(component.FlashError, err.Error(), time.Now())
		return nil, false
	}
	return m.newBrowser(cmd.Def, cmd.Scope, cmd.Filter), true
}

// completer feeds the command bar: an empty prompt lists every canonical
// resource (discoverability); a prefix narrows across names and aliases.
func completer(reg *resource.Registry) func(string) []string {
	return func(prefix string) []string {
		if prefix == "" {
			return append(reg.Canonical(), "sql")
		}
		return reg.Complete(prefix)
	}
}

func (m Model) newBrowser(def resource.Def, scope resource.Scope, filter string) view.View {
	return view.NewBrowser(def, scope, m.clients, m.eng, m.th, filter, m.favs)
}

func (m Model) top() view.View {
	if len(m.stack) == 0 {
		return nil
	}
	return m.stack[len(m.stack)-1]
}

// Init starts the heartbeat and the first view.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick()}
	if top := m.top(); top != nil {
		cmds = append(cmds, top.Init())
	}
	return tea.Batch(cmds...)
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Update routes: window size → app messages → command bar overlay → global
// keys → top view.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		return m, tick()

	case view.PushMsg:
		return m.push(msg.View)

	case view.PopMsg:
		return m.pop()

	case view.DrillDownMsg:
		def, ok := m.registry.Get(msg.Resource)
		if !ok {
			m.statusbar.Flash(component.FlashError, fmt.Sprintf("unknown resource %q", msg.Resource), time.Now())
			return m, nil
		}
		return m.push(m.newBrowser(def, msg.Scope, ""))

	case view.FlashMsg:
		m.statusbar.Flash(msg.Level, msg.Text, time.Now())
		return m, nil

	case view.OpenSQLMsg:
		return m.push(view.NewSQLView(m.th, m.clients, m.cfg.SQL, msg.Query, msg.Execute))

	case view.OpenURLMsg:
		return m, openURLCmd(msg.URL)

	case view.OpenLogMsg:
		return m.push(view.NewLogView(m.th, msg.Title, msg.Fetch, msg.Follow))

	case view.OpenTabsMsg:
		tabs := make([]view.Tab, 0, len(msg.Tabs))
		for _, spec := range msg.Tabs {
			switch {
			case spec.Log != nil:
				tabs = append(tabs, view.Tab{Name: spec.Name, View: view.NewLogView(m.th, spec.Name, spec.Log.Fetch, spec.Log.Follow)})
			case spec.Detail != nil:
				tabs = append(tabs, view.Tab{Name: spec.Name, View: view.NewLazyDescribe(m.th, msg.Title, spec.Detail)})
			case spec.SQL != nil:
				tabs = append(tabs, view.Tab{Name: spec.Name, View: view.NewSQLView(m.th, m.clients, m.cfg.SQL, spec.SQL.Query, spec.SQL.Execute)})
			case spec.Browse != nil:
				if def, ok := m.registry.Get(spec.Browse.Resource); ok {
					tabs = append(tabs, view.Tab{Name: spec.Name, View: m.newBrowser(def, spec.Browse.Scope, "")})
				}
			}
		}
		if len(tabs) == 0 {
			return m, nil
		}
		return m.push(view.NewTabbed(m.th, msg.Title, tabs))

	case view.ProfileSelectedMsg:
		m.selectProfile(msg.Profile)
		m.statusbar.Flash(component.FlashInfo, "profile: "+msg.Profile.Name, time.Now())
		if top := m.top(); top != nil {
			return m, top.Init()
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m.forward(msg)
}

// openURLCmd launches the system browser for url without blocking the TUI.
// It starts (not runs) the opener so we never wait on the browser process,
// and flashes the outcome.
func openURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if err := openurl.Command(url).Start(); err != nil {
			return view.FlashMsg{Level: view.FlashError, Text: "open browser: " + err.Error()}
		}
		return view.FlashMsg{Level: view.FlashInfo, Text: "opening " + url}
	}
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.cmdOpen {
		var event component.Event
		var cmd tea.Cmd
		m.cmdbar, event, cmd = m.cmdbar.Update(msg)
		switch event.Kind {
		case component.EventSubmit:
			m.cmdOpen = false
			return m.exec(event.Value)
		case component.EventCancel:
			m.cmdOpen = false
			return m, nil
		case component.EventChanged, component.EventNone:
		}
		return m, cmd
	}

	// Views that capture text input (e.g. the SQL editor) get first refusal on
	// keys so typed characters like ':' or 'q' aren't stolen by the global
	// shortcuts below. ctrl+c still quits unconditionally.
	if msg.String() != "ctrl+c" {
		if v, ok := m.top().(interface{ CapturesKeys() bool }); ok && v.CapturesKeys() {
			return m.forward(msg)
		}
	}

	switch msg.String() {
	case ":":
		if m.clients != nil {
			m.cmdOpen = true
			var cmd tea.Cmd
			m.cmdbar, cmd = m.cmdbar.Open()
			return m, cmd
		}
		return m, nil
	case "?":
		if _, isHelp := m.top().(*view.Help); !isHelp {
			return m.push(m.helpView())
		}
	case "p", "ctrl+p":
		if _, isPicker := m.top().(*view.Picker); !isPicker {
			return m.push(view.NewPicker(m.th, m.profiles))
		}
		return m, nil
	case "q", "ctrl+c":
		// q is quit only outside text inputs; views with inputs are
		// guarded because the command/filter bars intercept earlier.
		return m, tea.Quit
	}

	// Uppercase mode keys teleport to a root resource, resetting the stack
	// (unlike ':' commands, which push and keep history).
	if name, ok := modeKeys[msg.String()]; ok && m.clients != nil {
		return m.switchMode(name)
	}

	return m.forward(msg)
}

// modeKeys are single-keystroke jumps between top-level resources. Uppercase
// so they can't collide with j/k navigation or per-view verbs.
var modeKeys = map[string]string{
	"J": "jobs",
	"C": "catalogs",
	"P": "pipelines",
	"A": "apps", // registered in a later phase; flashes until then
}

// switchMode resets the stack to picker + the named root browser.
func (m Model) switchMode(name string) (tea.Model, tea.Cmd) {
	def, ok := m.registry.Get(name)
	if !ok {
		m.statusbar.Flash(component.FlashWarn, name+" is not available yet", time.Now())
		return m, nil
	}
	for i := 1; i < len(m.stack); i++ {
		m.stack[i].Close()
	}
	m.stack = m.stack[:1]
	b := m.newBrowser(def, resource.Scope{}, "")
	m.stack = append(m.stack, b)
	return m, b.Init()
}

// forward sends a message to the top view.
func (m Model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.stack) == 0 {
		return m, nil
	}
	v, cmd := m.top().Update(msg)
	m.stack[len(m.stack)-1] = v
	return m, cmd
}

func (m Model) push(v view.View) (tea.Model, tea.Cmd) {
	m.stack = append(m.stack, v)
	return m, v.Init()
}

func (m Model) pop() (tea.Model, tea.Cmd) {
	if len(m.stack) <= 1 {
		return m, nil // never pop the last view
	}
	m.top().Close()
	m.stack = m.stack[:len(m.stack)-1]
	return m, nil
}

// exec runs a submitted `:` command.
func (m Model) exec(input string) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(input) == "" {
		return m, nil
	}
	if input == "q" || input == "quit" {
		return m, tea.Quit
	}
	if input == "sql" || strings.HasPrefix(input, "sql ") {
		query := strings.TrimSpace(strings.TrimPrefix(input, "sql"))
		return m.push(view.NewSQLView(m.th, m.clients, m.cfg.SQL, query, false))
	}
	cmd, err := m.registry.Parse(input)
	if err != nil {
		m.statusbar.Flash(component.FlashError, err.Error(), time.Now())
		return m, nil
	}
	return m.push(m.newBrowser(cmd.Def, cmd.Scope, cmd.Filter))
}

func (m Model) helpView() view.View {
	sections := []view.HelpSection{
		{Title: "Global", Bindings: m.globalHints()},
	}
	if top := m.top(); top != nil && len(top.Hints()) > 0 {
		sections = append(sections, view.HelpSection{Title: top.Title(), Bindings: top.Hints()})
	}
	sections = append(sections, view.HelpSection{
		Title: "Resources — open with :name [args]",
		Lines: append(m.registry.Summaries(), "sql  (ad-hoc SQL editor)"),
	})
	return view.NewHelp(m.th, sections)
}

func (m Model) globalHints() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "command")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "profiles")),
		key.NewBinding(key.WithKeys("J"), key.WithHelp("J/C/P", "jobs/catalogs/pipelines")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

// View assembles chrome + body.
func (m Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	if m.width == 0 || m.height == 0 {
		return v
	}

	context := "select a profile"
	badges := []string{}
	if m.clients != nil {
		p := m.clients.Profile()
		context = p.Name + " " + m.th.Subtle.Render("("+p.ShortHost()+")")
		if p.IsAccount() {
			badges = append(badges, "ACCOUNT")
		}
	}
	if m.cfg.ReadOnly {
		badges = append(badges, "READONLY")
	}

	var hints []key.Binding
	if top := m.top(); top != nil {
		hints = append(hints, top.Hints()...)
	}
	hints = append(hints, m.globalHints()...)
	header := component.Header(m.th, m.width, m.height, context, badges, hints)

	bodyHeight := m.height - lipgloss.Height(header) - statusHeight
	var cmdbar string
	if m.cmdOpen {
		bodyHeight -= cmdBarHeight
		cmdbar = m.cmdbar.View(m.th, m.width) + "\n"
	}

	body := ""
	if top := m.top(); top != nil {
		body = top.Render(m.width, bodyHeight)
	}
	body = lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight).Render(body)

	now := time.Now()
	right := ""
	if s, ok := m.top().(view.StatusProvider); ok {
		right = s.Status(now)
	}
	crumbs := component.Breadcrumbs(m.th, m.titles())
	status := m.statusbar.Render(m.th, m.width, crumbs, right, now)

	v.SetContent(header + "\n" + body + "\n" + cmdbar + status)
	return v
}

func (m Model) titles() []string {
	titles := make([]string, len(m.stack))
	for i, v := range m.stack {
		titles[i] = v.Title()
	}
	return titles
}
