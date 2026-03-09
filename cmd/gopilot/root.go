package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	ghclient "github.com/bketelsen/gopilot/internal/github"
	"github.com/bketelsen/gopilot/internal/logging"
	"github.com/bketelsen/gopilot/internal/orchestrator"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	dryRun  bool
	debug   bool
	port    string
	logFile string
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gopilot",
		Short: "AI coding agent orchestrator for GitHub issues",
		Long:  "Gopilot dispatches AI coding agents to work on GitHub issues, with real-time monitoring and retry logic.",
		RunE:  runOrchestrator,
	}

	cmd.PersistentFlags().StringVar(&cfgFile, "config", "gopilot.yaml", "path to config file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list eligible issues without dispatching")
	cmd.Flags().BoolVar(&debug, "debug", false, "enable debug logging")
	cmd.Flags().StringVar(&port, "port", "", "override dashboard listen port (e.g., 8080)")
	cmd.Flags().StringVar(&logFile, "log", "", "write logs to file (in addition to stderr)")

	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newSetupCmd())

	return cmd
}

func runOrchestrator(cmd *cobra.Command, args []string) error {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	logging.Setup(level, logFile)

	cfg, err := config.Load(cfgFile)
	if err != nil {
		slog.Error("failed to load config", "path", cfgFile, "error", err)
		os.Exit(1)
	}

	if port != "" {
		cfg.Dashboard.Addr = ":" + port
		cfg.Dashboard.Enabled = true
	}

	restClient := ghclient.NewRESTClient(cfg.GitHub, "https://api.github.com/")

	var defaultRunner agent.Runner
	switch cfg.Agent.Command {
	case "claude", "claude-code":
		defaultRunner = &agent.ClaudeRunner{
			Command: cfg.Agent.Command,
			Token:   cfg.GitHub.Token,
		}
	default:
		defaultRunner = &agent.CopilotRunner{
			Command: cfg.Agent.Command,
			Token:   cfg.GitHub.Token,
		}
	}
	runners := map[string]agent.Runner{
		cfg.Agent.Command: defaultRunner,
	}
	for _, override := range cfg.Agent.Overrides {
		if _, exists := runners[override.Command]; !exists {
			switch override.Command {
			case "claude", "claude-code":
				runners[override.Command] = &agent.ClaudeRunner{
					Command: override.Command,
					Token:   cfg.GitHub.Token,
				}
			default:
				runners[override.Command] = &agent.CopilotRunner{
					Command: override.Command,
					Token:   cfg.GitHub.Token,
				}
			}
		}
	}

	orch := orchestrator.NewOrchestrator(cfg, restClient, runners, cfgFile)
	orch.SetRateLimitFunc(func() (int, int) {
		rl := restClient.GetRateLimit()
		return rl.Remaining, rl.Limit
	})

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	if dryRun {
		if err := orch.DryRun(ctx); err != nil {
			slog.Error("dry run failed", "error", err)
			os.Exit(1)
		}
		return nil
	}

	slog.Info("starting gopilot", "version", Version)
	if err := orch.Run(ctx); err != nil {
		slog.Error("orchestrator exited with error", "error", err)
		os.Exit(1)
	}
	return nil
}
