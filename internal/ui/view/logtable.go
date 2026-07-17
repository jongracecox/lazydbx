package view

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/theme"
	"github.com/jongracecox/lazydbx/internal/ui/component"
)

// Column widths for the collapsed log line; MESSAGE takes the rest and
// truncates (the full text lives on the record for filtering and drill-down).
const (
	logTimeW = 8
	logSevW  = 8
	logColW  = logTimeW + logSevW + 4 // two 2-space separators
)

// LogTable shows structured log records one collapsed line each: severity is
// colored, MESSAGE is truncated to the width (the full text is retained for
// filtering and drill-down), `/` filters across the whole record, `enter`
// expands the selected record to its full payload, and follow re-fetches on a
// timer at an adjustable interval.
//
// It manages its own cursor and scroll (rather than a component.Table) so a log
// tail scrolls predictably: the highlight moves within the page and only scrolls
// when it would leave the top or bottom edge.
type LogTable struct {
	th    theme.Theme
	title string
	fetch func(ctx context.Context) ([]LogRecord, error)

	records   []LogRecord    // accumulated append-only across refreshes
	rows      []resource.Row // current display rows (records after the filter)
	seqOffset int            // records dropped off the front, for stable row IDs
	newRows   int            // rows added since the user last saw the bottom

	cursor int // index into rows
	top    int // first visible row index (scroll offset)
	viewH  int // visible row count from the last render

	loaded    bool
	err       error
	fetchedAt time.Time

	follow         bool
	followInterval time.Duration
	gen            int
	followGen      int
	everLoaded     bool
	// pendingBottom defers a jump-to-bottom until Render knows the real height.
	pendingBottom bool
	// fetching guards against overlapping fetches: an app-log drain can take
	// several seconds (longer than the follow interval), so a follow tick must
	// not start a second fetch that would supersede the first as stale.
	fetching bool

	filterOpen  bool
	filter      component.FilterBar
	filterQuery string

	width, height int
}

// NewLogTable builds the structured log-record view. When follow is true it
// tails, re-fetching every followInterval.
func NewLogTable(th theme.Theme, title string, fetch func(ctx context.Context) ([]LogRecord, error), follow bool) *LogTable {
	return &LogTable{
		th:             th,
		title:          title,
		fetch:          fetch,
		follow:         follow,
		followInterval: followDefault,
		filter:         component.NewFilterBar(),
	}
}

// Init kicks off the first fetch and, when following, arms the poll ticker.
func (v *LogTable) Init() tea.Cmd {
	cmds := []tea.Cmd{v.fetchCmd()}
	if v.follow {
		cmds = append(cmds, v.followTick())
	}
	return tea.Batch(cmds...)
}

// Close implements View.
func (v *LogTable) Close() {}

// Title implements View.
func (v *LogTable) Title() string { return v.title }

// CapturesKeys reports true while the filter prompt is open, so global
// shortcuts don't steal typed characters.
func (v *LogTable) CapturesKeys() bool { return v.filterOpen }

// Hints implements View.
func (v *LogTable) Hints() []key.Binding {
	followHelp := "follow(off)"
	if v.follow {
		followHelp = "follow(on " + fmtFollowInterval(v.followInterval) + ")"
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("f"), key.WithHelp("f", followHelp)),
		key.NewBinding(key.WithKeys("+"), key.WithHelp("+/-", "poll interval")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}
}

// Update routes lifecycle messages first, then keys.
func (v *LogTable) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case logRecordsLoadedMsg:
		return v.onLoaded(msg)
	case logTableTickMsg:
		return v.onTick(msg)
	case tea.KeyPressMsg:
		return v.handleKey(msg)
	}
	return v, nil
}

func (v *LogTable) handleKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	if v.filterOpen {
		return v.handleFilterKey(msg)
	}

	switch msg.String() {
	case "f":
		return v.toggleFollow()
	case "+", "=":
		v.adjustInterval(time.Second)
		return v, nil
	case "-", "_":
		v.adjustInterval(-time.Second)
		return v, nil
	case "r", "ctrl+r":
		cmd := v.fetchCmd()
		return v, cmd
	case "/":
		v.filterOpen = true
		var cmd tea.Cmd
		v.filter, cmd = v.filter.Open(v.filterQuery)
		return v, cmd
	case "enter":
		cmd := v.expandSelected()
		return v, cmd
	case "esc":
		if v.filterQuery != "" {
			v.filterQuery = ""
			v.applyRows()
			return v, nil
		}
		return v, func() tea.Msg { return PopMsg{} }
	case "up", "k":
		v.moveCursor(-1)
	case "down", "j":
		v.moveCursor(1)
	case "pgup", "b":
		v.moveCursor(-v.pageStep())
	case "pgdown", " ":
		v.moveCursor(v.pageStep())
	case "home", "g":
		v.moveCursorTo(0)
	case "end", "G":
		v.moveCursorTo(len(v.rows) - 1)
	}
	return v, nil
}

