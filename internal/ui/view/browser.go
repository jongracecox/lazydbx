package view

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/engine"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/theme"
	"github.com/jongracecox/lazydbx/internal/ui/component"
)

// StatusProvider lets a view contribute the right-hand status bar segment.
type StatusProvider interface {
	Status(now time.Time) string
}

// Browser is THE generic resource list view: any ResourceDef renders through
// it. It subscribes to the engine on push and unsubscribes on pop; rows
// arrive as engine.DataEvent messages.
type Browser struct {
	def     resource.Def
	scope   resource.Scope
	clients *dbx.Clients
	eng     *engine.Engine
	th      theme.Theme
	key     engine.Key

	table     component.Table
	filter    component.FilterBar
	filtering bool
	filterVal string

	allRows   []resource.Row
	fetchedAt time.Time
	err       error
	stale     bool
	loaded    bool

	width, height int
}

// NewBrowser builds a browser for one def+scope.
func NewBrowser(def resource.Def, scope resource.Scope, clients *dbx.Clients, eng *engine.Engine, th theme.Theme, initialFilter string) *Browser {
	return &Browser{
		def:     def,
		scope:   scope,
		clients: clients,
		eng:     eng,
		th:      th,
		key: engine.Key{
			Profile:  clients.Profile().Name,
			Resource: def.Name(),
			Scope:    scope.Hash(),
		},
		table:     component.NewTable(th),
		filter:    component.NewFilterBar(),
		filterVal: initialFilter,
	}
}

// Init subscribes to the engine; cached rows arrive synchronously.
func (b *Browser) Init() tea.Cmd {
	def, clients, scope := b.def, b.clients, b.scope
	fetch := func(ctx context.Context) ([]resource.Row, error) {
		return def.List(ctx, clients, scope)
	}
	k, interval := b.key, def.PollInterval()
	eng := b.eng
	return func() tea.Msg {
		eng.Watch(k, fetch, interval)
		return nil
	}
}

// Close unsubscribes; the engine keeps the cache warm for re-entry.
func (b *Browser) Close() { b.eng.Unwatch(b.key) }

// Title is the breadcrumb segment: the item selected to reach this view
// (e.g. drilling into catalog qsic_internal gives "qsic_internal"), or the
// resource name at the root. Full context lives in the header's scope path.
func (b *Browser) Title() string {
	args := b.def.Args()
	if len(args) > 0 {
		if v := b.scope[args[len(args)-1]]; v != "" {
			return v
		}
	}
	return b.def.Name()
}

// Hints lists browser keys for the header. In sort mode the hints switch to
// the column picker's keys.
func (b *Browser) Hints() []key.Binding {
	if b.table.InSortMode() {
		return []key.Binding{
			key.NewBinding(key.WithKeys("left"), key.WithHelp("←/→", "pick column")),
			key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort (again reverses)")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	}
	hints := []key.Binding{}
	if b.def.Child() != "" {
		hints = append(hints, key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", b.def.Child())))
	}
	hints = append(hints,
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "describe")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort")),
		key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl-r", "refresh")),
	)
	for _, a := range b.def.Actions() {
		hints = append(hints, key.NewBinding(key.WithKeys(a.Key), key.WithHelp(a.Key, a.Name)))
	}
	return hints
}

// Update routes messages: filter bar first when open, then data events and
// browser keys.
func (b *Browser) Update(msg tea.Msg) (View, tea.Cmd) {
	if ev, ok := msg.(engine.DataEvent); ok {
		if ev.Key == b.key {
			b.applyData(ev)
		}
		return b, nil
	}

	if done, ok := msg.(loginDoneMsg); ok {
		if done.err != nil {
			return b, func() tea.Msg { return FlashMsg{Level: FlashError, Text: "login failed: " + done.err.Error()} }
		}
		b.eng.RefreshNow(b.key)
		return b, func() tea.Msg { return FlashMsg{Level: FlashInfo, Text: "logged in — refreshing…"} }
	}

	if b.filtering {
		var event component.Event
		var cmd tea.Cmd
		b.filter, event, cmd = b.filter.Update(msg)
		switch event.Kind {
		case component.EventChanged, component.EventSubmit, component.EventCancel:
			b.filterVal = event.Value
			b.refreshTable()
			if event.Kind != component.EventChanged {
				b.filtering = false
			}
		case component.EventNone:
		}
		return b, cmd
	}

	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		// Sort mode owns the keyboard until confirmed or canceled.
		if b.table.InSortMode() {
			var cmd tea.Cmd
			b.table, cmd = b.table.Update(kmsg)
			return b, cmd
		}
		return b.handleKey(kmsg)
	}
	return b, nil
}

func (b *Browser) handleKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "/":
		b.filtering = true
		var cmd tea.Cmd
		b.filter, cmd = b.filter.Open(b.filterVal)
		return b, cmd
	case "esc":
		if b.filterVal != "" {
			b.filterVal = ""
			b.refreshTable()
			return b, nil
		}
		return b, func() tea.Msg { return PopMsg{} }
	case "ctrl+r":
		b.eng.RefreshNow(b.key)
		return b, func() tea.Msg { return FlashMsg{Level: FlashInfo, Text: "refreshing…"} }
	case "L":
		if b.err != nil {
			cmd := b.login()
			return b, cmd
		}
	case "enter":
		cmd := b.drillDown()
		return b, cmd
	case "d":
		cmd := b.describe()
		return b, cmd
	}

	for _, action := range b.def.Actions() {
		if action.Key == msg.String() {
			cmd := b.runAction(action)
			return b, cmd
		}
	}

	var cmd tea.Cmd
	b.table, cmd = b.table.Update(msg)
	return b, cmd
}

