package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/spf13/cobra"

	"github.com/jongracecox/lazydbx/internal/config"
	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/engine"
	"github.com/jongracecox/lazydbx/internal/resource"
)

// Shell completion. Cobra re-invokes the binary in a hidden `__complete` mode
// (RunE never runs, so the TUI never starts) and reads candidate lines from
// stdout. Resource and tab names come from the registry; scope and item names
// are listed from the workspace when a profile is resolvable.
//
// Completion never blocks on the network: it serves whatever is in the on-disk
// cache (reused from the engine's Store, shared with the running TUI) and, when
// that entry is missing or stale, spawns a detached background prefetch so the
// NEXT press is warm. A cold entry therefore completes to nothing the first
// time; the TUI warms the same cache as you browse.

const (
	// completionCacheTTL bounds how long a cached listing is served before a
	// background refresh is triggered. Catalog/schema/table/job/app names change
	// slowly, so a generous TTL keeps completion instant.
	completionCacheTTL = 5 * time.Minute
	// prefetchTimeout caps the detached background fetch.
	prefetchTimeout = 20 * time.Second
	// prefetchCmdName is the hidden subcommand the background prefetch runs as.
	prefetchCmdName = "__prefetch"
)

// completeArgs completes positional launch args: the resource name, then its
// scope args, then the item name — all bare, so the completed line parses.
func completeArgs(reg *resource.Registry) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return filterPrefix(append(reg.Canonical(), "sql"), toComplete), cobra.ShellCompDirectiveNoFileComp
		}
		def, ok := reg.Get(args[0])
		if !ok {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		want := def.Args()
		provided := args[1:]

		switch {
		case len(provided) < len(want): // completing a scope arg
			prof, hasProf := profileForCompletion(cmd)
			if !hasProf {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			idx := len(provided)
			lister, scope, ok := scopeArgLister(reg, want, provided, idx)
			if !ok {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return filterPrefix(completeRemote(prof, lister, scope, provided[:idx]), toComplete), cobra.ShellCompDirectiveNoFileComp

		case len(provided) == len(want): // completing the item's own name
			prof, hasProf := profileForCompletion(cmd)
			if !hasProf {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return filterPrefix(completeRemote(prof, def, zipScope(want, provided), provided), toComplete), cobra.ShellCompDirectiveNoFileComp

		default: // item already provided — a further bare arg would not parse
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}
}

// registerCompletion wires flag-value completion: profile names, tab names for
// the resource on the line, and the fixed log levels.
func registerCompletion(root *cobra.Command, reg *resource.Registry) {
	_ = root.RegisterFlagCompletionFunc("profile", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return filterPrefix(profileNames(), toComplete), cobra.ShellCompDirectiveNoFileComp
	})
	_ = root.RegisterFlagCompletionFunc("log-level", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return filterPrefix([]string{"debug", "info", "warn", "error"}, toComplete), cobra.ShellCompDirectiveNoFileComp
	})
	_ = root.RegisterFlagCompletionFunc("tab", func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return filterPrefix(tabNames(reg, args), toComplete), cobra.ShellCompDirectiveNoFileComp
	})
}

// prefetchCmd is the hidden subcommand a background prefetch runs as:
// `lazydbx __prefetch <profile> <resource> [scopeVals...]`. It lists the
// resource once and writes the result to the completion cache, then exits. It
// is silent and best-effort — every failure path just returns.
func prefetchCmd(reg *resource.Registry) *cobra.Command {
	return &cobra.Command{
		Use:           prefetchCmdName,
		Hidden:        true,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			profileName, resourceName, scopeVals := args[0], args[1], args[2:]
			def, ok := reg.Get(resourceName)
			if !ok {
				return nil
			}
			prof, ok := profileByName(profileName)
			if !ok {
				return nil
			}
			scope := zipScope(def.Args(), scopeVals)
			ctx, cancel := context.WithTimeout(context.Background(), prefetchTimeout)
			defer cancel()
			rows, err := def.List(ctx, dbx.NewClients(prof), scope)
			if err != nil {
				return nil
			}
			completionStore().Save(engine.Key{Profile: prof.Name, Resource: def.Name(), Scope: scope.Hash()}, rows, time.Now())
			return nil
		},
	}
}

// scopeArgLister finds the resource that lists candidate values for scope arg
// want[idx], plus the scope it should be listed under (the earlier args). The
// lister is the resource whose singular name matches the arg key and whose own
// arg count equals idx (e.g. arg "catalog" at idx 0 → catalogs; "schema" at
// idx 1 → schemas scoped by the chosen catalog).
func scopeArgLister(reg *resource.Registry, want, provided []string, idx int) (resource.Def, resource.Scope, bool) {
	key := want[idx]
	for _, name := range reg.Canonical() {
		def, ok := reg.Get(name)
		if ok && len(def.Args()) == idx && singular(def.Name()) == key {
			return def, zipScope(want[:idx], provided[:idx]), true
		}
	}
	return nil, nil, false
}

