package resource

import (
	"fmt"
	"sort"
	"strings"
)

// Registry resolves command names and aliases to resource defs.
type Registry struct {
	defs    map[string]Def // canonical name → def
	aliases map[string]string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{defs: map[string]Def{}, aliases: map[string]string{}}
}

// MustRegister adds a def, panicking on name/alias collisions — collisions
// are programmer error and should fail at startup, loudly.
func (r *Registry) MustRegister(d Def) {
	name := d.Name()
	if _, ok := r.defs[name]; ok {
		panic(fmt.Sprintf("resource %q registered twice", name))
	}
	if existing, ok := r.aliases[name]; ok {
		panic(fmt.Sprintf("resource %q collides with alias of %q", name, existing))
	}
	r.defs[name] = d
	for _, alias := range d.Aliases() {
		if _, ok := r.defs[alias]; ok {
			panic(fmt.Sprintf("alias %q of %q collides with resource name", alias, name))
		}
		if owner, ok := r.aliases[alias]; ok {
			panic(fmt.Sprintf("alias %q of %q already owned by %q", alias, name, owner))
		}
		r.aliases[alias] = name
	}
}

// Get resolves a name or alias; ok is false when unknown.
func (r *Registry) Get(nameOrAlias string) (Def, bool) {
	if canonical, ok := r.aliases[nameOrAlias]; ok {
		nameOrAlias = canonical
	}
	d, ok := r.defs[nameOrAlias]
	return d, ok
}

// Names returns all canonical names plus aliases, sorted — the completion
// source for the command bar.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.defs)+len(r.aliases))
	for name := range r.defs {
		names = append(names, name)
	}
	for alias := range r.aliases {
		names = append(names, alias)
	}
	sort.Strings(names)
	return names
}

// Canonical returns only the canonical resource names, sorted — shown when
// the command bar opens empty, so every resource is discoverable.
func (r *Registry) Canonical() []string {
	names := make([]string, 0, len(r.defs))
	for name := range r.defs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Summaries renders one line per resource — "name (alias1, alias2) [args]" —
// for the help view.
func (r *Registry) Summaries() []string {
	out := make([]string, 0, len(r.defs))
	for _, name := range r.Canonical() {
		def := r.defs[name]
		line := name
		if aliases := def.Aliases(); len(aliases) > 0 {
			line += " (" + strings.Join(aliases, ", ") + ")"
		}
		if args := def.Args(); len(args) > 0 {
			line += "  <" + strings.Join(args, "> <") + ">"
		}
		out = append(out, line)
	}
	return out
}

// Command is a parsed `:` command line.
type Command struct {
	Def    Def
	Scope  Scope
	Filter string // pre-seeded filter from a trailing /text
}

// Parse interprets a command line like:
//
//	tables main silver
//	tables main.silver
//	tables main silver /events
//
// Positional args map onto Def.Args(); a single dotted arg is sugar for the
// full positional list; a trailing /text pre-seeds the view filter.
func (r *Registry) Parse(input string) (Command, error) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return Command{}, fmt.Errorf("empty command")
	}

	def, ok := r.Get(fields[0])
	if !ok {
		return Command{}, fmt.Errorf("unknown resource %q", fields[0])
	}

	args := fields[1:]
	var filter string
	if n := len(args); n > 0 && strings.HasPrefix(args[n-1], "/") {
		filter = strings.TrimPrefix(args[n-1], "/")
		args = args[:n-1]
	}

	want := def.Args()
	// Dotted sugar: `tables main.silver` ≡ `tables main silver`.
	if len(args) == 1 && len(want) > 1 && strings.Contains(args[0], ".") {
		args = strings.Split(args[0], ".")
	}
	if len(args) > len(want) {
		return Command{}, fmt.Errorf("%s takes at most %d args (%s)", def.Name(), len(want), strings.Join(want, " "))
	}
	if len(args) < len(want) {
		missing := want[len(args):]
		return Command{}, fmt.Errorf("%s requires %s", def.Name(), strings.Join(missing, ", "))
	}

	scope := Scope{}
	for i, arg := range args {
		scope[want[i]] = arg
	}
	return Command{Def: def, Scope: scope, Filter: filter}, nil
}

// Complete returns registered names with the given prefix, for autocomplete.
func (r *Registry) Complete(prefix string) []string {
	var out []string
	for _, name := range r.Names() {
		if strings.HasPrefix(name, prefix) {
			out = append(out, name)
		}
	}
	return out
}
