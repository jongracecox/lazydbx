package engine

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jongracecox/lazydbx/internal/resource"
)

// Store persists cache entries to disk so a fresh launch paints instantly
// from the last session's data (marked stale) while the poller refreshes in
// the background. Layout: <dir>/<profile>/<resource>[__scope].json.
type Store struct {
	dir string
}

// NewStore roots a store at dir (usually $XDG_CACHE_HOME/lazydbx).
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

type storedEntry struct {
	FetchedAt time.Time   `json:"fetched_at"`
	Rows      []storedRow `json:"rows"`
}

type storedRow struct {
	ID    string          `json:"id"`
	Cells []string        `json:"cells"`
	Data  json.RawMessage `json:"data,omitempty"`
}

func (s *Store) path(k Key) string {
	name := k.Resource
	if k.Scope != "" {
		name += "__" + k.Scope
	}
	return filepath.Join(s.dir, sanitize(k.Profile), sanitize(name)+".json")
}

// sanitize keeps cache filenames safe across platforms.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, s)
}

// Load reads a cached entry; ok is false on miss or unreadable data.
// Row.Data round-trips through JSON, so loaded rows carry generic maps
// rather than the original domain structs — fine for describe rendering.
func (s *Store) Load(k Key) (rows []resource.Row, fetchedAt time.Time, ok bool) {
	raw, err := os.ReadFile(s.path(k))
	if err != nil {
		return nil, time.Time{}, false
	}
	var entry storedEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		slog.Warn("discarding corrupt cache entry", "path", s.path(k), "err", err)
		return nil, time.Time{}, false
	}
	rows = make([]resource.Row, len(entry.Rows))
	for i, sr := range entry.Rows {
		row := resource.Row{ID: sr.ID, Cells: sr.Cells}
		if len(sr.Data) > 0 {
			var data any
			if err := json.Unmarshal(sr.Data, &data); err == nil {
				row.Data = data
			}
		}
		rows[i] = row
	}
	return rows, entry.FetchedAt, true
}

// Save writes an entry atomically (temp file + rename); failures are logged
// and ignored — the disk cache is best-effort.
func (s *Store) Save(k Key, rows []resource.Row, fetchedAt time.Time) {
	entry := storedEntry{FetchedAt: fetchedAt, Rows: make([]storedRow, len(rows))}
	for i, r := range rows {
		sr := storedRow{ID: r.ID, Cells: r.Cells}
		if r.Data != nil {
			if raw, err := json.Marshal(r.Data); err == nil {
				sr.Data = raw
			}
		}
		entry.Rows[i] = sr
	}

	raw, err := json.Marshal(entry)
	if err != nil {
		slog.Warn("cache marshal failed", "key", k, "err", err)
		return
	}
	path := s.path(k)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		slog.Warn("cache dir create failed", "path", path, "err", err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		slog.Warn("cache write failed", "path", tmp, "err", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		slog.Warn("cache rename failed", "path", path, "err", err)
	}
}