func (v *LogTable) handleFilterKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	var event component.Event
	var cmd tea.Cmd
	v.filter, event, cmd = v.filter.Update(msg)
	switch event.Kind {
	case component.EventSubmit, component.EventCancel, component.EventChanged:
		if event.Kind != component.EventChanged {
			v.filterOpen = false
		}
		v.filterQuery = event.Value
		v.applyRows()
	case component.EventNone:
	}
	return v, cmd
}

func (v *LogTable) toggleFollow() (View, tea.Cmd) {
	v.follow = !v.follow
	v.followGen++
	if v.follow {
		return v, tea.Batch(v.fetchCmd(), v.followTick())
	}
	return v, nil
}

// adjustInterval changes the poll cadence by delta, clamped to [followMin, followMax].
func (v *LogTable) adjustInterval(delta time.Duration) {
	v.followInterval += delta
	switch {
	case v.followInterval < followMin:
		v.followInterval = followMin
	case v.followInterval > followMax:
		v.followInterval = followMax
	}
}

// pageStep is the row jump for page up/down (one screen, min 1).
func (v *LogTable) pageStep() int {
	if v.viewH > 1 {
		return v.viewH - 1
	}
	return 1
}

// moveCursor shifts the cursor by delta rows; moveCursorTo jumps to an index.
func (v *LogTable) moveCursor(delta int) { v.moveCursorTo(v.cursor + delta) }

func (v *LogTable) moveCursorTo(i int) {
	if len(v.rows) == 0 {
		v.cursor = 0
		return
	}
	v.cursor = min(max(i, 0), len(v.rows)-1)
	v.ensureVisible()
	if v.atBottom() {
		v.newRows = 0
	}
}

// ensureVisible scrolls the minimum amount to keep the cursor on screen — so
// movement only scrolls once the cursor reaches an edge.
func (v *LogTable) ensureVisible() {
	if v.viewH <= 0 {
		return
	}
	if v.cursor < v.top {
		v.top = v.cursor
	} else if v.cursor >= v.top+v.viewH {
		v.top = v.cursor - v.viewH + 1
	}
	v.clampTop()
}

func (v *LogTable) clampTop() {
	maxTop := max(0, len(v.rows)-v.viewH)
	v.top = min(max(v.top, 0), maxTop)
}

func (v *LogTable) atBottom() bool {
	return len(v.rows) == 0 || v.cursor == len(v.rows)-1
}

// expandSelected pushes a scrollable view of the selected record's full payload.
func (v *LogTable) expandSelected() tea.Cmd {
	if v.cursor < 0 || v.cursor >= len(v.rows) {
		return nil
	}
	rec, ok := v.rows[v.cursor].Data.(LogRecord)
	if !ok {
		return nil
	}
	detail := recordDetail(rec)
	title := "record"
	if lvl := displayLevel(rec); lvl != "" {
		title = "record/" + strings.ToLower(lvl)
	}
	return func() tea.Msg {
		return PushMsg{View: NewLogView(v.th, title, func(context.Context) (string, error) { return detail, nil }, false)}
	}
}

// fetchCmd runs the fetch under a timeout, tagged with a fresh gen.
func (v *LogTable) fetchCmd() tea.Cmd {
	v.gen++
	v.fetching = true
	gen, fetch := v.gen, v.fetch
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		records, err := fetch(ctx)
		return logRecordsLoadedMsg{records: records, err: err, gen: gen}
	}
}

func (v *LogTable) onLoaded(msg logRecordsLoadedMsg) (View, tea.Cmd) {
	if msg.gen != v.gen {
		return v, nil
	}
	v.fetching = false
	v.loaded = true
	v.fetchedAt = time.Now()
	if msg.err != nil {
		v.err = msg.err
		return v, nil
	}
	v.err = nil

	// Was the user parked at the newest row before this refresh? If so we keep
	// tailing; if they'd scrolled up, we leave their position and just count the
	// new arrivals for the "new rows" marker.
	wasAtBottom := !v.everLoaded || v.atBottom()

	merged, added := mergeLogRecords(v.records, msg.records)
	if drop := len(merged) - maxLogRecords; drop > 0 {
		merged = merged[drop:]
		v.seqOffset += drop
	}
	v.records = merged
	v.applyRows()

	if wasAtBottom {
		v.pendingBottom = true // jump-to-bottom deferred to Render (needs height)
		v.newRows = 0
	} else {
		v.newRows += added
	}
	v.everLoaded = true
	return v, nil
}

