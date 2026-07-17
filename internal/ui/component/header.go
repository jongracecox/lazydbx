package component

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/jongracecox/lazydbx/internal/theme"
	"github.com/jongracecox/lazydbx/internal/version"
)

// hintRows is how many rows the key-hint block may occupy.
const hintRows = 4

// Banner is the top-right logo, k9s-style. Shown only on terminals with
// room to spare.
const Banner = `▄▄                   ▄▄▄▄▄▄   ▄▄▄▄▄▄▄   ▄▄▄   ▄▄▄
██                   ███▀▀██▄ ███▀▀███▄ ████▄████
██  ▀▀█▄ ▀▀▀██ ██ ██ ███  ███ ███▄▄███▀  ▀█████▀
██ ▄█▀██   ▄█▀ ██▄██ ███  ███ ███  ███▄ ▄███████▄
██ ▀█▄██ ▄██▄▄  ▀██▀ ██████▀  ████████▀ ███▀ ▀███
                 ██
               ▀▀▀`

// Banner display thresholds.
const (
	bannerMinWidth  = 100
	bannerMinHeight = 24
)

// Header renders the top chrome: identity line plus a k9s-style grid of the
// key hints valid right now, with the logo banner on the right when the
// terminal has room. Its height varies — measure with lipgloss.Height.
func Header(th theme.Theme, width, height int, context string, badges []string, hints []key.Binding) string {
	contentWidth := width
	var bannerBlock string
	showBanner := width >= bannerMinWidth && height >= bannerMinHeight
	if showBanner {
		bannerBlock = renderBanner(th)
		contentWidth = width - lipgloss.Width(Banner) - 2
	}

	var b strings.Builder
	var line string
	if !showBanner {
		// The Banner carries the app identity; without it, fall back to
		// the compact name chip.
		line = th.Logo.Render(" lazydbx ") + " "
	}
	line += th.Title.Render(context)
	for _, badge := range badges {
		line += "  " + th.Warning.Render("["+badge+"]")
	}
	b.WriteString(lipgloss.NewStyle().MaxWidth(contentWidth).Render(line))
	b.WriteString("\n")
	b.WriteString(renderHints(th, contentWidth, hints))

	if bannerBlock == "" {
		return b.String()
	}
	left := lipgloss.NewStyle().Width(contentWidth + 2).Render(b.String())
	return lipgloss.JoinHorizontal(lipgloss.Top, left, bannerBlock)
}

// renderBanner styles the logo and tucks the app name + version into the
// bottom-right, one line above the base (beside the y descender).
func renderBanner(th theme.Theme) string {
	lines := strings.Split(Banner, "\n")
	bannerWidth := lipgloss.Width(Banner)
	tag := "lazydbx " + version.Version

	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = th.Logo.Render(l)
	}
	tagLine := len(lines) - 2
	if pad := bannerWidth - lipgloss.Width(lines[tagLine]) - lipgloss.Width(tag); pad >= 1 {
		out[tagLine] = th.Logo.Render(lines[tagLine]) + strings.Repeat(" ", pad) + th.Subtle.Render(tag)
	}
	return strings.Join(out, "\n")
}

// renderHints lays bindings out in columns, k9s-style: fill down each
// column, up to hintRows rows.
func renderHints(th theme.Theme, width int, hints []key.Binding) string {
	visible := make([]key.Binding, 0, len(hints))
	for _, h := range hints {
		if h.Enabled() && h.Help().Key != "" {
			visible = append(visible, h)
		}
	}
	if len(visible) == 0 {
		return strings.Repeat("\n", hintRows-1)
	}

	cols := (len(visible) + hintRows - 1) / hintRows
	cells := make([][]string, cols)
	for i := range cells {
		lo := i * hintRows
		hi := min(lo+hintRows, len(visible))
		var colWidth int
		for _, h := range visible[lo:hi] {
			colWidth = max(colWidth, len(h.Help().Key))
		}
		for _, h := range visible[lo:hi] {
			keyStr := th.KeyHint.Render("<" + h.Help().Key + ">" + strings.Repeat(" ", colWidth-len(h.Help().Key)))
			cells[i] = append(cells[i], keyStr+" "+th.KeyLabel.Render(h.Help().Desc))
		}
	}

	columns := make([]string, cols)
	for i, cell := range cells {
		columns[i] = strings.Join(cell, "\n")
	}
	joined := lipgloss.JoinHorizontal(lipgloss.Top, interleave(columns, "   ")...)
	// Pad to a fixed height so the body doesn't jump when hints change.
	lines := strings.Split(joined, "\n")
	for len(lines) < hintRows {
		lines = append(lines, "")
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = lipgloss.NewStyle().MaxWidth(width).Render(l)
	}
	return strings.Join(out[:hintRows], "\n")
}

func interleave(items []string, sep string) []string {
	out := make([]string, 0, len(items)*2)
	for i, item := range items {
		if i > 0 {
			out = append(out, sep)
		}
		out = append(out, item)
	}
	return out
}
