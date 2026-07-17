package view

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jongracecox/lazydbx/internal/config"
	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/theme"
	"github.com/jongracecox/lazydbx/internal/ui/component"
)

// SQLView is the SQL editor/preview screen: a textarea on top, a status line,
// and a scrollable results grid below. It serves both ad-hoc SQL (`:sql`) and
// table preview (OpenSQLMsg with Execute). All statement I/O happens inside
// tea.Cmds; Update stays pure and drives the lifecycle through the private
// messages defined at the bottom of this file.
type SQLView struct {
	th      theme.Theme
	clients *dbx.Clients
	sqlCfg  config.SQLConfig

	editor textarea.Model
	vp     viewport.Model
	vpOK   bool

	warehouses []dbx.Warehouse
	wh         dbx.Warehouse
	whOK       bool

	state     sqlState
	result    *dbx.StmtResult
	elapsed   time.Duration
	errMsg    string
	stmtID    string
	startedAt time.Time
	canceling bool

	// gen guards against stale polls: it increments on every execute, and
	// lifecycle messages carrying an older generation are dropped.
	gen int

	focus focusTarget

	// picker is the inline warehouse chooser; while open it owns the keyboard.
	pickerOpen bool
	picker     component.Table

	// autoExec runs the query once warehouses resolve on Init.
	autoExec bool

	// gridLines is the fully rendered (styled) results grid; xoff is the
	// horizontal scroll offset applied at render time via ansi.Cut.
	gridLines []string
	gridWidth int
	xoff      int

	width, height int
}

type sqlState int

const (
	stateIdle sqlState = iota
	statePending
	stateRunning
	stateSucceeded
	stateFailed
	stateCanceled
)

type focusTarget int

const (
	focusEditor focusTarget = iota
	focusResults
)

// maxCell caps the width of any results cell (and thus column) in characters.
const maxCell = 40

// warehouseCols describes the inline warehouse picker table.
var warehouseCols = []resource.ColSpec[dbx.Warehouse]{
	{Column: resource.Column{Title: "NAME"}, Extract: func(w dbx.Warehouse) string { return w.Name }},
	{Column: resource.Column{Title: "STATE", Width: 12}, Extract: func(w dbx.Warehouse) string { return w.State }},
	{Column: resource.Column{Title: "SIZE", Width: 12}, Extract: func(w dbx.Warehouse) string { return w.Size }},
	{Column: resource.Column{Title: "TYPE", Width: 12}, Extract: warehouseType},
}

func warehouseType(w dbx.Warehouse) string {
	if w.Serverless {
		return "serverless"
	}
	return "classic"
}

// NewSQLView builds the view pre-filled with query. When autoExec is true the
// statement runs as soon as a warehouse resolves on Init.
func NewSQLView(th theme.Theme, clients *dbx.Clients, sqlCfg config.SQLConfig, query string, autoExec bool) *SQLView {
	ta := textarea.New()
	ta.SetValue(query)
	ta.CursorEnd()
	// Land focused on the editor (cursor in the query) whether this is an
	// ad-hoc editor or an auto-exec preview — so arriving on the SQL/data
	// screen starts on the SQL. tab/shift+tab then step to the results and back
	// (standalone via toggleFocus, inside a Tabbed via AdvanceFocus). Since the
	// container owns tab now, typed keys no longer risk switching tabs.
	ta.Focus()
	return &SQLView{
		th:       th,
		clients:  clients,
		sqlCfg:   sqlCfg,
		editor:   ta,
		state:    stateIdle,
		focus:    focusEditor,
		autoExec: autoExec,
	}
}

// Init loads the warehouse list (and focuses the editor for ad-hoc use).
func (v *SQLView) Init() tea.Cmd {
	if v.focus == focusEditor {
		return tea.Batch(v.editor.Focus(), v.loadWarehouses())
	}
	return v.loadWarehouses()
}

// Close implements View.
func (v *SQLView) Close() {}

// Title implements View.
func (v *SQLView) Title() string { return "sql" }

