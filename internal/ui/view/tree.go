package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jongracecox/lazydbx/internal/theme"
	"github.com/jongracecox/lazydbx/internal/ui/component"
)

// Tree collapse markers, jless-style.
const (
	treeOpen     = "▾ "
	treeClosed   = "▸ "
	treeNoMarker = "  "
	treeIndent   = 2
)

// treeItem is one TreeNode flattened into the display list. The forest is
// flattened once on load; collapse state and the filter only change which of
// these are *visible*, so no rebuild is needed while navigating.
type treeItem struct {
	label    string
	value    string
	note     string
	path     string // dotted path from the root — collapse-state key and filter subject
	depth    int
	parent   int   // -1 for roots
	children []int // indices into Tree.items
	leaves   int   // leaf descendants (1 for a leaf), shown on a collapsed branch
	haystack string
}

// Tree is a collapsible tree of key/value data — the view behind a table's
// properties tab. It opens fully expanded; left/right (or h/l) collapse and
// expand the node under the cursor, `-`/`+` do the whole tree at once, `/`
// filters across paths and values, and `enter` on a leaf pushes a RowDetail
// with the full untruncated value.
type Tree struct {
	webLink

	th    theme.Theme
	title string
	fetch func(ctx context.Context) ([]TreeNode, error)

	items   []treeItem
	roots   []int
	visible []int           // items on screen, in order, after collapse + filter
	folded  map[string]bool // path → collapsed
	cursor  int             // index into visible
	off     int             // first visible row (scroll offset)
	bodyH   int             // last rendered body height, for paging

	filterOpen  bool
	filter      component.FilterBar
	filterQuery string

	loading bool
	err     error
}

// NewTree builds a tree over nodes already in hand.
func NewTree(th theme.Theme, title string, nodes []TreeNode) *Tree {
	t := &Tree{th: th, title: title, folded: map[string]bool{}, filter: component.NewFilterBar()}
	t.setNodes(nodes)
	return t
}

// NewLazyTree fetches its nodes on Init — used for tabs, whose data isn't known
// when the tab set is declared.
func NewLazyTree(th theme.Theme, title string, fetch func(ctx context.Context) ([]TreeNode, error)) *Tree {
	t := NewTree(th, title, nil)
	t.fetch = fetch
	t.loading = true
	return t
}

// treeLoadedMsg carries a lazy tree fetch result.
type treeLoadedMsg struct {
	target *Tree
	nodes  []TreeNode
	err    error
}

// Init implements View; lazy instances start their fetch.
func (t *Tree) Init() tea.Cmd {
	if t.fetch == nil {
		return nil
	}
	fetch, target := t.fetch, t
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		nodes, err := fetch(ctx)
		return treeLoadedMsg{target: target, nodes: nodes, err: err}
	}
}

// Close implements View.
func (t *Tree) Close() {}

// Title implements View.
func (t *Tree) Title() string { return t.title }

// CapturesKeys reports true while the filter prompt is open, so global
// shortcuts don't steal typed characters.
func (t *Tree) CapturesKeys() bool { return t.filterOpen }

// Hints implements View.
func (t *Tree) Hints() []key.Binding {
	return append([]key.Binding{
		key.NewBinding(key.WithKeys("right"), key.WithHelp("←/→", "collapse/expand")),
		key.NewBinding(key.WithKeys("-"), key.WithHelp("-/+", "collapse/expand all")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "value")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	}, t.webHints()...)
}

// Update implements View.
func (t *Tree) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case treeLoadedMsg:
		if msg.target != t {
			return t, nil
		}
		t.loading = false
		t.err = msg.err
		t.setNodes(msg.nodes)
		return t, nil
	case tea.KeyPressMsg:
		return t.handleKey(msg)
	}
	return t, nil
}

func (t *Tree) handleKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	if t.filterOpen {
		return t.handleFilterKey(msg)
	}

	switch msg.String() {
	case "up", "k":
		t.moveCursor(-1)
	case "down", "j":
		t.moveCursor(1)
	case "pgup", "b":
		t.moveCursor(-t.pageStep())
	case "pgdown", " ":
		t.moveCursor(t.pageStep())
	case "home", "g":
		t.moveCursorTo(0)
	case "end", "G":
		t.moveCursorTo(len(t.visible) - 1)
	case "right", "l":
		t.expandCursor()
	case "left", "h":
		t.collapseCursor()
	case "enter":
		cmd := t.openCursor()
		return t, cmd
	case "-", "_":
		t.foldAll(true)
	case "+", "=":
		t.foldAll(false)
	case "/":
		t.filterOpen = true
		var cmd tea.Cmd
		t.filter, cmd = t.filter.Open(t.filterQuery)
		return t, cmd
	case "o":
		if cmd := t.openWeb(); cmd != nil {
			return t, cmd
		}
	case "esc":
		if t.filterQuery != "" {
			t.filterQuery = ""
			t.applyVisible()
			return t, nil
		}
		return t, func() tea.Msg { return PopMsg{} }
	}
	return t, nil
}