// completeRemote returns a resource's item names for completion, served from
// the on-disk cache and never blocking on the network. When the cache entry is
// missing or stale it spawns a detached background prefetch (so the next press
// is warm) and returns whatever is cached now (possibly nothing). Candidates
// are the rows' human names (resource.RowNamer) or IDs, de-duplicated.
func completeRemote(prof dbx.Profile, def resource.Def, scope resource.Scope, scopeVals []string) []string {
	key := engine.Key{Profile: prof.Name, Resource: def.Name(), Scope: scope.Hash()}
	rows, at, ok := completionStore().Load(key)
	if !ok || time.Since(at) >= completionCacheTTL {
		spawnPrefetch(prof.Name, def.Name(), scopeVals)
	}
	if !ok {
		return nil
	}
	name := rowNamer(def)
	seen := map[string]bool{}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		n := name(r)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// spawnPrefetch launches a detached `__prefetch` child that refreshes the
// completion cache, returning immediately so the shell never waits on it.
func spawnPrefetch(profileName, resourceName string, scopeVals []string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	// context.Background (not the completion request's context): the child must
	// outlive this __complete process to refresh the cache for the next press.
	c := exec.CommandContext(context.Background(), exe, append([]string{prefetchCmdName, profileName, resourceName}, scopeVals...)...) //nolint:gosec // fixed args, self-exec
	c.Stdin, c.Stdout, c.Stderr = nil, nil, nil
	detach(c)
	_ = c.Start() // fire-and-forget; the child outlives us and writes the cache
}

// completionStore roots the completion cache at the same directory the TUI's
// engine uses, so browsing in the app warms completion and vice versa.
func completionStore() *engine.Store {
	return engine.NewStore(filepath.Join(xdg.CacheHome, "lazydbx"))
}

// rowNamer returns the projection from a row to its completion candidate: the
// def's human name (resource.RowNamer) when it has one, else the row ID.
func rowNamer(def resource.Def) func(resource.Row) string {
	if namer, ok := def.(resource.RowNamer); ok {
		return namer.RowName
	}
	return func(r resource.Row) string { return r.ID }
}

// profileForCompletion resolves the profile completion should list against:
// the --profile flag on the line, else $DATABRICKS_CONFIG_PROFILE, else the
// config file's profile. ok is false when none resolves to a known profile.
func profileForCompletion(cmd *cobra.Command) (dbx.Profile, bool) {
	name, _ := cmd.Flags().GetString("profile")
	if name == "" {
		name = os.Getenv("DATABRICKS_CONFIG_PROFILE")
	}
	if name == "" {
		if cfg, err := config.Load(config.Flags{}); err == nil {
			name = cfg.Profile
		}
	}
	if name == "" {
		return dbx.Profile{}, false
	}
	return profileByName(name)
}

// profileByName finds a configured profile by exact name.
func profileByName(name string) (dbx.Profile, bool) {
	for _, p := range loadProfiles() {
		if p.Name == name {
			return p, true
		}
	}
	return dbx.Profile{}, false
}

// profileNames lists configured profile names for --profile completion.
func profileNames() []string {
	profiles := loadProfiles()
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}
	return names
}

// tabNames returns the tab names of the resource named first in args, for
// --tab completion; empty when unresolved or the resource has no tabs.
func tabNames(reg *resource.Registry, args []string) []string {
	if len(args) == 0 {
		return nil
	}
	def, ok := reg.Get(args[0])
	if !ok {
		return nil
	}
	if tabber, ok := def.(resource.Tabber); ok {
		return tabber.Tabs()
	}
	return nil
}

// loadProfiles reads the configured profiles, returning nil on any error
// (completion is best-effort).
func loadProfiles() []dbx.Profile {
	path, err := dbx.ConfigPath()
	if err != nil {
		return nil
	}
	profiles, err := dbx.LoadProfiles(path)
	if err != nil {
		return nil
	}
	return profiles
}

// zipScope pairs scope keys with their provided values (up to the shorter of
// the two).
func zipScope(keys, vals []string) resource.Scope {
	n := len(keys)
	if len(vals) < n {
		n = len(vals)
	}
	scope := make(resource.Scope, n)
	for i := 0; i < n; i++ {
		scope[keys[i]] = vals[i]
	}
	return scope
}

// singular strips a trailing plural "s" from a resource name so it matches the
// scope-arg key it lists (catalogs→catalog, jobs→job).
func singular(name string) string { return strings.TrimSuffix(name, "s") }

// filterPrefix keeps candidates matching the current partial word. Cobra's
// shell scripts also filter, but doing it here keeps behavior identical across
// shells and avoids emitting obviously-irrelevant candidates.
func filterPrefix(cands []string, prefix string) []string {
	if prefix == "" {
		return cands
	}
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}
