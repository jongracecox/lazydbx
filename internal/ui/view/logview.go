package view

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/adrg/xdg"
	"github.com/charmbracelet/x/ansi"

	"github.com/jongracecox/lazydbx/internal/theme"
	"github.com/jongracecox/lazydbx/internal/ui/component"
)

// LogView shows a text source (task-run output, pipeline events) in a
// scrollable viewport with optional follow-tail, wrap/no-wrap toggling,
// horizontal scrolling, in-content search, and save-to-file. All fetching
// happens inside tea.Cmds; Update stays pure and drives the lifecycle through
// the private messages at the bottom of this file.
type LogView struct {
	th    theme.Theme
	title string
	fetch func(ctx context.Context) (string, error)

	vp   viewport.Model
	vpOK bool

	content   string
	loaded    bool
	err       error
	fetchedAt time.Time

	follow bool
	wrap   bool
	xoff   int

	// gen guards fetch results: it increments on every fetch trigger, and a
	// logLoadedMsg carrying an older generation is dropped.
	gen int
	// followGen guards the self-rescheduling follow ticker: toggling follow
	// bumps it so ticks from a previous follow session stop.
	followGen int

	// search state; matchLines are display-line indices (recomputed at render)
	// containing a case-insensitive match, matchIdx points at the current one.
	searchOpen  bool
	search      component.FilterBar
	searchQuery string
	matchLines  []int
	matchIdx    int

	// render-time scroll intents, honored once and cleared in Render.
	jumpBottom   bool
	scrollToHit  bool
	displayWidth int

	width, height int
}

// NewLogView builds the log viewer. When follow is true it starts tailing on
// Init, re-invoking fetch every few seconds and jumping to the bottom on new
// content.
func NewLogView(th theme.Theme, title string, fetch func(ctx context.Context) (string, error), follow bool) *LogView {
	return &LogView{
		th:     th,
		title:  title,
		fetch:  fetch,
		follow: follow,
		search: component.NewFilterBar(),
	}
}

// Init kicks off the first fetch and, when following, arms the poll ticker.
func (v *LogView) Init() tea.Cmd {
	cmds := []tea.Cmd{v.fetchCmd()}
	if v.follow {
		cmds = append(cmds, v.followTick())
	}
	return tea.Batch(cmds...)
}

// Close implements View.
func (v *LogView) Close() {}

// Title implements View.
func (v *LogView) Title() string { return v.title }

// CapturesKeys reports true only while the inline search prompt is open, so
// the app's global shortcuts don't steal typed characters.
func (v *LogView) CapturesKeys() bool { return v.searchOpen }

// Hints implements View.
func (v *LogView) Hints() []key.Binding {
	followHelp := "follow(off)"
	if v.follow {
		followHelp = "follow(on)"
	}
	wrapHelp := "wrap(off)"
	if v.wrap {
		wrapHelp = "wrap(on)"
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("f"), key.WithHelp("f", followHelp)),
		key.NewBinding(key.WithKeys("w"), key.WithHelp("w", wrapHelp)),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n/N", "next/prev match")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl-s", "save")),
	}
}

// Update routes lifecycle messages first, then keys.
func (v *LogView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case logLoadedMsg:
		return v.onLogLoaded(msg)
	case followTickMsg:
		return v.onFollowTick(msg)
	case logSavedMsg:
		if msg.err != nil {
			return v, flash(FlashError, "save: "+msg.err.Error())
		}
		return v, flash(FlashInfo, "saved "+msg.path)
	case tea.KeyPressMsg:
		return v.handleKey(msg)
	}
	return v, nil
}

func (v *LogView) handleKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	if v.searchOpen {
		return v.handleSearchKey(msg)
	}

	switch msg.String() {
	case "f":
		return v.toggleFollow()
	case "w":
		v.wrap = !v.wrap
		v.xoff = 0
		return v, nil
	case "/":
		v.searchOpen = true
		var cmd tea.Cmd
		v.search, cmd = v.search.Open(v.searchQuery)
		return v, cmd
	case "n":
		v.jumpMatch(1)
		return v, nil
	case "N":
		v.jumpMatch(-1)
		return v, nil
	case "h", "left":
		if !v.wrap && v.xoff > 0 {
			v.xoff--
		}
		return v, nil
	case "l", "right":
		if !v.wrap && v.xoff < v.maxXOff() {
			v.xoff++
		}
		return v, nil
	case "r", "ctrl+r":
		cmd := v.fetchCmd()
		return v, cmd
	case "ctrl+s":
		cmd := v.save()
		return v, cmd
	case "esc":
		if v.searchQuery != "" {
			v.searchQuery = ""
			v.matchLines = nil
			v.matchIdx = 0
			return v, nil
		}
		return v, func() tea.Msg { return PopMsg{} }
	}

	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	return v, cmd
}

