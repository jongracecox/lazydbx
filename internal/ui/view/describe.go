package view

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"gopkg.in/yaml.v3"

	"github.com/jongracecox/lazydbx/internal/theme"
)

// Describe renders any detail object as scrollable YAML.
type Describe struct {
	th    theme.Theme
	title string
	body  string
	vp    viewport.Model
	ready bool
}

// NewDescribe marshals the object up front; marshal errors render inline.
func NewDescribe(th theme.Theme, title string, obj any) *Describe {
	body, err := yaml.Marshal(obj)
	if err != nil {
		return &Describe{th: th, title: title, body: fmt.Sprintf("failed to render: %v", err)}
	}
	return &Describe{th: th, title: title, body: string(body)}
}

// Init implements View.
func (d *Describe) Init() tea.Cmd { return nil }

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
