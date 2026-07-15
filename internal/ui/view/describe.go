package view

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/jongracecox/lazydbx/internal/theme"
)

// Describe renders any detail object as scrollable YAML.
type Describe struct {
	th    theme.Theme
	title string
	body  string
	vp    viewport.Model
	ready bool
	fetch func(ctx context.Context) (any, error)
}

// NewDescribe renders the object up front via the YAML-ish detail renderer
// (multi-line strings become readable indented blocks, keys are accented).
func NewDescribe(th theme.Theme, title string, obj any) *Describe {
	return &Describe{th: th, title: title, body: renderDetail(th, obj)}
}

// detailLoadedMsg carries a lazy describe fetch result.
type detailLoadedMsg struct {
	target *Describe
	obj    any
	err    error
}

// NewLazyDescribe fetches the object on Init — used where the detail isn't
// known up front (e.g. the details tab of the table view).
func NewLazyDescribe(th theme.Theme, title string, fetch func(ctx context.Context) (any, error)) *Describe {
	d := &Describe{th: th, title: title, body: "loading…", fetch: fetch}
	return d
}

// Init implements View; lazy instances start their fetch.
func (d *Describe) Init() tea.Cmd {
	if d.fetch == nil {
		return nil
	}
	fetch := d.fetch
	target := d
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		obj, err := fetch(ctx)
		return detailLoadedMsg{target: target, obj: obj, err: err}
	}
}

// Close implements View.
func (d *Describe) Close() {}

// Title implements View.
func (d *Describe) Title() string { return d.title }

// Hints implements View.
func (d *Describe) Hints() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("j"), key.WithHelp("j/k", "scroll")),
	}
}

// Update scrolls the viewport.
func (d *Describe) Update(msg tea.Msg) (View, tea.Cmd) {
	if loaded, ok := msg.(detailLoadedMsg); ok && loaded.target == d {
		if loaded.err != nil {
			d.body = d.th.Error.Render("failed to load: " + loaded.err.Error())
		} else {
			d.body = renderDetail(d.th, loaded.obj)
		}
		if d.ready {
			d.vp.SetContent(d.body)
		}
		return d, nil
	}
	if kmsg, ok := msg.(tea.KeyPressMsg); ok && kmsg.String() == "esc" {
		return d, func() tea.Msg { return PopMsg{} }
	}
	var cmd tea.Cmd
	d.vp, cmd = d.vp.Update(msg)
	return d, cmd
}

// Render implements View.
func (d *Describe) Render(width, height int) string {
	if !d.ready {
		d.vp = viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
		d.vp.SetContent(d.body)
		d.ready = true
	} else {
		d.vp.SetWidth(width)
		d.vp.SetHeight(height)
	}
	return d.vp.View()
}
