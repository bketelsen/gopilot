package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/bketelsen/gopilot/internal/config"
	ghclient "github.com/bketelsen/gopilot/internal/github"
	"github.com/bketelsen/gopilot/internal/setup"
	"github.com/spf13/cobra"
)

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Create required GitHub labels on configured repos",
		RunE:  runSetupCmd,
	}
}

func runSetupCmd(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	client := ghclient.NewRESTClient(cfg.GitHub, "https://api.github.com/")
	results, err := setup.EnsureLabels(context.Background(), cfg, client)
	if err != nil {
		slog.Error("setup failed", "error", err)
		os.Exit(1)
	}

	fmt.Print(setup.FormatResults(results))
	return nil
}