func (v *LogView) handleSearchKey(msg tea.KeyPressMsg) (View, tea.Cmd) {
	var event component.Event
	var cmd tea.Cmd
	v.search, event, cmd = v.search.Update(msg)
	switch event.Kind {
	case component.EventSubmit:
		v.searchOpen = false
		v.searchQuery = event.Value
		v.matchIdx = 0
		v.scrollToHit = true
	case component.EventCancel:
		v.searchOpen = false
	case component.EventChanged, component.EventNone:
	}
	return v, cmd
}

func (v *LogView) toggleFollow() (View, tea.Cmd) {
	v.follow = !v.follow
	v.followGen++
	if v.follow {
		v.jumpBottom = true
		return v, tea.Batch(v.fetchCmd(), v.followTick())
	}
	return v, nil
}

// jumpMatch moves the current match cursor by delta and requests a scroll.
func (v *LogView) jumpMatch(delta int) {
	if len(v.matchLines) == 0 {
		return
	}
	v.matchIdx = (v.matchIdx + delta + len(v.matchLines)) % len(v.matchLines)
	v.scrollToHit = true
}

// fetchCmd runs the source fetch under a timeout, tagged with a fresh gen.
func (v *LogView) fetchCmd() tea.Cmd {
	v.gen++
	gen, fetch := v.gen, v.fetch
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		content, err := fetch(ctx)
		return logLoadedMsg{content: content, err: err, gen: gen}
	}
}

func (v *LogView) onLogLoaded(msg logLoadedMsg) (View, tea.Cmd) {
	if msg.gen != v.gen {
		return v, nil
	}
	v.loaded = true
	v.fetchedAt = time.Now()
	if msg.err != nil {
		v.err = msg.err
		return v, nil
	}
	v.err = nil
	v.content = msg.content
	if v.follow {
		v.jumpBottom = true
	}
	return v, nil
}

// followTick arms the follow poll timer for the current follow session.
func (v *LogView) followTick() tea.Cmd {
	g := v.followGen
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return followTickMsg{gen: g} })
}

func (v *LogView) onFollowTick(msg followTickMsg) (View, tea.Cmd) {
	if !v.follow || msg.gen != v.followGen {
		return v, nil
	}
	return v, tea.Batch(v.fetchCmd(), v.followTick())
}

// save writes the raw content to the XDG state dumps directory.
func (v *LogView) save() tea.Cmd {
	content, title := v.content, v.title
	return func() tea.Msg {
		name := fmt.Sprintf("%s-%d.log", sanitizeTitle(title), time.Now().Unix())
		path, err := xdg.StateFile(filepath.Join("lazydbx", "dumps", name))
		if err != nil {
			return logSavedMsg{err: err}
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return logSavedMsg{err: err}
		}
		return logSavedMsg{path: path}
	}
}

// sanitizeTitle reduces a title to a filesystem-safe slug.
func sanitizeTitle(title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "log"
	}
	return out
}

// displayLines splits content into the lines shown for the current wrap mode.
func (v *LogView) displayLines(width int) []string {
	raw := strings.Split(v.content, "\n")
	if !v.wrap || width <= 0 {
		return raw
	}
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		wrapped := ansi.Hardwrap(l, width, false)
		out = append(out, strings.Split(wrapped, "\n")...)
	}
	return out
}

// highlight wraps case-insensitive matches of v.searchQuery in a match style
// and records the indices of lines that contain at least one match.
func (v *LogView) highlight(lines []string) []string {
	v.matchLines = v.matchLines[:0]
	if v.searchQuery == "" {
		return lines
	}
	style := v.th.KeyHint.Reverse(true)
	lower := strings.ToLower(v.searchQuery)
	out := make([]string, len(lines))
	for i, l := range lines {
		if !strings.Contains(strings.ToLower(l), lower) {
			out[i] = l
			continue
		}
		v.matchLines = append(v.matchLines, i)
		out[i] = highlightLine(l, lower, style)
	}
	if v.matchIdx >= len(v.matchLines) {
		v.matchIdx = 0
	}
	return out
}

