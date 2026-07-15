package view

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/engine"
	"github.com/jongracecox/lazydbx/internal/favorites"
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

	favs *favorites.Store

	table     component.Table
	filter    component.FilterBar
	filtering bool
	filterVal string

	// Tag filter (defs implementing resource.Tagger): the `t` popup toggles
	// tags; rows must carry ALL selected tags to stay visible.
	tagMode     bool
	tagOptions  []string
	tagCursor   int
	tagSelected map[string]bool

	allRows   []resource.Row
	fetchedAt time.Time
	err       error
	stale     bool
	loaded    bool

	width, height int
}

// favMarker is the cell content of the injected favorites column.
const favMarker = "*"

// NewBrowser builds a browser for one def+scope. favs may be nil (no
// favorites column, e.g. in tests).
func NewBrowser(def resource.Def, scope resource.Scope, clients *dbx.Clients, eng *engine.Engine, th theme.Theme, initialFilter string, favs *favorites.Store) *Browser {
	table := component.NewTable(th)
	// Cell 0 is the injected favorites star; defs that classify their cells
	// (run states, health) drive coloring for the rest, shifted by one.
	defStyler, hasStyler := def.(resource.Styler)
	table.SetCellStyler(func(col int, value string) resource.CellClass {
		if favs != nil {
			if col == 0 {
				if value == favMarker {
					return resource.CellRunning // accent — the star pops
				}
				return resource.CellDefault
			}
			col--
		}
		if hasStyler {
			return defStyler.CellClass(col, value)
		}
		return resource.CellDefault
	})
	return &Browser{
		def:     def,
		scope:   scope,
		clients: clients,
		eng:     eng,
		th:      th,
		favs:    favs,
		key: engine.Key{
			Profile:  clients.Profile().Name,
			Resource: def.Name(),
			Scope:    scope.Hash(),
		},
		table:     table,
		filter:    component.NewFilterBar(),
		filterVal: initialFilter,
	}
}

// favKey scopes favorites to this exact view (resource + drill-down scope).
func (b *Browser) favKey() string {
	return b.def.Name() + "|" + b.scope.Hash()
}