// maxLogRecords bounds the accumulated buffer; older records fall off the front.
const maxLogRecords = 5000

// applyRows rebuilds the display rows from records, honoring the active filter
// and preserving the cursor on the same record (by stable ID) across refreshes.
func (v *LogTable) applyRows() {
	var selectedID string
	if v.cursor >= 0 && v.cursor < len(v.rows) {
		selectedID = v.rows[v.cursor].ID
	}

	q := strings.ToLower(strings.TrimSpace(v.filterQuery))
	rows := make([]resource.Row, 0, len(v.records))
	for i := range v.records {
		rec := v.records[i]
		if q != "" && !strings.Contains(recordSearchText(rec), q) {
			continue
		}
		rows = append(rows, resource.Row{
			ID:    strconv.Itoa(v.seqOffset + i),
			Cells: []string{formatLogTime(rec.Time), displayLevel(rec), cleanLogMessage(rec.Message)},
			Data:  rec,
		})
	}
	v.rows = rows

	v.cursor = 0
	if selectedID != "" {
		for i, r := range rows {
			if r.ID == selectedID {
				v.cursor = i
				break
			}
		}
	}
	if v.cursor >= len(rows) {
		v.cursor = max(0, len(rows)-1)
	}
	v.ensureVisible()
}

// followTick arms the poll timer, reading followInterval at each tick.
func (v *LogTable) followTick() tea.Cmd {
	g := v.followGen
	return tea.Tick(v.followInterval, func(time.Time) tea.Msg { return logTableTickMsg{gen: g} })
}

func (v *LogTable) onTick(msg logTableTickMsg) (View, tea.Cmd) {
	if !v.follow || msg.gen != v.followGen {
		return v, nil
	}
	// Overlap-drop: if a fetch is still running (drains can outlast the
	// interval), just re-arm the timer — starting another would bump gen and
	// discard the in-flight result as stale, so nothing would ever load.
	if v.fetching {
		tick := v.followTick()
		return v, tick
	}
	return v, tea.Batch(v.fetchCmd(), v.followTick())
}

// Render draws the filter prompt (when open), the log rows, and the new-rows
// marker.
func (v *LogTable) Render(width, height int) string {
	v.width, v.height = width, height

	var top string
	bodyHeight := height
	if v.filterOpen {
		top = v.filter.View(v.th, width) + "\n"
		bodyHeight--
	}

	// A "new rows" marker sits below the list while the user is scrolled up and
	// fresh records have arrived (follow keeps counting without yanking them).
	showMarker := v.newRows > 0 && !v.atBottom()
	var bottom string
	if showMarker {
		bottom = "\n" + v.th.KeyHint.Render(fmt.Sprintf(" ↓ %d new %s ", v.newRows, plural("row", v.newRows)))
		bodyHeight--
	}
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	switch {
	case !v.loaded && v.err == nil:
		return top + v.th.Subtle.Render("loading…")
	case v.err != nil && len(v.records) == 0:
		return top + v.th.Error.Render("error: "+v.err.Error()) + "\n\n" +
			v.th.Subtle.Render("r to retry, esc to go back")
	case v.loaded && len(v.records) == 0:
		return top + v.th.Subtle.Render("(no logs)")
	case len(v.rows) == 0 && v.filterQuery != "":
		return top + v.th.Subtle.Render("no records match "+strconv.Quote(v.filterQuery))
	}

	// viewH is the row area below the header line.
	v.viewH = max(1, bodyHeight-1)
	if v.pendingBottom {
		v.cursor = len(v.rows) - 1
		v.pendingBottom = false
	}
	v.ensureVisible()
	if v.atBottom() {
		v.newRows = 0
	}

	msgW := max(10, width-logColW)
	lines := make([]string, 0, v.viewH+1)
	lines = append(lines, v.th.Subtle.Render(padCell("TIME", logTimeW)+"  "+padCell("SEV", logSevW)+"  MESSAGE"))
	end := min(v.top+v.viewH, len(v.rows))
	for i := v.top; i < end; i++ {
		lines = append(lines, v.renderRow(v.rows[i], i == v.cursor, msgW))
	}
	return top + strings.Join(lines, "\n") + bottom
}

