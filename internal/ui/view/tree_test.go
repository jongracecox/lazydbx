package view

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/theme"
)

// sampleTree mirrors the shape of table properties: dotted keys grouped into
// branches, with one noisy generated subtree.
func sampleTree() []TreeNode {
	return []TreeNode{
		{Label: "delta", Children: []TreeNode{
			{Label: "enableDeletionVectors", Value: "true"},
			{Label: "feature", Children: []TreeNode{
				{Label: "appendOnly", Value: "supported"},
				{Label: "deletionVectors", Value: "supported"},
			}},
		}},
		{Label: "pipeline", Children: []TreeNode{
			{Label: "drop_and_recreate", Value: "true"},
		}},
	}
}

func newTestTree() *Tree {
	tr := NewTree(theme.Default(), "props", sampleTree())
	tr.Render(80, 24) // establish the body height so paging/scrolling work
	return tr
}

// visiblePaths is the on-screen node list, as dotted paths.
func visiblePaths(tr *Tree) []string {
	out := make([]string, len(tr.visible))
	for i, idx := range tr.visible {
		out[i] = tr.items[idx].path
	}
	return out
}

func treePress(tr *Tree, r rune) tea.Cmd {
	_, cmd := tr.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	return cmd
}

func treeKey(tr *Tree, code rune) tea.Cmd {
	_, cmd := tr.Update(tea.KeyPressMsg{Code: code})
	return cmd
}

func TestTreeStartsFullyExpanded(t *testing.T) {
	tr := newTestTree()
	assert.Equal(t, []string{
		"delta", "delta.enableDeletionVectors", "delta.feature",
		"delta.feature.appendOnly", "delta.feature.deletionVectors",
		"pipeline", "pipeline.drop_and_recreate",
	}, visiblePaths(tr), "every node is visible by default")

	out := tr.Render(80, 24)
	assert.Contains(t, out, treeOpen+"delta:", "expanded branches show the open marker")
	assert.Contains(t, out, "drop_and_recreate:", "leaves show their key")
	assert.Contains(t, out, "supported", "leaves show their value")
}

func TestTreeCollapseAndExpandNode(t *testing.T) {
	tr := newTestTree()

	// Cursor starts on the first root; left collapses its whole subtree.
	treeKey(tr, tea.KeyLeft)
	assert.Equal(t, []string{"delta", "pipeline", "pipeline.drop_and_recreate"}, visiblePaths(tr))
	assert.Contains(t, tr.Render(80, 24), treeClosed+"delta:", "a collapsed branch shows the closed marker")
	assert.Contains(t, tr.Render(80, 24), "{3}", "and how many leaves it hides, at any depth")

	// Right expands it again, and a second right steps into the first child.
	treeKey(tr, tea.KeyRight)
	assert.Len(t, visiblePaths(tr), 7)
	assert.Equal(t, "delta", tr.cursorPath(), "expanding leaves the cursor on the branch")
	treeKey(tr, tea.KeyRight)
	assert.Equal(t, "delta.enableDeletionVectors", tr.cursorPath(), "a second right steps into the branch")

	// h/l are aliases for left/right.
	treePress(tr, 'l')
	assert.Equal(t, "delta.enableDeletionVectors", tr.cursorPath(), "right on a leaf does nothing")
	treePress(tr, 'h')
	assert.Equal(t, "delta", tr.cursorPath(), "left on a leaf jumps out to its parent")
}

func TestTreeCollapseAndExpandAll(t *testing.T) {
	tr := newTestTree()

	treePress(tr, '-')
	assert.Equal(t, []string{"delta", "pipeline"}, visiblePaths(tr), "- collapses every branch")

	treePress(tr, '+')
	assert.Len(t, visiblePaths(tr), 7, "+ expands everything again")
}

func TestTreeEnterOnLeafOpensFullValue(t *testing.T) {
	tr := newTestTree()
	tr.moveCursorTo(1) // delta.enableDeletionVectors

	cmd := treeKey(tr, tea.KeyEnter)
	require.NotNil(t, cmd)
	push, ok := cmd().(PushMsg)
	require.True(t, ok, "enter on a leaf pushes the full value")
	detail, ok := push.View.(*RowDetail)
	require.True(t, ok)
	out := detail.Render(80, 24)
	assert.Contains(t, out, "delta.enableDeletionVectors", "the detail names the full path")
	assert.Contains(t, out, "true")
}

func TestTreeEnterOnBranchToggles(t *testing.T) {
	tr := newTestTree()

	cmd := treeKey(tr, tea.KeyEnter)
	assert.Nil(t, cmd, "enter on a branch stays in the tree")
	assert.Equal(t, []string{"delta", "pipeline", "pipeline.drop_and_recreate"}, visiblePaths(tr))
	treeKey(tr, tea.KeyEnter)
	assert.Len(t, visiblePaths(tr), 7)
}

