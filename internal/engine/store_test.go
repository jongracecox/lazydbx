package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/resource"
)

func TestStoreRoundTrip(t *testing.T) {
	s := NewStore(t.TempDir())
	key := Key{Profile: "dev", Resource: "tables", Scope: "catalog=main,schema=silver"}
	at := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	rows := []resource.Row{
		{ID: "a", Cells: []string{"a", "MANAGED"}, Data: map[string]any{"name": "a"}},
		{ID: "b", Cells: []string{"b", "VIEW"}},
	}

	s.Save(key, rows, at)

	got, fetchedAt, ok := s.Load(key)
	require.True(t, ok)
	assert.Equal(t, at, fetchedAt)
	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].ID)
	assert.Equal(t, []string{"a", "MANAGED"}, got[0].Cells)
	assert.Equal(t, map[string]any{"name": "a"}, got[0].Data, "Data round-trips as generic JSON")
	assert.Nil(t, got[1].Data)
}

func TestStoreMiss(t *testing.T) {
	s := NewStore(t.TempDir())
	_, _, ok := s.Load(Key{Profile: "dev", Resource: "catalogs"})
	assert.False(t, ok)
}

func TestStoreKeysAreIsolatedPerProfile(t *testing.T) {
	s := NewStore(t.TempDir())
	at := time.Now().UTC().Truncate(time.Second)
	s.Save(Key{Profile: "dev", Resource: "catalogs"}, []resource.Row{{ID: "dev-cat"}}, at)
	s.Save(Key{Profile: "prod", Resource: "catalogs"}, []resource.Row{{ID: "prod-cat"}}, at)

	devRows, _, ok := s.Load(Key{Profile: "dev", Resource: "catalogs"})
	require.True(t, ok)
	assert.Equal(t, "dev-cat", devRows[0].ID)

	prodRows, _, ok := s.Load(Key{Profile: "prod", Resource: "catalogs"})
	require.True(t, ok)
	assert.Equal(t, "prod-cat", prodRows[0].ID)
}

func TestWatchServesDiskCacheAcrossEngineInstances(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	NewStore(dir).Save(testKey, []resource.Row{{ID: "cached", Cells: []string{"cached"}}}, at)

	// Fresh engine (fresh process, in effect): Watch must serve the disk
	// entry as stale before the live fetch lands.
	h := newHarness()
	h.eng.store = NewStore(dir)
	h.eng.Watch(testKey, func(context.Context) ([]resource.Row, error) {
		return rowsOf("live"), nil
	}, time.Second)
	defer h.eng.Stop()

	ev := h.event(t)
	assert.True(t, ev.Stale, "disk rows arrive marked stale")
	require.Len(t, ev.Rows, 1)
	assert.Equal(t, "cached", ev.Rows[0].ID)
	assert.Equal(t, at, ev.FetchedAt, "age reflects the original fetch time")

	ev = h.event(t)
	assert.False(t, ev.Stale)
	assert.Equal(t, "live", ev.Rows[0].ID)
}
