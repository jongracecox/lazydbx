// Command lazydbx is a k9s-style terminal UI for Databricks.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/adrg/xdg"
	"github.com/spf13/cobra"

	"github.com/jongracecox/lazydbx/internal/app"
	"github.com/jongracecox/lazydbx/internal/config"
	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/engine"
	"github.com/jongracecox/lazydbx/internal/logging"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/resources"
	"github.com/jongracecox/lazydbx/internal/version"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var flags config.Flags
	var launchTab string

	// Built once and shared with completion, which resolves resource/scope
	// names without starting the TUI.
	registry := resources.NewRegistry()

	root := &cobra.Command{
		Use:   "lazydbx [resource [args...]] [item] [/filter]",
		Short: "A lazier way to Databricks — a fast terminal UI",
		Long: "A lazier way to Databricks — a fast terminal UI.\n\n" +
			"Optional positional args launch straight into a resource view, using the\n" +
			"same syntax as the in-app ':' command bar:\n\n" +
			"  lazydbx -p DEV jobs                 # open in the jobs list\n" +
			"  lazydbx -p DEV schemas prod         # schemas in the 'prod' catalog\n" +
			"  lazydbx -p DEV tables main.silver   # drill straight to a schema's tables\n" +
			"  lazydbx -p DEV jobs /etl            # jobs list pre-filtered to 'etl'\n\n" +
			"A trailing item name opens that item directly (jobs match by name):\n\n" +
			"  lazydbx -p DEV apps my-app                    # open the app 'my-app'\n" +
			"  lazydbx -p DEV jobs 'Nightly ETL'             # open a job by name\n" +
			"  lazydbx -p DEV tables main.silver orders      # open the 'orders' table\n\n" +
			"--tab opens a specific tab of such an item:\n\n" +
			"  lazydbx -p DEV apps my-app --tab logs         # app, on its logs tab\n" +
			"  lazydbx -p DEV tables main.silver orders --tab data  # table, on its data tab\n\n" +
			"esc from a launched view returns to the profile picker.\n\n" +
			"Shell completion (resource, scope, and item names — the latter live from\n" +
			"the workspace when -p is given) is available via `lazydbx completion`.",
		// ArbitraryArgs lets positional launch args fall through to RunE while
		// still routing `lazydbx version` to its subcommand.
		Args:              cobra.ArbitraryArgs,
		SilenceUsage:      true,
		SilenceErrors:     true,
		ValidArgsFunction: completeArgs(registry),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags)
			if err != nil {
				return err
			}
			logPath, closeLog, err := logging.Setup(cfg.LogLevel)
			if err != nil {
				return err
			}
			defer closeLog() //nolint:errcheck // best-effort close on exit
			slog.Info("starting", "version", version.Version, "log", logPath)

			return run(cmd, cfg, registry, args, launchTab)
		},
	}

	root.Flags().StringVarP(&flags.Profile, "profile", "p", "", "Databricks config profile to use")
	root.Flags().BoolVar(&flags.ReadOnly, "readonly", false, "disable all mutating actions")
	root.Flags().StringVar(&flags.LogLevel, "log-level", "", "log level: debug, info, warn, error")
	root.Flags().StringVar(&flags.ConfigFile, "config", "", "path to config file (default "+config.Path()+")")
	root.Flags().StringVar(&launchTab, "tab", "", "tab to open when launching into a specific item (e.g. logs, data)")

	registerCompletion(root, registry)

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(*cobra.Command, []string) {
			fmt.Println(version.String())
		},
	})
	root.AddCommand(prefetchCmd(registry))

	return root
}

func run(cmd *cobra.Command, cfg config.Config, registry *resource.Registry, launchArgs []string, launchTab string) error {
	cfgPath, err := dbx.ConfigPath()
	if err != nil {
		return err
	}
	profiles, err := dbx.LoadProfiles(cfgPath)
	if err != nil {
		return fmt.Errorf("no Databricks profiles found (%w) — create one with `databricks configure`", err)
	}
	if len(profiles) == 0 {
		return fmt.Errorf("no usable profiles in %s — create one with `databricks configure`", cfgPath)
	}

	// Validate the launch command up front so a typo prints a clean error to
	// stderr rather than a flash behind the alt-screen (mirrors app.launchView).
	if err := validateLaunch(registry, launchArgs, launchTab); err != nil {
		return err
	}

	// The engine needs p.Send before the program exists; route through an
	// atomic pointer set immediately after construction. Pollers only start
	// once views Init inside Run, so no event can precede the store.
	var program atomic.Pointer[tea.Program]
	store := engine.NewStore(filepath.Join(xdg.CacheHome, "lazydbx"))
	eng := engine.New(func(ev engine.DataEvent) {
		if p := program.Load(); p != nil {
			p.Send(ev)
		}
	}, store)
	defer eng.Stop()

	m := app.New(cfg, profiles, registry, dbx.NewPool(), eng, launchArgs, launchTab)
	p := tea.NewProgram(m, tea.WithContext(cmd.Context()))
	program.Store(p)

	_, err = p.Run()
	return err
}

// validateLaunch checks a positional launch command (and any --tab) before the
// TUI starts. No args and the special `sql` command are always accepted as
// launch commands; anything else must parse as a resource command. A --tab
// selection additionally requires a resource with named tabs and a specific
// item to open. Keep in sync with app.launchView.
func validateLaunch(reg *resource.Registry, args []string, tab string) error {
	tab = strings.TrimSpace(tab)
	if len(args) == 0 || args[0] == "sql" {
		if tab != "" {
			return fmt.Errorf("--tab requires launching into a resource item, e.g. `apps <name> --tab %s`", tab)
		}
		return nil
	}
	cmd, err := reg.ParseArgs(args)
	if err != nil {
		return err
	}
	if tab != "" {
		return validateTab(cmd, tab)
	}
	return nil
}

// validateTab checks a --tab selection against a parsed launch command: the
// resource must expose named tabs (resource.Tabber) and name a specific item
// to open (a trailing positional / Item), and tab must be one of the
// resource's tab names (case-insensitive).
func validateTab(cmd resource.Command, tab string) error {
	tabber, ok := cmd.Def.(resource.Tabber)
	if !ok {
		return fmt.Errorf("--tab is not supported by %s (it has no tabs)", cmd.Def.Name())
	}
	if cmd.Item == "" {
		return fmt.Errorf("--tab requires naming a single %s, e.g. `%s <name> --tab %s`", cmd.Def.Name(), cmd.Def.Name(), tab)
	}
	tabs := tabber.Tabs()
	for _, t := range tabs {
		if strings.EqualFold(t, tab) {
			return nil
		}
	}
	return fmt.Errorf("unknown tab %q for %s (valid: %s)", tab, cmd.Def.Name(), strings.Join(tabs, ", "))
}
