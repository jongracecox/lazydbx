package view

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/theme"
)

func updateLogTable(v View, msg tea.Msg) (*LogTable, tea.Cmd) {
	got, cmd := v.Update(msg)
	return got.(*LogTable), cmd
}

func newTestLogTable(records []LogRecord, follow bool) *LogTable {
	fetch := func(context.Context) ([]LogRecord, error) { return records, nil }
	return NewLogTable(theme.Default(), "logs/dash", fetch, follow)
}

func loadRecords(t *testing.T, v *LogTable) *LogTable {
	t.Helper()
	msg := runCmd(t, v.fetchCmd())
	v, _ = updateLogTable(v, msg)
	return v
}

func TestLogTableRendersCollapsed(t *testing.T) {
	v := newTestLogTable([]LogRecord{
		{Severity: "INFO", Message: "line one\n   with a wrapped continuation"},
		{Severity: "ERROR", Message: "boom"},
	}, false)
	assert.Contains(t, v.Render(120, 20), "loading", "loading before first fetch")

	v = loadRecords(t, v)
	out := v.Render(120, 20)
	assert.Contains(t, out, "line one with a wrapped continuation", "newlines collapsed to spaces")
	assert.Contains(t, out, "ERROR")
	assert.Contains(t, out, "boom")
}

func TestLogTableEmpty(t *testing.T) {
	v := loadRecords(t, newTestLogTable(nil, false))
	assert.Contains(t, v.Render(120, 20), "(no logs)")
}

func TestLogTableFilterMatchesFullRecord(t *testing.T) {
	// The needle sits deep in a long message, past where the column truncates,
	// and the source field isn't shown at all — both must still match.
	long := "INFO start " + strings.Repeat("padding ", 40) + "needle-in-message tail"
	v := loadRecords(t, newTestLogTable([]LogRecord{
		{Severity: "INFO", Message: long},
		{Severity: "INFO", Source: "worker-7", Message: "unrelated"},
		{Severity: "INFO", Message: "nothing here"},
	}, false))
	require.Len(t, v.rows, 3)

	v.filterQuery = "needle-in-message"
	v.applyRows()
	assert.Len(t, v.rows, 1, "matches text hidden past the truncation point")

	v.filterQuery = "worker-7"
	v.applyRows()
	assert.Len(t, v.rows, 1, "matches the source field, which has no column")

	v.filterQuery = "zzz-none"
	v.applyRows()
	assert.Empty(t, v.rows)
}

func TestLogTableEscClearsFilterThenPops(t *testing.T) {
	v := loadRecords(t, newTestLogTable([]LogRecord{{Message: "a"}, {Message: "b"}}, false))
	v.filterQuery = "a"
	v.applyRows()
	require.Len(t, v.rows, 1)

	// First esc clears the filter.
	v, cmd := updateLogTable(v, logKey("esc"))
	assert.Empty(t, v.filterQuery)
	assert.Len(t, v.rows, 2)
	assert.Nil(t, cmd)

	// Second esc pops the view.
	_, cmd = updateLogTable(v, logKey("esc"))
	msg := runCmd(t, cmd)
	_, ok := msg.(PopMsg)
	assert.True(t, ok)
}

func TestLogTableEnterExpands(t *testing.T) {
	v := loadRecords(t, newTestLogTable([]LogRecord{
		{Severity: "ERROR", Message: "kaboom", Raw: `{"severity":"ERROR","message":"kaboom"}`},
	}, false))
	v.Render(120, 20) // realize the table so a row is selected

	_, cmd := updateLogTable(v, logKey("enter"))
	msg := runCmd(t, cmd)
	push, ok := msg.(PushMsg)
	require.True(t, ok, "enter pushes a detail view")
	lv, ok := push.View.(*LogView)
	require.True(t, ok, "detail is a scrollable log view")
	assert.Equal(t, "record/error", lv.Title())

	detail := runCmd(t, lv.fetchCmd()).(logLoadedMsg)
	assert.Contains(t, detail.content, "kaboom", "shows the record payload")
	assert.Contains(t, detail.content, "\n  ", "raw JSON is pretty-printed")
}

func TestLogTableFollowIntervalAdjust(t *testing.T) {
	v := newTestLogTable([]LogRecord{{Message: "x"}}, true)
	assert.Equal(t, followDefault, v.followInterval)
	for i := 0; i < 10; i++ {
		v, _ = updateLogTable(v, logKey("-"))
	}
	assert.Equal(t, followMin, v.followInterval, "clamps at min")
	for i := 0; i < 200; i++ {
		v, _ = updateLogTable(v, logKey("+"))
	}
	assert.Equal(t, followMax, v.followInterval, "clamps at max")
}

func TestLogTableFollowOverlapDrop(t *testing.T) {
	// A slow drain can outlast the follow interval: a tick arriving mid-fetch
	// must not start a second fetch (which would bump gen and drop the first
	// result as stale) — the regression behind "logs don't load while following".
	v := newTestLogTable([]LogRecord{{Message: "hi"}}, true)
	cmd := v.fetchCmd() // simulate the in-flight first fetch
	require.NotNil(t, cmd)
	require.True(t, v.fetching)
	genBefore := v.gen

	// Tick while fetching → re-arm only, gen unchanged.
	v, tickCmd := updateLogTable(v, logTableTickMsg{gen: v.followGen})
	require.NotNil(t, tickCmd)
	assert.Equal(t, genBefore, v.gen, "no new fetch started while one is in flight")

	// The original fetch's result still matches and loads.
	v, _ = updateLogTable(v, logRecordsLoadedMsg{records: []LogRecord{{Message: "hi"}}, gen: genBefore})
	assert.True(t, v.loaded)
	assert.False(t, v.fetching)
	assert.Contains(t, v.Render(80, 10), "hi")
}

