package view

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jongracecox/lazydbx/internal/theme"
)

// renderDetail renders any object as YAML-ish text for the describe view.
// Unlike yaml.Marshal, multi-line strings are emitted as indented raw blocks
// (yaml falls back to `"...\n..."` quoting when lines start with spaces,
// which is unreadable for markdown-style comments). Keys are theme-accented.
func renderDetail(th theme.Theme, obj any) string {
	var node yaml.Node
	if err := node.Encode(obj); err != nil {
		return fmt.Sprintf("failed to render: %v", err)
	}
	var b strings.Builder
	writeNode(&b, th, &node, 0)
	return b.String()
}

const indentStep = 2

func writeNode(b *strings.Builder, th theme.Theme, n *yaml.Node, indent int) {
	switch n.Kind {
	case yaml.DocumentNode:
		for _, c := range n.Content {
			writeNode(b, th, c, indent)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i], n.Content[i+1]
			b.WriteString(strings.Repeat(" ", indent))
			b.WriteString(th.KeyHint.Render(key.Value + ":"))
			writeValue(b, th, val, indent)
		}
	case yaml.SequenceNode:
		for _, item := range n.Content {
			b.WriteString(strings.Repeat(" ", indent))
			b.WriteString(th.Subtle.Render("-"))
			writeValue(b, th, item, indent)
		}
	case yaml.ScalarNode:
		writeScalar(b, n.Value, indent)
	case yaml.AliasNode:
		if n.Alias != nil {
			writeNode(b, th, n.Alias, indent)
		}
	}
}

// writeValue writes a mapping value or sequence item following its "key:"
// or "-" prefix, recursing with increased indentation for nested nodes.
func writeValue(b *strings.Builder, th theme.Theme, n *yaml.Node, indent int) {
	switch {
	case n.Kind == yaml.ScalarNode:
		writeScalar(b, n.Value, indent)
	case len(n.Content) == 0:
		b.WriteString("\n")
	default:
		b.WriteString("\n")
		writeNode(b, th, n, indent+indentStep)
	}
}

// writeScalar writes a scalar after its prefix: single-line values inline,
// multi-line values as an indented raw block so markdown comments read
// naturally.
func writeScalar(b *strings.Builder, value string, indent int) {
	if value == "" {
		b.WriteString("\n")
		return
	}
	if !strings.Contains(value, "\n") {
		b.WriteString(" " + value + "\n")
		return
	}
	b.WriteString("\n")
	pad := strings.Repeat(" ", indent+indentStep)
	for _, line := range strings.Split(value, "\n") {
		b.WriteString(pad + line + "\n")
	}
}