// highlightLine styles every case-insensitive occurrence of lowerQuery in line.
// It operates on the raw (unstyled) line, so it must not be given ANSI input.
func highlightLine(line, lowerQuery string, style lipgloss.Style) string {
	lowerLine := strings.ToLower(line)
	var b strings.Builder
	for {
		idx := strings.Index(lowerLine, lowerQuery)
		if idx < 0 {
			b.WriteString(line)
			break
		}
		b.WriteString(line[:idx])
		end := idx + len(lowerQuery)
		b.WriteString(style.Render(line[idx:end]))
		line = line[end:]
		lowerLine = lowerLine[end:]
	}
	return b.String()
}

func (v *LogView) maxXOff() int {
	if v.displayWidth <= v.width || v.width <= 0 {
		return 0
	}
	return v.displayWidth - v.width
}

// Render draws the search prompt (when open) and the content viewport.
func (v *LogView) Render(width, height int) string {
	v.width, v.height = width, height

	var top string
	vpHeight := height
	if v.searchOpen {
		top = v.search.View(v.th, width) + "\n"
		vpHeight--
	}
	if vpHeight < 1 {
		vpHeight = 1
	}

	if !v.vpOK {
		v.vp = viewport.New(viewport.WithWidth(width), viewport.WithHeight(vpHeight))
		v.vpOK = true
	} else {
		v.vp.SetWidth(width)
		v.vp.SetHeight(vpHeight)
	}

	switch {
	case !v.loaded && v.err == nil:
		v.vp.SetContent(v.th.Subtle.Render("loading…"))
		return top + v.vp.View()
	case v.err != nil && v.content == "":
		body := v.th.Error.Render("error: "+v.err.Error()) + "\n\n" +
			v.th.Subtle.Render("ctrl+r to retry, esc to go back")
		v.vp.SetContent(body)
		return top + v.vp.View()
	}

	lines := v.displayLines(width)
	v.displayWidth = 0
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > v.displayWidth {
			v.displayWidth = w
		}
	}

	lines = v.highlight(lines)

	if !v.wrap && v.xoff > v.maxXOff() {
		v.xoff = v.maxXOff()
	}
	if !v.wrap && v.xoff > 0 {
		for i, l := range lines {
			lines[i] = ansi.Cut(l, v.xoff, v.xoff+width)
		}
	}

	v.vp.SetContent(strings.Join(lines, "\n"))

	// Honor scroll intents after content is set (order: explicit match jump
	// wins over a follow-driven bottom jump).
	switch {
	case v.scrollToHit && len(v.matchLines) > 0:
		v.vp.SetYOffset(v.matchLines[v.matchIdx])
		v.scrollToHit = false
		v.jumpBottom = false
	case v.jumpBottom:
		v.vp.GotoBottom()
		v.jumpBottom = false
	}
	v.scrollToHit = false

	return top + v.vp.View()
}

// Status renders the right status segment: line count, follow indicator, age.
func (v *LogView) Status(now time.Time) string {
	if !v.loaded {
		return ""
	}
	n := 0
	if v.content != "" {
		n = strings.Count(v.content, "\n") + 1
	}
	parts := []string{strconv.Itoa(n) + " lines"}
	if v.follow {
		parts = append(parts, v.th.Warning.Render("following"))
	}
	age := now.Sub(v.fetchedAt).Round(time.Second)
	parts = append(parts, fmt.Sprintf("⟳ %s ago", age))
	line := strings.Join(parts, "  ")
	if v.err != nil {
		line = v.th.Error.Render(line + "  (refresh failed)")
	} else {
		line = v.th.Subtle.Render(line)
	}
	return line
}

// --- private lifecycle messages (see Update) ---

// logLoadedMsg carries the outcome of one fetch.
type logLoadedMsg struct {
	content string
	err     error
	gen     int
}

// followTickMsg fires on the follow poll timer for a follow session.
type followTickMsg struct{ gen int }

// logSavedMsg reports the outcome of a save-to-file.
type logSavedMsg struct {
	path string
	err  error
}