// renderRow formats one log line. The selected row is highlighted whole (plain
// text under a reverse style); other rows color the SEV column by level.
func (v *LogTable) renderRow(r resource.Row, selected bool, msgW int) string {
	timeCell := padCell(cell(r, 0), logTimeW)
	sevCell := padCell(cell(r, 1), logSevW)
	msgCell := padCell(ansi.Truncate(cell(r, 2), msgW, "…"), msgW)
	if selected {
		return lipgloss.NewStyle().Reverse(true).Render(timeCell + "  " + sevCell + "  " + msgCell)
	}
	sev := sevCell
	if st, ok := v.sevStyle(cell(r, 1)); ok {
		sev = st.Render(sevCell)
	}
	return v.th.Subtle.Render(timeCell) + "  " + sev + "  " + msgCell
}

// sevStyle maps a level to its theme style; ok is false for neutral levels.
func (v *LogTable) sevStyle(level string) (lipgloss.Style, bool) {
	switch logLevelClass(level) {
	case resource.CellBad:
		return v.th.Error, true
	case resource.CellWarn:
		return v.th.Warning, true
	case resource.CellGood:
		return v.th.Success, true
	default:
		return lipgloss.Style{}, false
	}
}

// Status renders the right status segment: record count, filter, follow, age.
func (v *LogTable) Status(now time.Time) string {
	if !v.loaded {
		return ""
	}
	parts := []string{strconv.Itoa(len(v.records)) + " records"}
	if v.filterQuery != "" {
		parts = append(parts, fmt.Sprintf("filter %q (%d)", v.filterQuery, len(v.rows)))
	}
	if v.follow {
		parts = append(parts, v.th.Warning.Render("following "+fmtFollowInterval(v.followInterval)))
	}
	parts = append(parts, fmt.Sprintf("⟳ %s ago", now.Sub(v.fetchedAt).Round(time.Second)))
	line := strings.Join(parts, "  ")
	if v.err != nil {
		return v.th.Error.Render(line + "  (refresh failed)")
	}
	return v.th.Subtle.Render(line)
}

// --- helpers ---

// cell returns column i of a row, or "" when absent.
func cell(r resource.Row, i int) string {
	if i < len(r.Cells) {
		return r.Cells[i]
	}
	return ""
}

// pad truncates s to w (with an ellipsis) and right-pads it to width w.
func padCell(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if gap := w - ansi.StringWidth(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}

// plural returns word or its simple plural for n.
func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// mergeLogRecords appends the genuinely new records from incoming (the latest
// buffer) onto existing (everything seen so far), aligning on the overlap so a
// sliding server buffer neither duplicates lines nor shifts stable row IDs.
// added is how many records were appended.
func mergeLogRecords(existing, incoming []LogRecord) (merged []LogRecord, added int) {
	if len(existing) == 0 {
		return incoming, len(incoming)
	}
	if len(incoming) == 0 {
		return existing, 0
	}
	maxK := min(len(incoming), len(existing))
	for k := maxK; k > 0; k-- {
		if logRecordsEqual(existing[len(existing)-k:], incoming[:k]) {
			extra := incoming[k:]
			return append(existing, extra...), len(extra)
		}
	}
	// No overlap: the buffer rotated past everything we have — append it all.
	return append(existing, incoming...), len(incoming)
}

func logRecordsEqual(a, b []LogRecord) bool {
	for i := range a {
		if !sameLogRecord(a[i], b[i]) {
			return false
		}
	}
	return true
}

// sameLogRecord compares records for merge alignment, preferring the exact raw
// frame (unique per line, timestamp included) and falling back to message+time.
func sameLogRecord(a, b LogRecord) bool {
	if a.Raw != "" || b.Raw != "" {
		return a.Raw == b.Raw
	}
	return a.Message == b.Message && a.Time.Equal(b.Time)
}

// collapseMessage folds a multi-line message into a single trimmed line,
// collapsing every run of whitespace (including newlines) to one space.
func collapseMessage(msg string) string {
	return strings.Join(strings.Fields(msg), " ")
}

// cleanLogMessage collapses the message and strips the level word and the app's
// own leading "[date time]" prefix — both already shown in their own columns —
// so the MESSAGE column isn't redundant. The full original is kept on the
// record for filtering and the expanded view.
func cleanLogMessage(msg string) string {
	s := collapseMessage(msg)
	for {
		before := s
		s = strings.TrimSpace(stripLeadingTimestamp(s))
		s = strings.TrimSpace(stripLeadingLevel(s))
		if s == before {
			return s
		}
	}
}

// stripLeadingTimestamp removes a leading "[...]" group when it looks like a
// date/time (contains a digit and a `:`/`/`/`-`), else leaves the text intact.
func stripLeadingTimestamp(s string) string {
	if !strings.HasPrefix(s, "[") {
		return s
	}
	end := strings.IndexByte(s, ']')
	if end < 0 {
		return s
	}
	inner := s[1:end]
	if strings.ContainsAny(inner, "0123456789") && strings.ContainsAny(inner, ":/-") {
		return s[end+1:]
	}
	return s
}

// stripLeadingLevel removes a leading level word (optionally with a trailing
// colon) when it's a recognized level, else leaves the text intact.
func stripLeadingLevel(s string) string {
	i := strings.IndexAny(s, " :")
	if i < 0 {
		i = len(s)
	}
	if logLevels[strings.ToUpper(s[:i])] {
		return strings.TrimPrefix(s[i:], ":")
	}
	return s
}

// logLevels is the set of recognized level keywords, shared by detection,
// coloring, and message cleaning.
var logLevels = map[string]bool{
	"TRACE": true, "DEBUG": true, "INFO": true, "NOTICE": true,
	"WARN": true, "WARNING": true, "ERROR": true, "ERR": true,
	"CRITICAL": true, "CRIT": true, "FATAL": true, "SEVERE": true,
}

// formatLogTime renders a record time as HH:MM:SS, empty when unset.
func formatLogTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("15:04:05")
}

