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

// Title is the breadcrumb segment, e.g. "tables(main.silver)".
func (b *Browser) Title() string {
	if h := b.scope.Hash(); h != "" {
		return b.def.Name() + "(" + h + ")"
	}
	return b.def.Name()
}

// Hints lists browser keys for the header.
func (b *Browser) Hints() []key.Binding {
	hints := []key.Binding{}
	if b.def.Child() != "" {
		hints = append(hints, key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", b.def.Child())))
	}
	hints = append(hints,
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "describe")),
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
	def, clients, th := b.def, b.clients, b.th
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		detail, err := def.Describe(ctx, clients, row)
		if err != nil {
			return FlashMsg{Level: FlashError, Text: fmt.Sprintf("describe: %v", err)}
		}
		return PushMsg{View: NewDescribe(th, def.Name()+"/"+row.ID, detail)}
	}
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
			"\n\n" + b.th.Subtle.Render("press ctrl+r to retry, esc to go back")
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
