package favorites

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToggleAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	s := NewStore(path)

	assert.False(t, s.IsFavorite("dev", "jobs|", "123"))
	assert.True(t, s.Toggle("dev", "jobs|", "123"), "first toggle stars")
	assert.True(t, s.IsFavorite("dev", "jobs|", "123"))

	// Fresh store from the same file sees the persisted star.
	s2 := NewStore(path)
	assert.True(t, s2.IsFavorite("dev", "jobs|", "123"))

	assert.False(t, s2.Toggle("dev", "jobs|", "123"), "second toggle unstars")
	assert.False(t, NewStore(path).IsFavorite("dev", "jobs|", "123"))
}

func TestIsolation(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "f.json"))
	s.Toggle("dev", "jobs|", "1")

	assert.False(t, s.IsFavorite("prod", "jobs|", "1"), "profiles are isolated")
	assert.False(t, s.IsFavorite("dev", "pipelines|", "1"), "views are isolated")
	assert.False(t, s.IsFavorite("dev", "jobs|", "2"), "ids are isolated")
}

func TestCorruptFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))
	s := NewStore(path)
	assert.False(t, s.IsFavorite("dev", "jobs|", "1"))
	// And it can still save over the corrupt file.
	assert.True(t, s.Toggle("dev", "jobs|", "1"))
	assert.True(t, NewStore(path).IsFavorite("dev", "jobs|", "1"))
}