// drillDown pushes the child browser for the selected row, or falls back to
// describe on leaf resources so Enter always does something.
func (b *Browser) drillDown() tea.Cmd {
	row, ok := b.table.SelectedRow()
	if !ok {
		return nil
	}
	if b.def.Child() == "" {
		return b.describe()
	}
	return func() tea.Msg {
		return DrillDownMsg{
			Resource: b.def.Child(),
			Scope:    b.def.ChildScope(b.scope, row),
		}
	}
}

// describe loads the detail object and pushes a describe view.
func (b *Browser) describe() tea.Cmd {
	row, ok := b.table.SelectedRow()
	if !ok {
		return nil
	}
	def, clients, scope, th := b.def, b.clients, b.scope, b.th
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		detail, err := def.Describe(ctx, clients, scope, row)
		if err != nil {
			return FlashMsg{Level: FlashError, Text: fmt.Sprintf("describe: %v", err)}
		}
		return PushMsg{View: NewDescribe(th, def.Name()+"/"+row.ID, detail)}
	}
}

// loginDoneMsg reports the result of a suspended `databricks auth login`.
type loginDoneMsg struct{ err error }

// login suspends the TUI and runs the CLI's browser-based OAuth flow.
func (b *Browser) login() tea.Cmd {
	cmd, err := dbx.LoginCommand(b.clients.Profile().Name)
	if err != nil {
		return func() tea.Msg { return FlashMsg{Level: FlashError, Text: err.Error()} }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return loginDoneMsg{err: err} })
}

func (b *Browser) runAction(action resource.Action) tea.Cmd {
	row, hasRow := b.table.SelectedRow()
	if action.NeedsRow && !hasRow {
		return nil
	}
	clients, scope := b.clients, b.scope
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return action.Run(ctx, clients, scope, row)
	}
}

func (b *Browser) applyData(ev engine.DataEvent) {
	b.allRows = ev.Rows
	b.fetchedAt = ev.FetchedAt
	b.err = ev.Err
	b.stale = ev.Stale
	b.loaded = b.loaded || ev.Err == nil || len(ev.Rows) > 0
	b.refreshTable()
}

func (b *Browser) refreshTable() {
	filtered := b.allRows
	if b.filterVal != "" {
		filtered = make([]resource.Row, 0, len(b.allRows))
		for _, r := range b.allRows {
			if r.MatchesFilter(b.filterVal) {
				filtered = append(filtered, r)
			}
		}
	}
	b.table.SetData(b.def.Columns(), filtered)
}

// Render draws the (optional) filter bar, the table, and error states.
func (b *Browser) Render(width, height int) string {
	b.width, b.height = width, height

	var top string
	tableHeight := height
	if b.filtering || b.filterVal != "" {
		top = b.filter.View(b.th, width) + "\n"
		tableHeight--
	}

	switch {
	case !b.loaded && b.err != nil:
		return top + b.th.Error.Render(fmt.Sprintf("error loading %s: %v", b.def.Name(), b.err)) +
			"\n\n" + b.th.Subtle.Render("press L to log in, ctrl+r to retry, esc to go back")
	case !b.loaded:
		return top + b.th.Subtle.Render("loading "+b.def.Name()+"…")
	}

	b.table.SetSize(width, tableHeight)
	return top + b.table.View()
}

// Status renders the right status segment: counts and freshness.
func (b *Browser) Status(now time.Time) string {
	if !b.loaded {
		return ""
	}
	count := strconv.Itoa(b.table.Len()) + "/" + strconv.Itoa(len(b.allRows))
	age := now.Sub(b.fetchedAt).Round(time.Second)
	fresh := fmt.Sprintf("⟳ %s ago", age)
	if b.err != nil {
		return b.th.Error.Render(fmt.Sprintf("%s  ⚠ %s ago (refresh failed)", count, age))
	}
	if b.stale {
		fresh = "⟳ refreshing…"
	}
	return b.th.Subtle.Render(count + "  " + fresh)
}