func TestMergeLogRecords(t *testing.T) {
	rec := func(id string) LogRecord { return LogRecord{Message: id, Raw: id} }
	existing := []LogRecord{rec("a"), rec("b"), rec("c")}

	// Overlapping sliding window: b,c carry over, d is new.
	merged, added := mergeLogRecords(existing, []LogRecord{rec("b"), rec("c"), rec("d")})
	assert.Equal(t, 1, added)
	assert.Equal(t, []string{"a", "b", "c", "d"}, rawIDs(merged))

	// First load.
	merged, added = mergeLogRecords(nil, []LogRecord{rec("a"), rec("b")})
	assert.Equal(t, 2, added)
	assert.Equal(t, []string{"a", "b"}, rawIDs(merged))

	// No overlap (full rotation) appends everything.
	merged, added = mergeLogRecords(existing, []LogRecord{rec("x"), rec("y")})
	assert.Equal(t, 2, added)
	assert.Equal(t, []string{"a", "b", "c", "x", "y"}, rawIDs(merged))

	// Nothing new.
	_, added = mergeLogRecords(existing, []LogRecord{rec("b"), rec("c")})
	assert.Equal(t, 0, added)
}

func rawIDs(recs []LogRecord) []string {
	ids := make([]string, len(recs))
	for i, r := range recs {
		ids[i] = r.Raw
	}
	return ids
}

func TestLogTableFollowKeepsPositionWhenScrolledUp(t *testing.T) {
	recs := make([]LogRecord, 10)
	for i := range recs {
		recs[i] = LogRecord{Message: fmt.Sprintf("line-%d", i), Raw: fmt.Sprintf("r%d", i)}
	}
	v := newTestLogTable(recs, true)
	v = loadRecords(t, v)
	v.Render(80, 5) // size + honor pendingBottom → cursor at last
	require.True(t, v.atBottom())

	// Scroll up off the bottom.
	v, _ = updateLogTable(v, logKey("k"))
	v.Render(80, 5)
	require.False(t, v.atBottom())
	before := v.rows[v.cursor].ID

	// A follow refresh brings two new lines (older ones still in the window).
	next := append(append([]LogRecord{}, recs...),
		LogRecord{Message: "line-10", Raw: "r10"}, LogRecord{Message: "line-11", Raw: "r11"})
	v, _ = updateLogTable(v, logRecordsLoadedMsg{records: next, gen: v.gen})

	assert.False(t, v.pendingBottom, "does not yank to the bottom while scrolled up")
	assert.Equal(t, 2, v.newRows, "counts the new arrivals")
	assert.Equal(t, before, v.rows[v.cursor].ID, "cursor stays on the same record")
	assert.Contains(t, v.Render(80, 5), "2 new rows", "shows the new-rows marker")
}

func TestCollapseMessage(t *testing.T) {
	assert.Equal(t, "a b c", collapseMessage("  a\n   b\t c \n"))
	assert.Empty(t, collapseMessage("   \n  "))
}

func TestCleanLogMessage(t *testing.T) {
	// Leading app timestamp + level both stripped (they have their own columns).
	assert.Equal(t, "Middleware: has_credential=True",
		cleanLogMessage("[07/17/26 13:02:42] INFO Middleware: has_credential=True"))
	assert.Equal(t, "Middleware: stored credential", cleanLogMessage("INFO Middleware: stored credential"))
	assert.Equal(t, `10.88.67.146:0 - "POST /mcp"`, cleanLogMessage(`INFO: 10.88.67.146:0 - "POST /mcp"`))
	// A non-timestamp bracket and a non-level first word are left intact.
	assert.Equal(t, "[worker] starting up", cleanLogMessage("[worker] starting up"))
	assert.Equal(t, "plain message", cleanLogMessage("plain message"))
}

func TestDisplayLevel(t *testing.T) {
	assert.Equal(t, "ERROR", displayLevel(LogRecord{Severity: "error"}))
	// Severity UNKNOWN falls back to a level word detected in the message.
	assert.Equal(t, "INFO", displayLevel(LogRecord{Severity: "UNKNOWN", Message: "   INFO doing a thing"}))
	assert.Equal(t, "WARNING", displayLevel(LogRecord{Message: "WARNING low disk"}))
	assert.Empty(t, displayLevel(LogRecord{Message: "just a message"}))
}

func TestLogLevelClass(t *testing.T) {
	assert.Equal(t, resource.CellBad, logLevelClass("ERROR"))
	assert.Equal(t, resource.CellBad, logLevelClass("CRITICAL"))
	assert.Equal(t, resource.CellWarn, logLevelClass("WARN"))
	assert.Equal(t, resource.CellGood, logLevelClass("INFO"))
	assert.Equal(t, resource.CellDefault, logLevelClass("DEBUG"))
}

func TestPrettyJSON(t *testing.T) {
	out, ok := prettyJSON(`{"a":1,"b":2}`)
	require.True(t, ok)
	assert.Contains(t, out, "\n  \"a\": 1")

	_, ok = prettyJSON("not json")
	assert.False(t, ok)
}

func TestFormatLogTime(t *testing.T) {
	assert.Empty(t, formatLogTime(time.Time{}))
	assert.NotEmpty(t, formatLogTime(time.Now()))
}
