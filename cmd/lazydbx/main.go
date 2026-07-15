// Command lazydbx is a k9s-style terminal UI for Databricks.
package main

import (
	"fmt"
	"log/slog"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/jongracecox/lazydbx/internal/app"
	"github.com/jongracecox/lazydbx/internal/config"
	"github.com/jongracecox/lazydbx/internal/logging"
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

			p := tea.NewProgram(app.New(cfg), tea.WithContext(cmd.Context()))
			_, err = p.Run()
			return err
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
