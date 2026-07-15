// Command lazydbx is a k9s-style terminal UI for Databricks.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/jongracecox/lazydbx/internal/app"
	"github.com/jongracecox/lazydbx/internal/config"
	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/engine"
	"github.com/jongracecox/lazydbx/internal/logging"
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
		Use:           "lazydbx",
		Short:         "A lazier way to Databricks — a fast terminal UI",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
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

			return run(cmd, cfg)
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

func run(cmd *cobra.Command, cfg config.Config) error {
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

	// The engine needs p.Send before the program exists; route through an
	// atomic pointer set immediately after construction. Pollers only start
	// once views Init inside Run, so no event can precede the store.
	var program atomic.Pointer[tea.Program]
	eng := engine.New(func(ev engine.DataEvent) {
		if p := program.Load(); p != nil {
			p.Send(ev)
		}
	})
	defer eng.Stop()

	m := app.New(cfg, profiles, resources.NewRegistry(), dbx.NewPool(), eng)
	p := tea.NewProgram(m, tea.WithContext(cmd.Context()))
	program.Store(p)

	_, err = p.Run()
	return err
}
