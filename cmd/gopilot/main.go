package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bketelsen/gopilot/internal/agent"
	"github.com/bketelsen/gopilot/internal/config"
	ghclient "github.com/bketelsen/gopilot/internal/github"
	"github.com/bketelsen/gopilot/internal/logging"
	"github.com/bketelsen/gopilot/internal/orchestrator"
	"github.com/bketelsen/gopilot/internal/setup"
)

var Version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Printf("gopilot %s\n", Version)
			return
		case "init":
			runInit()
			return
		case "setup":
			runSetup()
			return
		}
	}

	configPath := flag.String("config", "gopilot.yaml", "path to config file")
	dryRun := flag.Bool("dry-run", false, "list eligible issues without dispatching")
	debug := flag.Bool("debug", false, "enable debug logging")
	port := flag.String("port", "", "override dashboard listen port (e.g., 8080)")
	logFile := flag.String("log", "", "write logs to file (in addition to stderr)")
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logging.Setup(level, *logFile)

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "path", *configPath, "error", err)
		os.Exit(1)
	}

	if *port != "" {
		cfg.Dashboard.Addr = ":" + *port
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

	orch := orchestrator.NewOrchestrator(cfg, restClient, runners, *configPath)
	orch.SetRateLimitFunc(func() (int, int) {
		rl := restClient.GetRateLimit()
		return rl.Remaining, rl.Limit
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	if *dryRun {
		if err := orch.DryRun(ctx); err != nil {
			slog.Error("dry run failed", "error", err)
			os.Exit(1)
		}
		return
	}

	slog.Info("starting gopilot", "version", Version)
	if err := orch.Run(ctx); err != nil {
		slog.Error("orchestrator exited with error", "error", err)
		os.Exit(1)
	}
}

func runSetup() {
	configPath := "gopilot.yaml"
	if len(os.Args) > 2 {
		configPath = os.Args[2]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	client := ghclient.NewRESTClient(cfg.GitHub, "https://api.github.com/")
	results, err := setup.EnsureLabels(context.Background(), cfg, client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(setup.FormatResults(results))
}

func runInit() {
	path := "gopilot.yaml"
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "%s already exists\n", path)
		os.Exit(1)
	}
	if err := os.WriteFile(path, []byte(config.ExampleConfig), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", path, err)
		os.Exit(1)
	}
	fmt.Printf("Created %s — edit it with your GitHub token and repos, then run `gopilot setup` to create labels.\n", path)
}