func (t *Tree) handleFilterKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	var event component.Event
	var cmd tea.Cmd
	t.filter, event, cmd = t.filter.Update(msg)
	switch event.Kind {
	case component.EventSubmit, component.EventCancel, component.EventChanged:
		if event.Kind != component.EventChanged {
			t.filterOpen = false
		}
		t.filterQuery = event.Value
		t.applyVisible()
	case component.EventNone:
	}
	return t, cmd
}

// setNodes flattens the forest and rebuilds the visible list.
func (t *Tree) setNodes(nodes []TreeNode) {
	t.items = nil
	t.roots = t.flatten(nodes, -1, "", 0)
	t.applyVisible()
}

// flatten appends nodes (depth-first) to t.items and returns their indices,
// filling in each node's path, leaf count, and filter haystack.
func (t *Tree) flatten(nodes []TreeNode, parent int, prefix string, depth int) []int {
	idx := make([]int, 0, len(nodes))
	for _, n := range nodes {
		path := n.Label
		if prefix != "" {
			path = prefix + "." + n.Label
		}
		i := len(t.items)
		t.items = append(t.items, treeItem{
			label: n.Label, value: n.Value, note: n.Note, path: path, depth: depth, parent: parent,
			// The note is searchable too, so `/2026-07` finds a timestamp by its
			// readable form as well as its epoch value.
			haystack: strings.ToLower(path + " " + n.Value + " " + n.Note),
		})
		idx = append(idx, i)

		kids := t.flatten(n.Children, i, path, depth+1)
		leaves := 1
		if len(kids) > 0 {
			leaves = 0
			for _, k := range kids {
				leaves += t.items[k].leaves
			}
		}
		t.items[i].children = kids
		t.items[i].leaves = leaves
	}
	return idx
}

// applyVisible recomputes the on-screen list from collapse state and the
// filter, keeping the cursor on the same node where it survives.
func (t *Tree) applyVisible() {
	query := strings.ToLower(strings.TrimSpace(t.filterQuery))
	var keep []bool
	if query != "" {
		keep = make([]bool, len(t.items))
		for i := range t.items {
			if strings.Contains(t.items[i].haystack, query) {
				t.keepSubtree(i, keep)
				for p := t.items[i].parent; p >= 0; p = t.items[p].parent {
					keep[p] = true
				}
			}
		}
	}

	was := t.cursorPath()
	t.visible = t.visible[:0]
	t.walk(t.roots, query != "", keep)
	t.restoreCursor(was)
}

// keepSubtree marks i and everything under it as filter survivors: matching a
// branch (e.g. "delta") keeps its whole subtree, not just the branch line.
func (t *Tree) keepSubtree(i int, keep []bool) {
	keep[i] = true
	for _, k := range t.items[i].children {
		t.keepSubtree(k, keep)
	}
}

// walk appends the visible items in display order. While a filter is active,
// only survivors (keep) are listed and collapse state is ignored, so no hit can
// stay hidden inside a collapsed branch.
func (t *Tree) walk(idx []int, filtering bool, keep []bool) {
	for _, i := range idx {
		if filtering && !keep[i] {
			continue
		}
		t.visible = append(t.visible, i)
		if !filtering && t.folded[t.items[i].path] {
			continue
		}
		t.walk(t.items[i].children, filtering, keep)
	}
}

// cursorPath is the path of the node under the cursor ("" when there is none).
func (t *Tree) cursorPath() string {
	if t.cursor < 0 || t.cursor >= len(t.visible) {
		return ""
	}
	return t.items[t.visible[t.cursor]].path
}

// restoreCursor puts the cursor back on path when it is still visible, else
// clamps it into range.
func (t *Tree) restoreCursor(path string) {
	if path != "" {
		for i, item := range t.visible {
			if t.items[item].path == path {
				t.moveCursorTo(i)
				return
			}
		}
	}
	t.moveCursorTo(t.cursor)
}

func (t *Tree) moveCursor(delta int) { t.moveCursorTo(t.cursor + delta) }

// moveCursorTo clamps the cursor into range and scrolls it into view.
func (t *Tree) moveCursorTo(i int) {
	t.cursor = min(max(i, 0), max(len(t.visible)-1, 0))
	switch {
	case t.cursor < t.off:
		t.off = t.cursor
	case t.bodyH > 0 && t.cursor >= t.off+t.bodyH:
		t.off = t.cursor - t.bodyH + 1
	}
	if maxOff := max(len(t.visible)-max(t.bodyH, 1), 0); t.off > maxOff {
		t.off = maxOff
	}
}

func (t *Tree) pageStep() int { return max(t.bodyH-1, 1) }

// expandCursor opens the branch under the cursor, or steps into it when it is
// already open (its first child is the next visible row).
func (t *Tree) expandCursor() {
	path := t.cursorPath()
	if path == "" || len(t.items[t.visible[t.cursor]].children) == 0 {
		return
	}
	if t.folded[path] {
		delete(t.folded, path)
		t.applyVisible()
		return
	}
	t.moveCursor(1)
}