// CapturesKeys reports whether the view needs first refusal on key input so
// the app's global shortcuts (`:`, `?`, `q`, ...) don't steal characters. It
// is true while the editor is focused (the user is typing) or the warehouse
// picker is open. See app.handleKey for the routing this drives.
func (v *SQLView) CapturesKeys() bool {
	return v.pickerOpen || v.focus == focusEditor
}

// Hints implements View.
func (v *SQLView) Hints() []key.Binding {
	if v.pickerOpen {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select warehouse")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	}
	hints := []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "editor/results")),
		key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl-e", "execute")),
		key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("ctrl-k", "cancel")),
		key.NewBinding(key.WithKeys("ctrl+w"), key.WithHelp("ctrl-w", "warehouse")),
	}
	if v.focus == focusResults {
		hints = append(hints,
			key.NewBinding(key.WithKeys("j"), key.WithHelp("j/k", "scroll")),
			key.NewBinding(key.WithKeys("h"), key.WithHelp("h/l", "scroll ↔")),
		)
	}
	return hints
}

// Update routes messages: lifecycle messages first, then keys (picker owns
// them while open, otherwise the editor/results split handles them).
func (v *SQLView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case whLoadedMsg:
		return v.onWarehousesLoaded(msg)
	case stmtStartedMsg:
		return v.onStmtStarted(msg)
	case pollMsg:
		return v.onPollTick(msg)
	case pollDoneMsg:
		return v.onPollDone(msg)
	case cancelDoneMsg:
		if msg.err != nil {
			return v, flash(FlashError, "cancel: "+msg.err.Error())
		}
		return v, nil
	case tea.KeyPressMsg:
		return v.handleKey(msg)
	}
	return v, nil
}

func (v *SQLView) handleKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	if v.pickerOpen {
		return v.handlePickerKey(msg)
	}

	switch msg.String() {
	case "tab":
		v.toggleFocus()
		return v, nil
	case "ctrl+e":
		return v.execute()
	case "ctrl+k":
		return v.cancel()
	case "ctrl+w":
		return v.openPicker()
	case "esc":
		if v.focus == focusEditor && v.isRunning() {
			return v.cancel()
		}
		return v, func() tea.Msg { return PopMsg{} }
	}

	if v.focus == focusEditor {
		var cmd tea.Cmd
		v.editor, cmd = v.editor.Update(msg)
		return v, cmd
	}

	// Results focused: horizontal scroll on our own offset, everything else
	// (j/k/pgup/pgdn) to the viewport.
	switch msg.String() {
	case "h", "left":
		if v.xoff > 0 {
			v.xoff--
		}
		return v, nil
	case "l", "right":
		if v.xoff < v.maxXOff() {
			v.xoff++
		}
		return v, nil
	}
	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	return v, cmd
}

func (v *SQLView) handlePickerKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if row, ok := v.picker.SelectedRow(); ok {
			v.wh = row.Data.(dbx.Warehouse)
			v.whOK = true
		}
		v.pickerOpen = false
		return v, nil
	case "esc":
		v.pickerOpen = false
		return v, nil
	}
	var cmd tea.Cmd
	v.picker, cmd = v.picker.Update(msg)
	return v, cmd
}

// setFocus moves focus to f and syncs the editor's focus state so it only
// draws its cursor / consumes typing while selected.
func (v *SQLView) setFocus(f focusTarget) {
	v.focus = f
	if f == focusEditor {
		v.editor.Focus()
	} else {
		v.editor.Blur()
	}
}

// toggleFocus flips between the editor and results panes. It drives tab when
// this view is used standalone (`:sql`, OpenSQLMsg); inside a Tabbed container
// the AdvanceFocus/EnterFocus cycle takes over instead.
func (v *SQLView) toggleFocus() {
	if v.focus == focusEditor {
		v.setFocus(focusResults)
	} else {
		v.setFocus(focusEditor)
	}
}

// AdvanceFocus makes the editor and results panes act as stops in the global
// tab cycle (see view.TabCycler). Moving forward from the editor lands on the
// results; moving back from the results lands on the editor. At the boundary
// (results going forward, editor going back) it returns false so the container
// switches tabs. While the warehouse picker is open it consumes the key so the
// cycle can't slip past an open modal.
func (v *SQLView) AdvanceFocus(forward bool) bool {
	if v.pickerOpen {
		return true
	}
	if forward {
		if v.focus == focusEditor {
			v.setFocus(focusResults)
			return true
		}
		return false
	}
	if v.focus == focusResults {
		v.setFocus(focusEditor)
		return true
	}
	return false
}

