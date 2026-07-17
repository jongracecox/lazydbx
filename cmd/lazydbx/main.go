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

	root := &cobra.Command{
		Use:   "lazydbx [resource [args...]] [/filter]",
		Short: "A lazier way to Databricks — a fast terminal UI",
		Long: "A lazier way to Databricks — a fast terminal UI.\n\n" +
			"Optional positional args launch straight into a resource view, using the\n" +
			"same syntax as the in-app ':' command bar:\n\n" +
			"  lazydbx -p DEV jobs                 # open in the jobs list\n" +
			"  lazydbx -p DEV schemas prod         # schemas in the 'prod' catalog\n" +
			"  lazydbx -p DEV tables main.silver   # drill straight to a schema's tables\n" +
			"  lazydbx -p DEV runs 123             # runs for job 123\n" +
			"  lazydbx -p DEV jobs /etl            # jobs list pre-filtered to 'etl'\n\n" +
			"esc from a launched view returns to the profile picker.",
		// ArbitraryArgs lets positional launch args fall through to RunE while
		// still routing `lazydbx version` to its subcommand.
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
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

			return run(cmd, cfg, strings.Join(args, " "))
		},
	}

	root.Flags().StringVarP(&flags.Profile, "profile", "p", "", "Databricks config profile to use")
	root.Flags().BoolVar(&flags.ReadOnly, "readonly", false, "disable all mutating actions")
	root.Flags().StringVar(&flags.LogLevel, "log-level", "", "log level: debug, info, warn, error")
	root.Flags().StringVar(&flags.ConfigFile, "config", "", "path to config file (default "+config.Path()+")")

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(*cobra.Command, []string) {
			fmt.Println(version.String())
		},
	})

	return root
}

func run(cmd *cobra.Command, cfg config.Config, launch string) error {
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

	registry := resources.NewRegistry()
	launch = normalizeLaunch(registry, launch)
	// Validate the launch command up front so a typo prints a clean error to
	// stderr rather than a flash behind the alt-screen (mirrors app.launchView).
	if err := validateLaunch(registry, launch); err != nil {
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

	m := app.New(cfg, profiles, registry, dbx.NewPool(), eng, launch)
	p := tea.NewProgram(m, tea.WithContext(cmd.Context()))
	program.Store(p)

	_, err = p.Run()
	return err
}

// validateLaunch checks a positional launch command before the TUI starts.
// Empty (no args) and the special `sql` command are always accepted; anything
// else must parse as a resource command. Keep in sync with app.launchView.
func validateLaunch(reg *resource.Registry, launch string) error {
	launch = strings.TrimSpace(launch)
	if launch == "" || launch == "sql" || strings.HasPrefix(launch, "sql ") {
		return nil
	}
	_, err := reg.Parse(launch)
	return err
}

// normalizeLaunch applies launch-command sugar. `apps <name>` (or its alias) is
// rewritten to `apps /<name>` so a bare app name lands directly on that app in
// the list — apps is unscoped, so a positional would otherwise be an error.
// Other commands and the explicit `apps /filter` form pass through unchanged.
func normalizeLaunch(reg *resource.Registry, launch string) string {
	fields := strings.Fields(strings.TrimSpace(launch))
	if len(fields) != 2 || strings.HasPrefix(fields[1], "/") {
		return launch
	}
	if def, ok := reg.Get(fields[0]); ok && def.Name() == "apps" {
		return fields[0] + " /" + fields[1]
	}
	return launch
}