// collapseCursor closes the branch under the cursor; on a leaf or an
// already-closed branch it jumps to the parent instead, so repeated presses
// walk back out of the tree.
func (t *Tree) collapseCursor() {
	if t.cursorPath() == "" {
		return
	}
	item := t.items[t.visible[t.cursor]]
	if len(item.children) > 0 && !t.folded[item.path] {
		t.folded[item.path] = true
		t.applyVisible()
		return
	}
	if item.parent < 0 {
		return
	}
	for i, idx := range t.visible {
		if idx == item.parent {
			t.moveCursorTo(i)
			return
		}
	}
}

// foldAll collapses or expands every branch.
func (t *Tree) foldAll(fold bool) {
	t.folded = map[string]bool{}
	if fold {
		for i := range t.items {
			if len(t.items[i].children) > 0 {
				t.folded[t.items[i].path] = true
			}
		}
	}
	t.applyVisible()
}

// openCursor toggles a branch, or shows a leaf's full value in a RowDetail
// (tree lines are truncated to the width; this is where the whole value lives).
func (t *Tree) openCursor() tea.Cmd {
	if t.cursorPath() == "" {
		return nil
	}
	item := t.items[t.visible[t.cursor]]
	if len(item.children) > 0 {
		if t.folded[item.path] {
			delete(t.folded, item.path)
		} else {
			t.folded[item.path] = true
		}
		t.applyVisible()
		return nil
	}
	detail := NewRowDetail(t.th, item.label, []string{"PROPERTY", "VALUE"}, []string{item.path, item.value})
	return func() tea.Msg { return PushMsg{View: detail} }
}

// Render draws the filter prompt (when open) and the visible rows, reserving a
// right-hand column for the scrollbar when the tree overflows.
func (t *Tree) Render(width, height int) string {
	var top string
	if t.filterOpen {
		top = t.filter.View(t.th, width) + "\n"
		height--
	}
	t.bodyH = max(height, 1)
	t.moveCursorTo(t.cursor)

	if msg := t.emptyMessage(); msg != "" {
		return top + msg
	}

	bar := len(t.visible) > t.bodyH && width > 1
	contentW := width
	if bar {
		contentW = width - 1
	}

	lines := make([]string, 0, t.bodyH)
	for i := t.off; i < len(t.visible) && i < t.off+t.bodyH; i++ {
		lines = append(lines, t.line(t.items[t.visible[i]], contentW, i == t.cursor))
	}
	body := strings.Join(lines, "\n")
	if bar {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body,
			component.Scrollbar(t.th, t.bodyH, len(t.visible), t.bodyH, t.off))
	}
	return top + body
}

// emptyMessage is the stand-in body for a tree with nothing to draw.
func (t *Tree) emptyMessage() string {
	switch {
	case t.err != nil:
		return t.th.Error.Render("failed to load: " + t.err.Error())
	case t.loading:
		return t.th.Subtle.Render("loading…")
	case len(t.visible) == 0 && t.filterQuery != "":
		return t.th.Subtle.Render("no match for " + t.filterQuery)
	case len(t.visible) == 0:
		return t.th.Subtle.Render("none")
	}
	return ""
}

// line renders one node: an indented marker, the accented label, and either the
// leaf value or — on a collapsed branch — the number of leaves it hides.
func (t *Tree) line(item treeItem, width int, selected bool) string {
	indent := strings.Repeat(" ", item.depth*treeIndent)
	branch := len(item.children) > 0
	closed := branch && t.folded[item.path] && t.filterQuery == ""

	marker := treeNoMarker
	if branch {
		marker = treeOpen
		if closed {
			marker = treeClosed
		}
	}
	label := item.label + ":"
	// trail is the value (or the hidden-leaf count); gloss is the subtle
	// parenthesised note after it.
	trail, gloss := "", ""
	switch {
	case closed:
		gloss = fmt.Sprintf(" {%d}", item.leaves)
	case item.value != "":
		trail = " " + oneLine(item.value)
		if item.note != "" {
			gloss = " (" + oneLine(item.note) + ")"
		}
	}

	// The cursor row is reversed as a whole, so it renders unstyled to keep the
	// highlight uniform (nested styles would punch holes in it).
	if selected {
		return lipgloss.NewStyle().Reverse(true).Width(width).
			Render(ansi.Truncate(indent+marker+label+trail+gloss, width, "…"))
	}
	styled := indent + t.th.Subtle.Render(marker) + t.th.KeyHint.Render(label) +
		trail + t.th.Subtle.Render(gloss)
	return ansi.Truncate(styled, width, "…")
}

// oneLine flattens a value so a multi-line property still occupies one row;
// `enter` shows the original.
func oneLine(s string) string {
	if !strings.ContainsAny(s, "\n\r\t") {
		return s
	}
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}
