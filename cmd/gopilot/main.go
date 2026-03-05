package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bketelsen/gopilot/internal/config"
	"github.com/bketelsen/gopilot/internal/orchestrator"
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
		}
	}

	// Main run flags
	configPath := flag.String("config", "gopilot.yaml", "path to config file")
	dryRun := flag.Bool("dry-run", false, "list eligible issues without dispatching")
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	// Setup logging
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	config.SetupLogging(level)

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "path", *configPath, "error", err)
		os.Exit(1)
	}

	// Create orchestrator
	orch, err := orchestrator.New(cfg, *configPath)
	if err != nil {
		slog.Error("failed to create orchestrator", "error", err)
		os.Exit(1)
	}

	// Setup signal handling
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

	// Run the orchestrator
	slog.Info("starting gopilot", "version", Version)
	if err := orch.Run(ctx); err != nil {
		slog.Error("orchestrator exited with error", "error", err)
		os.Exit(1)
	}
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

	fmt.Printf("Created %s — edit it with your GitHub token and repos.\n", path)
}