// CapturesKeys claims keyboard priority over global shortcuts while an
// interactive mode is active: typing "prod" into the filter must not open
// the profile picker, and popup/sort keys must not quit the app.
func (b *Browser) CapturesKeys() bool {
	return b.filtering || b.tagMode || b.table.InSortMode()
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
	if b.tagMode {
		return []key.Binding{
			key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle tag")),
			key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear all")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
		}
	}
	if b.table.InSortMode() {
		return []key.Binding{
			key.NewBinding(key.WithKeys("left"), key.WithHelp("←/→", "pick column")),
			key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort (again reverses)")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	}
	hints := []key.Binding{}
	if _, isOpener := b.def.(resource.Opener); isOpener {
		hints = append(hints, key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")))
	} else if b.def.Child() != "" {
		hints = append(hints, key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", b.def.Child())))
	}
	hints = append(hints,
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "describe")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort")),
		key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "favorite")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	)
	if _, ok := b.def.(resource.Tagger); ok {
		hints = append(hints, key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tags")))
	}
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
		if b.tagMode {
			b.handleTagKey(kmsg.String())
			return b, nil
		}
		return b.handleKey(kmsg)
	}
	return b, nil
}

// handleTagKey drives the tag-filter popup.
func (b *Browser) handleTagKey(key string) {
	switch key {
	case "j", "down":
		if b.tagCursor < len(b.tagOptions)-1 {
			b.tagCursor++
		}
	case "k", "up":
		if b.tagCursor > 0 {
			b.tagCursor--
		}
	case "space", "enter":
		if len(b.tagOptions) > 0 {
			tag := b.tagOptions[b.tagCursor]
			if b.tagSelected[tag] {
				delete(b.tagSelected, tag)
			} else {
				b.tagSelected[tag] = true
			}
			b.refreshTable() // live effect while the popup is open
		}
	case "c":
		clear(b.tagSelected)
		b.refreshTable()
	case "esc", "t", "q":
		b.tagMode = false
	}
}

// openTagPopup collects the distinct tags across current rows.
func (b *Browser) openTagPopup(tagger resource.Tagger) tea.Cmd {
	seen := map[string]bool{}
	options := []string{}
	for _, r := range b.allRows {
		for _, tag := range tagger.RowTags(r) {
			if !seen[tag] {
				seen[tag] = true
				options = append(options, tag)
			}
		}
	}
	sort.Strings(options)
	if len(options) == 0 {
		return func() tea.Msg { return FlashMsg{Level: FlashInfo, Text: "no tags on " + b.def.Name()} }
	}
	b.tagOptions = options
	b.tagCursor = 0
	if b.tagSelected == nil {
		b.tagSelected = map[string]bool{}
	}
	b.tagMode = true
	return nil
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
		if len(b.tagSelected) > 0 {
			clear(b.tagSelected)
			b.refreshTable()
			return b, func() tea.Msg { return FlashMsg{Level: FlashInfo, Text: "tag filter cleared"} }
		}
		return b, func() tea.Msg { return PopMsg{} }
	case "t":
		if tagger, ok := b.def.(resource.Tagger); ok {
			cmd := b.openTagPopup(tagger)
			return b, cmd
		}
	case "r", "ctrl+r":
		b.eng.RefreshNow(b.key)
		return b, func() tea.Msg { return FlashMsg{Level: FlashInfo, Text: "refreshing…"} }
	case "L":
		if b.err != nil {
			cmd := b.login()
			return b, cmd
		}
	case "f":
		if b.favs == nil {
			break
		}
		row, ok := b.table.SelectedRow()
		if !ok {
			return b, nil
		}
		starred := b.favs.Toggle(b.clients.Profile().Name, b.favKey(), row.ID)
		b.refreshTable()
		text := "unfavorited " + row.ID
		if starred {
			text = "favorited " + row.ID
		}
		return b, func() tea.Msg { return FlashMsg{Level: FlashInfo, Text: text} }
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

// drillDown pushes the child browser for the selected row; Opener defs emit
// their own message instead (e.g. tables open the tabbed view), and leaves
// fall back to describe so Enter always does something.
func (b *Browser) drillDown() tea.Cmd {
	row, ok := b.table.SelectedRow()
	if !ok {
		return nil
	}
	if opener, isOpener := b.def.(resource.Opener); isOpener {
		clients, scope := b.clients, b.scope
		return func() tea.Msg { return opener.EnterMsg(clients, scope, row) }
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
	// Disk-cached rows written by an older binary can have a different
	// column count than the current def; rendering them would misalign
	// every cell. Skip them and wait for the fresh fetch.
	if ev.Stale && len(ev.Rows) > 0 && len(ev.Rows[0].Cells) != len(b.def.Columns()) {
		return
	}
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
	if len(b.tagSelected) > 0 {
		if tagger, ok := b.def.(resource.Tagger); ok {
			filtered = filterByTags(filtered, tagger, b.tagSelected)
		}
	}
	if b.favs == nil {
		b.table.SetData(b.def.Columns(), filtered)
		return
	}

	// Inject the favorites column and float starred rows to the top; both
	// groups keep their supplied order (name-sorted from the DAO), giving
	// the favorite-then-name default. Explicit sort mode still overrides.
	profile, key := b.clients.Profile().Name, b.favKey()
	starred := make([]resource.Row, 0, len(filtered))
	rest := make([]resource.Row, 0, len(filtered))
	for _, r := range filtered {
		marker := ""
		if b.favs.IsFavorite(profile, key, r.ID) {
			marker = favMarker
		}
		augmented := resource.Row{
			ID:    r.ID,
			Cells: append([]string{marker}, r.Cells...),
			Data:  r.Data,
		}
		if marker != "" {
			starred = append(starred, augmented)
		} else {
			rest = append(rest, augmented)
		}
	}
	cols := append([]resource.Column{{Title: favMarker, Width: 3}}, b.def.Columns()...)
	b.table.SetData(cols, append(starred, rest...))
}

// filterByTags keeps rows carrying ALL selected tags.
func filterByTags(rows []resource.Row, tagger resource.Tagger, selected map[string]bool) []resource.Row {
	kept := make([]resource.Row, 0, len(rows))
	for _, r := range rows {
		tags := map[string]bool{}
		for _, tag := range tagger.RowTags(r) {
			tags[tag] = true
		}
		all := true
		for want := range selected {
			if !tags[want] {
				all = false
				break
			}
		}
		if all {
			kept = append(kept, r)
		}
	}
	return kept
}

// renderTagPopup draws the tag-filter panel shown above the table.
func (b *Browser) renderTagPopup(width int) string {
	var lines []string
	lines = append(lines, b.th.Title.Render("filter by tags")+
		b.th.Subtle.Render("  space toggle · c clear · esc close"))
	for i, tag := range b.tagOptions {
		marker := "[ ]"
		style := b.th.KeyLabel
		if b.tagSelected[tag] {
			marker = "[x]"
			style = b.th.KeyHint
		}
		line := " " + marker + " " + tag
		if i == b.tagCursor {
			line = b.th.Title.Render("▶") + line[1:]
		}
		lines = append(lines, style.Render(line))
	}
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(b.th.Accent).
		Padding(0, 1).
		MaxWidth(width).
		Render(strings.Join(lines, "\n"))
	return panel
}

// tagStatus is the active-tags indicator shown above the table.
func (b *Browser) tagStatus() string {
	if len(b.tagSelected) == 0 {
		return ""
	}
	tags := make([]string, 0, len(b.tagSelected))
	for tag := range b.tagSelected {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return b.th.KeyHint.Render("⚑ " + strings.Join(tags, ", "))
}

// Render draws the (optional) filter bar, tag popup, the table, and error
// states.
func (b *Browser) Render(width, height int) string {
	b.width, b.height = width, height

	var top string
	tableHeight := height
	if b.filtering || b.filterVal != "" {
		top = b.filter.View(b.th, width) + "\n"
		tableHeight--
	}
	if b.tagMode {
		popup := b.renderTagPopup(width)
		top += popup + "\n"
		tableHeight -= lipgloss.Height(popup) + 1
	} else if ts := b.tagStatus(); ts != "" {
		top += ts + "\n"
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