func TestTreeFilterShowsMatchesAndAncestors(t *testing.T) {
	tr := newTestTree()
	treePress(tr, '-') // collapsed: a hit must still surface
	treePress(tr, '/')
	assert.True(t, tr.CapturesKeys(), "the filter prompt captures typing")

	for _, r := range "drop" {
		treePress(tr, r)
	}
	assert.Equal(t, []string{"pipeline", "pipeline.drop_and_recreate"}, visiblePaths(tr),
		"a match brings its ancestors with it, ignoring collapse state")

	// A branch match keeps its whole subtree.
	treeKey(tr, tea.KeyEscape) // close the prompt, keeping the query
	treePress(tr, '/')
	for i := 0; i < len("drop"); i++ {
		treeKey(tr, tea.KeyBackspace)
	}
	for _, r := range "feature" {
		treePress(tr, r)
	}
	assert.Equal(t, []string{
		"delta", "delta.feature", "delta.feature.appendOnly",
		"delta.feature.deletionVectors",
	}, visiblePaths(tr))

	// Values are searched too, not just keys.
	treeKey(tr, tea.KeyEnter) // submit
	assert.False(t, tr.CapturesKeys())
	tr.filterQuery = "supported"
	tr.applyVisible()
	assert.Equal(t, []string{
		"delta", "delta.feature", "delta.feature.appendOnly",
		"delta.feature.deletionVectors",
	}, visiblePaths(tr))

	// esc clears the filter before it pops the view.
	_, cmd := tr.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Nil(t, cmd, "the first esc only clears the filter")
	assert.Len(t, visiblePaths(tr), 2, "clearing the filter restores the collapsed tree")
	_, cmd = tr.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	_, ok := cmd().(PopMsg)
	assert.True(t, ok, "the second esc pops")
}

func TestTreeFilterNoMatch(t *testing.T) {
	tr := newTestTree()
	tr.filterQuery = "nothing-here"
	tr.applyVisible()
	assert.Empty(t, visiblePaths(tr))
	assert.Contains(t, tr.Render(80, 24), "no match", "an empty result says so")
}

func TestTreeScrollsCursorIntoView(t *testing.T) {
	nodes := make([]TreeNode, 40)
	for i := range nodes {
		nodes[i] = TreeNode{Label: "key" + strings.Repeat("x", i%3), Value: "v"}
	}
	tr := NewTree(theme.Default(), "props", nodes)
	tr.Render(80, 10)

	treePress(tr, 'G')
	out := tr.Render(80, 10)
	assert.Equal(t, 39, tr.cursor)
	assert.Positive(t, tr.off, "the view scrolled to keep the cursor visible")
	assert.Contains(t, out, "█", "the scrollbar shows when the tree overflows")
	assert.Len(t, strings.Split(out, "\n"), 10, "the body fills exactly the given height")

	treePress(tr, 'g')
	assert.Equal(t, 0, tr.cursor)
	assert.Equal(t, 0, tr.off, "home scrolls back to the top")
	assert.NotContains(t, NewTree(theme.Default(), "props", sampleTree()).Render(80, 24), "█",
		"no scrollbar when everything fits")
}

func TestTreeLazyFetch(t *testing.T) {
	tr := NewLazyTree(theme.Default(), "props", func(context.Context) ([]TreeNode, error) {
		return sampleTree(), nil
	})
	assert.Contains(t, tr.Render(80, 24), "loading…")

	cmd := tr.Init()
	require.NotNil(t, cmd)
	tr.Update(cmd())
	assert.Len(t, visiblePaths(tr), 7)
	assert.Contains(t, tr.Render(80, 24), "drop_and_recreate")
}

func TestTreeLazyFetchError(t *testing.T) {
	tr := NewLazyTree(theme.Default(), "props", func(context.Context) ([]TreeNode, error) {
		return nil, errors.New("boom")
	})
	tr.Update(tr.Init()())
	assert.Contains(t, tr.Render(80, 24), "boom")
}

func TestTreeEmpty(t *testing.T) {
	tr := NewTree(theme.Default(), "props", nil)
	assert.Contains(t, tr.Render(80, 24), "none")
	// Navigation on an empty tree must not panic.
	treePress(tr, 'j')
	treeKey(tr, tea.KeyLeft)
	treeKey(tr, tea.KeyRight)
	assert.Nil(t, treeKey(tr, tea.KeyEnter))
}

func TestTreeBranchWithOwnValue(t *testing.T) {
	tr := NewTree(theme.Default(), "props", []TreeNode{
		{Label: "delta", Value: "root", Children: []TreeNode{{Label: "kid", Value: "v"}}},
	})
	out := tr.Render(80, 24)
	assert.Contains(t, out, "root", "a branch that also holds a value renders it inline")
}

func TestTreeRendersNoteAfterValue(t *testing.T) {
	tr := NewTree(theme.Default(), "props", []TreeNode{
		{Label: "version", Value: "6215"},
		{Label: "createdAt", Value: "1785273330123", Note: "2026-07-29 12:35:30.123"},
	})
	lines := strings.Split(tr.Render(80, 24), "\n")
	require.Len(t, lines, 2)

	assert.Contains(t, lines[1], "1785273330123", "the raw value is still shown")
	assert.Contains(t, lines[1], "(2026-07-29 12:35:30.123)", "with the note glossed after it")
	assert.Contains(t, lines[1], theme.Default().Subtle.Render(" (2026-07-29 12:35:30.123)"),
		"the note is rendered subtly")
	assert.NotContains(t, lines[0], "(", "a value with no note gets no parentheses")

	// The note is searchable, so a timestamp is findable by its readable form.
	tr.filterQuery = "12:35:30"
	tr.applyVisible()
	assert.Equal(t, []string{"createdAt"}, visiblePaths(tr))
}

func TestTreeLeafValueIsOneLine(t *testing.T) {
	tr := NewTree(theme.Default(), "props", []TreeNode{
		{Label: "json", Value: "{\n  \"a\": 1\n}"},
	})
	out := tr.Render(80, 24)
	assert.Len(t, strings.Split(out, "\n"), 1, "a multi-line value still occupies one row")
	assert.Contains(t, out, `{ "a": 1 }`)
}
