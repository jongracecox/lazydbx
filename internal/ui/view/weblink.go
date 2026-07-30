package view

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// WebLink is the workspace page a detail view corresponds to — the `o` out-link
// on screens that aren't a Browser (logs, describe, SQL results). The Browser
// derives its link per row from resource.WebLinker; these views can't (they have
// no def), so whoever opens them supplies the URL. A zero value means no link
// and `o` stays unbound.
type WebLink struct {
	URL string
	// Hint labels the `o` binding in the header; defaults to defaultWebHint.
	Hint string
}

const defaultWebHint = "open in browser"

// WebLinkSetter is implemented by views that accept a WebLink (via the embedded
// webLink). The app uses it to attach the link after constructing a view from a
// message, without caring which concrete view it built.
type WebLinkSetter interface {
	SetWebLink(WebLink)
}

// webLink is embedded by views supporting the `o` out-link. It keeps the key,
// the hint, and the "no link → don't bind" rule in one place so every detail
// view behaves identically.
type webLink struct{ link WebLink }

// SetWebLink attaches (or, with a zero value, clears) the view's out-link.
func (w *webLink) SetWebLink(l WebLink) { w.link = l }

// webHints returns the `o` binding when a link is set, else nil — callers
// append it unconditionally.
func (w *webLink) webHints() []key.Binding {
	if w.link.URL == "" {
		return nil
	}
	hint := w.link.Hint
	if hint == "" {
		hint = defaultWebHint
	}
	return []key.Binding{key.NewBinding(key.WithKeys("o"), key.WithHelp("o", hint))}
}

// openWeb returns the command opening the link in the system browser, or nil
// when there is no link — so the caller falls through to its normal handling of
// the key instead of swallowing it.
func (w *webLink) openWeb() tea.Cmd {
	if w.link.URL == "" {
		return nil
	}
	url := w.link.URL
	return func() tea.Msg { return OpenURLMsg{URL: url} }
}
