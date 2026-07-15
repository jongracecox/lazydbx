package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/resource"
)

// harness wires an Engine with manual time: ticks fire only when the test
// sends on tickCh, events arrive on evCh, and every timer arm is reported on
// armCh so tests can assert wait durations without racing the poller.
type harness struct {
	eng    *Engine
	evCh   chan DataEvent
	tickCh chan time.Time
	armCh  chan time.Duration
}

func newHarness() *harness {
	h := &harness{
		evCh:   make(chan DataEvent, 100),
		tickCh: make(chan time.Time),
		armCh:  make(chan time.Duration, 100),
	}
	h.eng = New(func(ev DataEvent) { h.evCh <- ev })
	h.eng.now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	h.eng.after = func(d time.Duration) <-chan time.Time {
		h.armCh <- d
		return h.tickCh
	}
	return h
}

// arm returns the duration of the next timer arm, waiting for the poller.
func (h *harness) arm(t *testing.T) time.Duration {
	t.Helper()
	select {
	case d := <-h.armCh:
		return d
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for timer arm")
		return 0
	}
}

func (h *harness) event(t *testing.T) DataEvent {
	t.Helper()
	select {
	case ev := <-h.evCh:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for DataEvent")
		return DataEvent{}
	}
}

// tick releases the poller for one more fetch cycle.
func (h *harness) tick(t *testing.T) {
	t.Helper()
	select {
	case h.tickCh <- time.Now():
	case <-time.After(2 * time.Second):
		t.Fatal("poller never armed its timer")
	}
}

func rowsOf(ids ...string) []resource.Row {
	rows := make([]resource.Row, len(ids))
	for i, id := range ids {
		rows[i] = resource.Row{ID: id, Cells: []string{id}}
	}
	return rows
}

var testKey = Key{Profile: "dev", Resource: "catalogs"}

func TestWatchFetchesImmediately(t *testing.T) {
	h := newHarness()
	h.eng.Watch(testKey, func(context.Context) ([]resource.Row, error) {
		return rowsOf("a", "b"), nil
	}, time.Second)
	defer h.eng.Stop()

	ev := h.event(t)
	assert.Equal(t, testKey, ev.Key)
	require.Len(t, ev.Rows, 2)
	require.NoError(t, ev.Err)
	assert.False(t, ev.Stale)
	assert.False(t, ev.FetchedAt.IsZero())
}

func TestPollEmitsOnEachTick(t *testing.T) {
	h := newHarness()
	calls := 0
	h.eng.Watch(testKey, func(context.Context) ([]resource.Row, error) {
		calls++
		return rowsOf("a"), nil
	}, time.Second)
	defer h.eng.Stop()

	h.event(t) // initial fetch
	h.tick(t)
	h.event(t) // second fetch
	assert.Equal(t, 2, calls)
}

func TestErrorKeepsStaleRowsAndBacksOff(t *testing.T) {
	h := newHarness()
	var fail bool
	h.eng.Watch(testKey, func(context.Context) ([]resource.Row, error) {
		if fail {
			return nil, errors.New("boom")
		}
		return rowsOf("a"), nil
	}, time.Second)
	defer h.eng.Stop()

	ev := h.event(t)
	require.NoError(t, ev.Err)
	h.arm(t) // initial arm after first fetch

	fail = true
	h.tick(t)
	ev = h.event(t)
	require.Error(t, ev.Err)
	assert.True(t, ev.Stale)
	require.Len(t, ev.Rows, 1, "stale rows retained on error")
	h.arm(t) // first failure: interval unchanged

	// Backoff doubles from the second consecutive failure (±10% jitter).
	h.tick(t)
	h.event(t)
	assert.InDelta(t, float64(2*time.Second), float64(h.arm(t)), float64(300*time.Millisecond))

	// Recovery resets interval and staleness.
	fail = false
	h.tick(t)
	ev = h.event(t)
	require.NoError(t, ev.Err)
	assert.False(t, ev.Stale)
	assert.InDelta(t, float64(time.Second), float64(h.arm(t)), float64(200*time.Millisecond))
}

func TestRewatchServesCacheSynchronously(t *testing.T) {
	h := newHarness()
	fetch := func(context.Context) ([]resource.Row, error) { return rowsOf("a"), nil }

	h.eng.Watch(testKey, fetch, time.Second)
	h.event(t)
	h.eng.Unwatch(testKey)

	// Second watch: cached rows arrive first (Stale), then the fresh fetch.
	h.eng.Watch(testKey, fetch, time.Second)
	defer h.eng.Stop()

	ev := h.event(t)
	assert.True(t, ev.Stale, "cached rows delivered as stale")
	require.Len(t, ev.Rows, 1)

	ev = h.event(t)
	assert.False(t, ev.Stale, "fresh fetch follows")
}

func TestRefreshNow(t *testing.T) {
	h := newHarness()
	calls := 0
	h.eng.Watch(testKey, func(context.Context) ([]resource.Row, error) {
		calls++
		return rowsOf("a"), nil
	}, time.Hour) // interval effectively never fires on its own
	defer h.eng.Stop()

	h.event(t)
	h.eng.RefreshNow(testKey)
	h.event(t)
	assert.Equal(t, 2, calls)
}

func TestUnwatchStopsPolling(t *testing.T) {
	h := newHarness()
	h.eng.Watch(testKey, func(context.Context) ([]resource.Row, error) {
		return rowsOf("a"), nil
	}, time.Second)
	h.event(t)

	h.eng.Unwatch(testKey)

	// The poller is stopped: RefreshNow on a stopped key is a no-op.
	h.eng.RefreshNow(testKey)
	select {
	case ev := <-h.evCh:
		t.Fatalf("unexpected event after unwatch: %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRefcounting(t *testing.T) {
	h := newHarness()
	fetch := func(context.Context) ([]resource.Row, error) { return rowsOf("a"), nil }

	h.eng.Watch(testKey, fetch, time.Second) // first watcher: starts the poller
	h.event(t)
	h.eng.Watch(testKey, fetch, time.Second) // second watcher: serves cache
	ev := h.event(t)
	assert.True(t, ev.Stale)

	h.eng.Unwatch(testKey) // refs=1 — still polling
	h.eng.RefreshNow(testKey)
	h.event(t) // refresh still works

	h.eng.Unwatch(testKey) // refs=0 — stopped
	h.eng.RefreshNow(testKey)
	select {
	case ev := <-h.evCh:
		t.Fatalf("unexpected event after final unwatch: %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}
