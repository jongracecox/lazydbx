// Package favorites persists per-profile row stars. Databricks exposes no
// public API for the web UI's favorites, so lazydbx keeps its own — the same
// local-metadata pattern lazyssh uses for pins.
package favorites

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/adrg/xdg"
)

// Store holds favorites keyed profile → view key (resource+scope) → row ID.
type Store struct {
	path string

	mu   sync.Mutex
	data map[string]map[string]map[string]bool
}

// NewStore loads (best-effort) the store at path.
func NewStore(path string) *Store {
	s := &Store{path: path, data: map[string]map[string]map[string]bool{}}
	raw, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(raw, &s.data); err != nil {
			slog.Warn("discarding corrupt favorites file", "path", path, "err", err)
			s.data = map[string]map[string]map[string]bool{}
		}
	}
	return s
}

// NewDefault opens the store at its standard state-dir location.
func NewDefault() *Store {
	path, err := xdg.StateFile(filepath.Join("lazydbx", "favorites.json"))
	if err != nil {
		slog.Warn("favorites unavailable", "err", err)
		return NewStore("") // in-memory only; saves will fail quietly
	}
	return NewStore(path)
}

// IsFavorite reports whether an ID is starred.
func (s *Store) IsFavorite(profile, key, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[profile][key][id]
}

// Toggle flips an ID's star and persists; it returns the new state.
func (s *Store) Toggle(profile, key, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data[profile] == nil {
		s.data[profile] = map[string]map[string]bool{}
	}
	if s.data[profile][key] == nil {
		s.data[profile][key] = map[string]bool{}
	}
	now := !s.data[profile][key][id]
	if now {
		s.data[profile][key][id] = true
	} else {
		delete(s.data[profile][key], id)
	}
	s.save()
	return now
}

// save writes atomically; failures are logged and ignored (favorites are
// best-effort metadata).
func (s *Store) save() {
	if s.path == "" {
		return
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		slog.Warn("favorites marshal failed", "err", err)
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		slog.Warn("favorites write failed", "err", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		slog.Warn("favorites rename failed", "err", err)
	}
}
