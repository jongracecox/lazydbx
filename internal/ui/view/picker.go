package view

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/theme"
	"github.com/jongracecox/lazydbx/internal/ui/component"
)

var pickerCols = []resource.ColSpec[dbx.Profile]{
	{Column: resource.Column{Title: "PROFILE"}, Extract: func(p dbx.Profile) string { return p.Name }},
	{Column: resource.Column{Title: "HOST"}, Extract: dbx.Profile.ShortHost},
	{Column: resource.Column{Title: "TYPE", Width: 10}, Extract: func(p dbx.Profile) string {
		if p.IsAccount() {
			return "account"
		}
		return "workspace"
	}},
	{Column: resource.Column{Title: "AUTH", Width: 16}, Extract: func(p dbx.Profile) string { return p.AuthType }},
}

// Picker selects a Databricks profile. It is the first screen when no
// --profile flag is given, and reachable any time via ctrl+p.
type Picker struct {
	th       theme.Theme
	profiles []dbx.Profile
	table    component.Table
}

// NewPicker builds the profile picker.
func NewPicker(th theme.Theme, profiles []dbx.Profile) *Picker {
	p := &Picker{th: th, profiles: profiles, table: component.NewTable(th)}
	p.table.SetData(resource.Cols(pickerCols), p.rows())
	return p
}

func (p *Picker) rows() []resource.Row {
	return resource.BuildRows(p.profiles, func(pr dbx.Profile) string { return pr.Name }, pickerCols)
}

// Init implements View.
func (p *Picker) Init() tea.Cmd { return nil }

// Close implements View.
func (p *Picker) Close() {}

// Title implements View.
func (p *Picker) Title() string { return "profiles" }

// Hints implements View.
func (p *Picker) Hints() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "use profile")),
	}
}

// Status implements StatusProvider.
func (p *Picker) Status(time.Time) string {
	return p.th.Subtle.Render("~/.databrickscfg")
}

// Update handles selection.
func (p *Picker) Update(msg tea.Msg) (View, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch kmsg.String() {
		case "enter":
			if row, ok := p.table.SelectedRow(); ok {
				profile := row.Data.(dbx.Profile)
				return p, func() tea.Msg { return ProfileSelectedMsg{Profile: profile} }
			}
			return p, nil
		case "esc":
			return p, func() tea.Msg { return PopMsg{} }
		}
	}
	var cmd tea.Cmd
	p.table, cmd = p.table.Update(msg)
	return p, cmd
}

// Render implements View.
func (p *Picker) Render(width, height int) string {
	p.table.SetSize(width, height)
	return p.table.View()
}