// displayLevel is the severity to show/color: the structured severity when
// meaningful, else a level word detected at the start of the message.
func displayLevel(rec LogRecord) string {
	s := strings.ToUpper(strings.TrimSpace(rec.Severity))
	if s != "" && s != "UNKNOWN" {
		return s
	}
	return detectLevel(rec.Message)
}

// detectLevel returns a known level keyword found among the first few tokens of
// the message, else "". App logs often bake the level into the message text
// (after their own "[date time]" prefix) while leaving the structured severity
// UNKNOWN, so we scan a short window rather than just the first token.
func detectLevel(msg string) string {
	fields := strings.Fields(msg)
	if len(fields) > 5 {
		fields = fields[:5]
	}
	for _, f := range fields {
		if up := strings.ToUpper(strings.Trim(f, "[]():")); logLevels[up] {
			return up
		}
	}
	return ""
}

// logLevelClass maps a level to a semantic cell class for coloring.
func logLevelClass(level string) resource.CellClass {
	switch strings.ToUpper(level) {
	case "ERROR", "ERR", "CRITICAL", "CRIT", "FATAL", "SEVERE":
		return resource.CellBad
	case "WARN", "WARNING":
		return resource.CellWarn
	case "INFO", "NOTICE":
		return resource.CellGood
	default:
		return resource.CellDefault
	}
}

// recordSearchText is the lowercased haystack the filter matches — every field,
// so filtering finds text hidden by the collapsed/truncated display.
func recordSearchText(rec LogRecord) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(rec.Severity))
	b.WriteByte(' ')
	b.WriteString(strings.ToLower(rec.Source))
	b.WriteByte(' ')
	b.WriteString(strings.ToLower(rec.Message))
	b.WriteByte(' ')
	b.WriteString(formatLogTime(rec.Time))
	return b.String()
}

// recordDetail renders a record's full payload for the expanded view: pretty
// JSON of the raw frame when it is JSON, else the raw text, always followed by
// the full (uncollapsed) message so long lines are readable.
func recordDetail(rec LogRecord) string {
	var out strings.Builder
	if pretty, ok := prettyJSON(rec.Raw); ok {
		out.WriteString(pretty)
	} else if rec.Raw != "" {
		out.WriteString(rec.Raw)
	}
	if rec.Message != "" {
		if out.Len() > 0 {
			out.WriteString("\n\n── message ──\n\n")
		}
		out.WriteString(rec.Message)
	}
	return out.String()
}

// prettyJSON indents a JSON string; ok is false when s is not valid JSON.
func prettyJSON(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(s), "", "  "); err != nil {
		return "", false
	}
	return buf.String(), true
}

// --- private lifecycle messages ---

// logRecordsLoadedMsg carries the outcome of one fetch.
type logRecordsLoadedMsg struct {
	records []LogRecord
	err     error
	gen     int
}

// logTableTickMsg fires on the follow poll timer for a follow session.
type logTableTickMsg struct{ gen int }