// EnterFocus places focus at the pane the cycle arrives on: the editor when
// entering forward (its first stop), the results when entering backward.
func (v *SQLView) EnterFocus(forward bool) {
	if forward {
		v.setFocus(focusEditor)
	} else {
		v.setFocus(focusResults)
	}
}

func (v *SQLView) openPicker() (View, tea.Cmd) {
	v.picker = component.NewTable(v.th)
	v.picker.SetData(resource.Cols(warehouseCols),
		resource.BuildRows(v.warehouses, func(w dbx.Warehouse) string { return w.ID }, warehouseCols))
	v.pickerOpen = true
	return v, nil
}

func (v *SQLView) isRunning() bool {
	return v.state == statePending || v.state == stateRunning
}

// loadWarehouses fetches the warehouse list and resolves the default.
func (v *SQLView) loadWarehouses() tea.Cmd {
	clients, cfgID := v.clients, v.sqlCfg.WarehouseID
	return func() tea.Msg {
		dao, err := clients.Warehouses()
		if err != nil {
			return whLoadedMsg{err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		list, err := dao.List(ctx)
		if err != nil {
			return whLoadedMsg{err: err}
		}
		wh, ok := dbx.PickWarehouse(cfgID, list)
		return whLoadedMsg{list: list, wh: wh, ok: ok}
	}
}

func (v *SQLView) onWarehousesLoaded(msg whLoadedMsg) (View, tea.Cmd) {
	if msg.err != nil {
		return v, flash(FlashError, "warehouses: "+msg.err.Error())
	}
	v.warehouses = msg.list
	v.wh, v.whOK = msg.wh, msg.ok
	if v.autoExec {
		v.autoExec = false
		if v.whOK {
			return v.execute()
		}
	}
	return v, nil
}

// execute submits the current editor contents for asynchronous execution.
func (v *SQLView) execute() (View, tea.Cmd) {
	query := strings.TrimSpace(v.editor.Value())
	switch {
	case query == "":
		return v, flash(FlashWarn, "nothing to execute")
	case !v.whOK:
		return v, flash(FlashWarn, "no warehouse — ctrl+w to pick")
	case v.isRunning():
		return v, flash(FlashWarn, "statement already running")
	}

	v.gen++
	v.state = statePending
	v.result = nil
	v.errMsg = ""
	v.canceling = false
	v.xoff = 0

	gen := v.gen
	clients, whID, limit := v.clients, v.wh.ID, v.sqlCfg.RowLimit
	return v, func() tea.Msg {
		dao, err := clients.Statements()
		if err != nil {
			return stmtStartedMsg{gen: gen, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		id, err := dao.Submit(ctx, whID, query, limit)
		return stmtStartedMsg{gen: gen, id: id, startedAt: time.Now(), err: err}
	}
}

func (v *SQLView) onStmtStarted(msg stmtStartedMsg) (View, tea.Cmd) {
	if msg.gen != v.gen {
		return v, nil
	}
	if msg.err != nil {
		v.state = stateFailed
		v.errMsg = msg.err.Error()
		return v, nil
	}
	v.stmtID = msg.id
	v.startedAt = msg.startedAt
	v.state = stateRunning
	cmd := v.pollTick(msg.gen)
	return v, cmd
}

// pollTick arms the poll timer for a generation.
func (v *SQLView) pollTick(gen int) tea.Cmd {
	return tea.Tick(800*time.Millisecond, func(time.Time) tea.Msg { return pollMsg{gen: gen} })
}

func (v *SQLView) onPollTick(msg pollMsg) (View, tea.Cmd) {
	if msg.gen != v.gen || !v.isRunning() {
		return v, nil
	}
	gen, clients, id, startedAt := v.gen, v.clients, v.stmtID, v.startedAt
	return v, func() tea.Msg {
		dao, err := clients.Statements()
		if err != nil {
			return pollDoneMsg{gen: gen, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		poll, err := dao.Poll(ctx, id)
		return pollDoneMsg{gen: gen, poll: poll, err: err, elapsed: time.Since(startedAt)}
	}
}

func (v *SQLView) onPollDone(msg pollDoneMsg) (View, tea.Cmd) {
	if msg.gen != v.gen {
		return v, nil
	}
	if msg.err != nil {
		v.state = stateFailed
		v.errMsg = msg.err.Error()
		return v, nil
	}
	switch msg.poll.State {
	case dbx.StmtPending:
		v.state = statePending
		cmd := v.pollTick(msg.gen)
		return v, cmd
	case dbx.StmtRunning:
		v.state = stateRunning
		cmd := v.pollTick(msg.gen)
		return v, cmd
	case dbx.StmtSucceeded:
		v.state = stateSucceeded
		v.result = msg.poll.Result
		v.elapsed = msg.elapsed
		v.buildGrid()
		return v, nil
	case dbx.StmtFailed:
		v.state = stateFailed
		v.errMsg = msg.poll.Message
		return v, nil
	case dbx.StmtCanceled, dbx.StmtClosed:
		v.state = stateCanceled
		return v, nil
	}
	return v, nil
}

// cancel aborts the running statement; the poll loop observes CANCELED.
func (v *SQLView) cancel() (View, tea.Cmd) {
	if !v.isRunning() {
		return v, nil
	}
	v.canceling = true
	clients, id := v.clients, v.stmtID
	return v, func() tea.Msg {
		dao, err := clients.Statements()
		if err != nil {
			return cancelDoneMsg{err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return cancelDoneMsg{err: dao.Cancel(ctx, id)}
	}
}

// buildGrid renders the current result into styled, padded grid lines.
func (v *SQLView) buildGrid() {
	v.gridLines = nil
	v.gridWidth = 0
	v.xoff = 0
	if v.result == nil {
		return
	}
	cols := v.result.Columns
	rows := v.result.Rows

	// Column widths: max of header and every (flattened, truncated) cell,
	// capped at maxCell.
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = min(ansi.StringWidth(c.Name), maxCell)
	}
	cells := make([][]string, len(rows))
	for r, row := range rows {
		cells[r] = make([]string, len(cols))
		for i := range cols {
			var raw string
			if i < len(row) {
				raw = ansi.Truncate(dbx.OneLine(row[i]), maxCell, "…")
			}
			cells[r][i] = raw
			if w := ansi.StringWidth(raw); w > widths[i] {
				widths[i] = w
			}
		}
	}

	gutter := len(strconv.Itoa(len(rows)))
	if gutter < 1 {
		gutter = 1
	}

	// Header line.
	var hdr strings.Builder
	hdr.WriteString(strings.Repeat(" ", gutter))
	hdr.WriteString(" ")
	headerStyle := v.th.Title.Foreground(v.th.Accent)
	for i, c := range cols {
		if i > 0 {
			hdr.WriteString(" ")
		}
		hdr.WriteString(headerStyle.Render(pad(ansi.Truncate(c.Name, maxCell, "…"), widths[i])))
	}
	lines := []string{hdr.String()}

	for r, row := range cells {
		var b strings.Builder
		b.WriteString(v.th.Subtle.Render(pad(strconv.Itoa(r+1), gutter)))
		b.WriteString(" ")
		for i := range cols {
			if i > 0 {
				b.WriteString(" ")
			}
			b.WriteString(pad(row[i], widths[i]))
		}
		lines = append(lines, b.String())
	}

	v.gridLines = lines
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > v.gridWidth {
			v.gridWidth = w
		}
	}
}

func (v *SQLView) maxXOff() int {
	if v.gridWidth <= v.width || v.width <= 0 {
		return 0
	}
	return v.gridWidth - v.width
}

// pad right-pads s with spaces to display width w.
func pad(s string, w int) string {
	if gap := w - ansi.StringWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// Render draws the editor, status line, and results grid (or the warehouse
// picker when open).
func (v *SQLView) Render(width, height int) string {
	v.width, v.height = width, height
	if height < 5 {
		height = 5
	}

	editorHeight := max(3, height*35/100)
	resultsHeight := height - editorHeight - 1
	if resultsHeight < 1 {
		resultsHeight = 1
	}

	v.editor.SetWidth(width)
	v.editor.SetHeight(editorHeight)
	editor := v.editor.View()

	status := v.renderStatus(width)

	var bottom string
	if v.pickerOpen {
		bottom = v.renderPicker(width, resultsHeight)
	} else {
		bottom = v.renderResults(width, resultsHeight)
	}

	return editor + "\n" + status + "\n" + bottom
}

func (v *SQLView) renderPicker(width, height int) string {
	head := v.th.Subtle.Render("select warehouse — enter to use, esc to cancel")
	v.picker.SetSize(width, height-1)
	return head + "\n" + v.picker.View()
}

func (v *SQLView) renderResults(width, height int) string {
	if !v.vpOK {
		v.vp = viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
		v.vpOK = true
	} else {
		v.vp.SetWidth(width)
		v.vp.SetHeight(height)
	}

	if len(v.gridLines) == 0 {
		v.vp.SetContent(v.th.Subtle.Render("no results — press ctrl+e to execute"))
		return v.vp.View()
	}

	// Apply the horizontal offset by cutting each line to the visible window.
	windowed := make([]string, len(v.gridLines))
	for i, l := range v.gridLines {
		windowed[i] = ansi.Cut(l, v.xoff, v.xoff+width)
	}
	v.vp.SetContent(strings.Join(windowed, "\n"))
	return v.vp.View()
}

func (v *SQLView) renderStatus(width int) string {
	var whPart string
	if v.whOK {
		badge := fmt.Sprintf(" [%s·%s]", warehouseType(v.wh), v.wh.State)
		whPart = v.th.Subtle.Render("wh: ") + v.wh.Name + v.th.Subtle.Render(badge)
	} else {
		whPart = v.th.Warning.Render("no warehouse — ctrl+w to pick")
	}

	var statePart string
	switch v.state {
	case stateIdle:
		statePart = v.th.Subtle.Render("idle")
	case statePending:
		statePart = v.th.Subtle.Render("PENDING…")
	case stateRunning:
		if v.canceling {
			statePart = v.th.Warning.Render("canceling…")
		} else {
			statePart = v.th.Warning.Render("RUNNING…")
		}
	case stateFailed:
		statePart = v.th.Error.Render("error: " + dbx.OneLine(v.errMsg))
	case stateCanceled:
		statePart = v.th.Warning.Render("canceled")
	case stateSucceeded:
		n := 0
		if v.result != nil {
			n = len(v.result.Rows)
		}
		statePart = v.th.Success.Render(fmt.Sprintf("%d rows in %.1fs", n, v.elapsed.Seconds()))
		if v.result != nil && v.result.Truncated {
			statePart += "  " + v.th.Warning.Render(fmt.Sprintf("(truncated — showing first %d rows)", n))
		}
	}

	line := whPart + v.th.Subtle.Render("  •  ") + statePart
	if width > 0 {
		line = truncateToWidth(line, width)
	}
	return line
}

// truncateToWidth clips a styled line to a display width without wrapping.
func truncateToWidth(s string, width int) string {
	if ansi.StringWidth(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "…")
}

func flash(level component.FlashLevel, text string) tea.Cmd {
	return func() tea.Msg { return FlashMsg{Level: level, Text: text} }
}

// --- private lifecycle messages (see Update) ---

// whLoadedMsg carries the resolved warehouse list and default selection.
type whLoadedMsg struct {
	list []dbx.Warehouse
	wh   dbx.Warehouse
	ok   bool
	err  error
}

// stmtStartedMsg reports the outcome of Submit.
type stmtStartedMsg struct {
	gen       int
	id        string
	startedAt time.Time
	err       error
}

// pollMsg fires on the poll timer for a generation.
type pollMsg struct{ gen int }

// pollDoneMsg carries one Poll observation.
type pollDoneMsg struct {
	gen     int
	poll    dbx.StatementPoll
	err     error
	elapsed time.Duration
}

// cancelDoneMsg reports the outcome of Cancel.
type cancelDoneMsg struct{ err error }
