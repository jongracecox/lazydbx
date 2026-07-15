// Package engine keeps resource data fresh, k9s-style: one poll goroutine
// per watched (profile, resource, scope) key feeding an in-memory cache,
// with results pushed to the UI through a sink (the app wires p.Send).
// Views render cached rows instantly (stale-while-revalidate) while the
// poller refreshes in the background.
//
// The engine deliberately runs outside the Bubble Tea loop so cache entries
// outlive view pushes/pops and drill-back is instant.
package engine

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jongracecox/lazydbx/internal/resource"
)

// maxBackoff caps the error-doubling of poll intervals.
const maxBackoff = 5 * time.Minute

// fetchTimeout bounds a single List call.
const fetchTimeout = 60 * time.Second

// Key identifies one cached dataset.
type Key struct {
	Profile  string
	Resource string
	Scope    string // resource.Scope.Hash()
}

// DataEvent is pushed to the sink after every fetch, and synchronously on
// Watch when cached rows exist. On fetch errors Rows retains the last good
// data (Stale=true) so views can keep rendering with a staleness badge.
type DataEvent struct {
	Key       Key
	Rows      []resource.Row
	Err       error
	FetchedAt time.Time
	Stale     bool
}

// FetchFunc loads rows for one key. Bound to a def+clients+scope by the
// caller; the engine knows nothing about resources or the SDK.
type FetchFunc func(ctx context.Context) ([]resource.Row, error)

// Engine owns the cache and poll goroutines.
type Engine struct {
	sink  func(DataEvent)
	store *Store // optional disk cache; nil disables persistence
	// now and after are injection points for tests.
	now   func() time.Time
	after func(d time.Duration) <-chan time.Time

	mu      sync.Mutex
	entries map[Key]*entry
}

type entry struct {
	fetch    FetchFunc
	interval time.Duration

	refs      int
	cancel    context.CancelFunc
	refreshCh chan struct{}

	// Cached state, guarded by Engine.mu.
	rows      []resource.Row
	fetchedAt time.Time
	err       error
	hasData   bool
}

// New builds an engine delivering events to sink. The sink must be safe to
// call from any goroutine (tea.Program.Send is). A nil store disables the
// disk cache.
func New(sink func(DataEvent), store *Store) *Engine {
	return &Engine{
		sink:    sink,
		store:   store,
		now:     time.Now,
		after:   time.After,
		entries: map[Key]*entry{},
	}
}

// Watch registers interest in a key. The first watcher starts the poll
// goroutine; if cached data exists it is delivered synchronously first, so
// re-entering a view paints instantly. Fetch and interval are only used when
// the key is not already being polled.
func (e *Engine) Watch(key Key, fetch FetchFunc, interval time.Duration) {
	e.mu.Lock()
	ent, ok := e.entries[key]
	if !ok {
		ent = &entry{fetch: fetch, interval: interval, refreshCh: make(chan struct{}, 1)}
		e.entries[key] = ent
	}
	ent.refs++
	starting := ent.refs == 1 && ent.cancel == nil
	// Memory miss → try the disk cache from a previous session, so first
	// paint is instant even across restarts.
	if !ent.hasData && e.store != nil {
		if rows, fetchedAt, ok := e.store.Load(key); ok {
			ent.rows, ent.fetchedAt, ent.hasData = rows, fetchedAt, true
		}
	}
	var cached *DataEvent
	if ent.hasData {
		cached = &DataEvent{Key: key, Rows: ent.rows, Err: ent.err, FetchedAt: ent.fetchedAt, Stale: true}
	}
	if starting {
		ctx, cancel := context.WithCancel(context.Background())
		ent.cancel = cancel
		go e.poll(ctx, key, ent)
	}
	e.mu.Unlock()

	if cached != nil {
		e.sink(*cached)
	}
}

// Unwatch drops one watcher; at zero the poll goroutine stops. The cache
// entry survives so the next Watch paints instantly.
func (e *Engine) Unwatch(key Key) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ent, ok := e.entries[key]
	if !ok {
		return
	}
	ent.refs--
	if ent.refs <= 0 {
		ent.refs = 0
		if ent.cancel != nil {
			ent.cancel()
			ent.cancel = nil
		}
	}
}

// RefreshNow requests an immediate out-of-band fetch (ctrl+r). Non-blocking;
// coalesces if a refresh is already queued.
func (e *Engine) RefreshNow(key Key) {
	e.mu.Lock()
	ent, ok := e.entries[key]
	e.mu.Unlock()
	if !ok || ent.cancel == nil {
		return
	}
	select {
	case ent.refreshCh <- struct{}{}:
	default:
	}
}

// Stop cancels all pollers (app shutdown).
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, ent := range e.entries {
		if ent.cancel != nil {
			ent.cancel()
			ent.cancel = nil
		}
		ent.refs = 0
	}
}

// poll is the per-key loop: fetch immediately, then re-arm a jittered timer
// after each fetch completes (overlap is structurally impossible), doubling
// the interval on consecutive errors up to maxBackoff.
func (e *Engine) poll(ctx context.Context, key Key, ent *entry) {
	failures := 0
	for {
		e.fetchOnce(ctx, key, ent, &failures)

		wait := ent.interval
		if failures > 0 {
			wait = backoff(ent.interval, failures)
		}
		select {
		case <-ctx.Done():
			return
		case <-ent.refreshCh:
		case <-e.after(jitter(wait)):
		}
	}
}

func (e *Engine) fetchOnce(ctx context.Context, key Key, ent *entry, failures *int) {
	fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	rows, err := ent.fetch(fctx)
	cancel()
	if ctx.Err() != nil {
		return // stopped mid-fetch; don't emit
	}

	e.mu.Lock()
	if err != nil {
		*failures++
		ent.err = err
		// Keep the last good rows for stale rendering.
		slog.Warn("fetch failed", "resource", key.Resource, "scope", key.Scope, "failures", *failures, "err", err)
	} else {
		*failures = 0
		ent.rows = rows
		ent.err = nil
		ent.fetchedAt = e.now()
		ent.hasData = true
	}
	ev := DataEvent{Key: key, Rows: ent.rows, Err: ent.err, FetchedAt: ent.fetchedAt, Stale: err != nil}
	e.mu.Unlock()

	if err == nil && e.store != nil {
		e.store.Save(key, rows, ev.FetchedAt)
	}
	e.sink(ev)
}

func backoff(base time.Duration, failures int) time.Duration {
	d := base
	for i := 1; i < failures; i++ {
		d *= 2
		if d >= maxBackoff {
			return maxBackoff
		}
	}
	return min(d, maxBackoff)
}

// jitter spreads poll ticks ±10% so many watched keys don't thundering-herd
// the API on the same beat.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	delta := float64(d) * 0.1
	return d + time.Duration((rand.Float64()*2-1)*delta)
}
