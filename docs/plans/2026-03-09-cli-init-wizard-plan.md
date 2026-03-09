# CLI Init Wizard & Cobra Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate gopilot CLI from `flag` to cobra+fang, add interactive init wizard with huh, and embed skills in the binary.

**Architecture:** Three cobra subcommands (root=run, init, setup) wrapped by fang for styled output. Skills embedded via `//go:embed` in a top-level `embedded/` package. Init wizard uses huh forms to collect GitHub token, repos, agent choice, and skill selection, then writes `gopilot.yaml` and extracts skill directories.

**Tech Stack:** Go 1.25, cobra, charmbracelet/fang, charmbracelet/huh, embed FS

---

### Task 1: Add Dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Add cobra, fang, and huh**

Run:
```bash
cd /home/debian/gopilot && go get github.com/spf13/cobra@latest github.com/charmbracelet/fang@latest github.com/charmbracelet/huh@latest
```

**Step 2: Verify dependencies resolve**

Run: `go mod tidy`
Expected: Clean exit, no errors.

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add cobra, fang, and huh for CLI migration"
```

---

### Task 2: Embedded Skills Package

**Files:**
- Create: `embedded/skills.go`
- Create: `embedded/skills_test.go`

**Step 1: Write the failing test**

Create `embedded/skills_test.go`:

```go
package embedded

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListSkills(t *testing.T) {
	skills, err := ListSkills()
	if err != nil {
		t.Fatal(err)
	}

	if len(skills) == 0 {
		t.Fatal("expected at least one embedded skill")
	}

	// Check that all known skills are present
	names := make(map[string]bool)
	for _, s := range skills {
		names[s.Name] = true
		if s.Description == "" {
			t.Errorf("skill %q has empty description", s.Name)
		}
	}

	for _, expected := range []string{"code-review", "debugging", "planning", "pr-workflow", "tdd", "verification"} {
		if !names[expected] {
			t.Errorf("missing expected skill %q", expected)
		}
	}
}

func TestExtractSkill(t *testing.T) {
	dest := t.TempDir()

	if err := ExtractSkill("verification", dest); err != nil {
		t.Fatal(err)
	}

	// Check SKILL.md was written
	skillPath := filepath.Join(dest, "verification", "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("SKILL.md not extracted: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("extracted SKILL.md is empty")
	}
}

func TestExtractSkillNotFound(t *testing.T) {
	dest := t.TempDir()
	err := ExtractSkill("nonexistent", dest)
	if err == nil {
		t.Fatal("expected error for nonexistent skill")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./embedded/...`
Expected: FAIL — package doesn't exist yet.

**Step 3: Write the implementation**

Create `embedded/skills.go`:

```go
package embedded

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed all:skills
var skills embed.FS

// SkillInfo holds metadata for display in the init wizard.
type SkillInfo struct {
	Name        string
	Description string
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// ListSkills returns all embedded skill names and descriptions.
func ListSkills() ([]SkillInfo, error) {
	var result []SkillInfo

	entries, err := fs.ReadDir(skills, "skills")
	if err != nil {
		return nil, fmt.Errorf("reading embedded skills: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillFile := filepath.Join("skills", entry.Name(), "SKILL.md")
		data, err := fs.ReadFile(skills, skillFile)
		if err != nil {
			continue // skip dirs without SKILL.md
		}

		info, err := parseFrontmatter(data)
		if err != nil {
			continue
		}
		result = append(result, info)
	}

	return result, nil
}

// ExtractSkill copies the full skill directory tree to destDir/name/.
func ExtractSkill(name string, destDir string) error {
	skillRoot := filepath.Join("skills", name)

	// Verify the skill exists
	if _, err := fs.Stat(skills, skillRoot); err != nil {
		return fmt.Errorf("skill %q not found in embedded assets", name)
	}

	return fs.WalkDir(skills, skillRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Compute destination path: strip "skills/" prefix so we get name/...
		rel, _ := filepath.Rel("skills", path)
		dest := filepath.Join(destDir, rel)

		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}

		data, err := fs.ReadFile(skills, path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0644)
	})
}

func parseFrontmatter(data []byte) (SkillInfo, error) {
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return SkillInfo{}, fmt.Errorf("no frontmatter")
	}

	parts := strings.SplitN(content[3:], "---", 2)
	if len(parts) < 2 {
		return SkillInfo{}, fmt.Errorf("incomplete frontmatter")
	}

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(parts[0]), &fm); err != nil {
		return SkillInfo{}, err
	}

	if fm.Name == "" {
		return SkillInfo{}, fmt.Errorf("missing name")
	}

	return SkillInfo{Name: fm.Name, Description: fm.Description}, nil
}
```

**Important:** The `//go:embed all:skills` directive embeds from the `skills/` directory relative to the file. Since this file is at `embedded/skills.go`, we need a symlink or the actual skills directory. Create a symlink:

```bash
cd /home/debian/gopilot/embedded && ln -s ../skills skills
```

**Step 4: Run tests to verify they pass**

Run: `go test -race ./embedded/...`
Expected: PASS (3 tests)

**Step 5: Commit**

```bash
git add embedded/
git commit -m "feat: add embedded skills package with list and extract"
```

---

### Task 3: Root Command (Orchestrator)

**Files:**
- Create: `cmd/gopilot/root.go`

**Step 1: Create root command**

Create `cmd/gopilot/root.go`:

```go
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
```

**Step 2: Verify it compiles (don't run yet — main.go still has old code)**

Run: `go build ./cmd/gopilot/root.go` — expect failure since `newInitCmd` and `newSetupCmd` don't exist yet. That's fine, we'll add them next.

**Step 3: No commit yet — depends on Tasks 4 and 5.**

---

### Task 4: Setup Command

**Files:**
- Create: `cmd/gopilot/setup.go`

**Step 1: Create setup command**

Create `cmd/gopilot/setup.go`:

```go
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
```

---

### Task 5: Init Command (Wizard)

**Files:**
- Create: `cmd/gopilot/init.go`

**Step 1: Create init command with huh wizard**

Create `cmd/gopilot/init.go`:

```go
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/bketelsen/gopilot/embedded"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a new gopilot workspace with guided setup",
		RunE:  runInitCmd,
	}
}

func runInitCmd(cmd *cobra.Command, args []string) error {
	// Check for existing config
	if _, err := os.Stat(cfgFile); err == nil {
		var overwrite bool
		err := huh.NewConfirm().
			Title(cfgFile + " already exists. Overwrite?").
			Value(&overwrite).
			Run()
		if err != nil {
			return err
		}
		if !overwrite {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// --- Collect user input ---

	var token string
	var reposInput string
	var agentCmd string

	// Pre-fill token from env
	envToken := os.Getenv("GITHUB_TOKEN")
	tokenTitle := "GitHub personal access token"
	if envToken != "" {
		tokenTitle += " (detected from $GITHUB_TOKEN)"
		token = envToken
	}

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(tokenTitle).
				Value(&token).
				EchoMode(huh.EchoModePassword).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("token is required")
					}
					return nil
				}),

			huh.NewInput().
				Title("GitHub repos to monitor (comma-separated, e.g. owner/repo)").
				Value(&reposInput).
				Validate(func(s string) error {
					parts := splitRepos(s)
					if len(parts) == 0 {
						return fmt.Errorf("at least one repo is required")
					}
					for _, p := range parts {
						if !strings.Contains(p, "/") {
							return fmt.Errorf("invalid repo format %q, expected owner/repo", p)
						}
					}
					return nil
				}),

			huh.NewSelect[string]().
				Title("Agent CLI to use").
				Options(
					huh.NewOption("Claude Code (claude)", "claude"),
					huh.NewOption("GitHub Copilot CLI (copilot)", "copilot"),
				).
				Value(&agentCmd),
		),
	).Run()
	if err != nil {
		return err
	}

	// --- Skills selection ---

	allSkills, err := embedded.ListSkills()
	if err != nil {
		return fmt.Errorf("listing embedded skills: %w", err)
	}

	// Build options with defaults pre-selected
	defaults := map[string]bool{"verification": true, "pr-workflow": true, "code-review": true}
	var skillOptions []huh.Option[string]
	var selectedSkills []string

	for _, s := range allSkills {
		skillOptions = append(skillOptions, huh.NewOption(s.Name+" — "+s.Description, s.Name))
		if defaults[s.Name] {
			selectedSkills = append(selectedSkills, s.Name)
		}
	}

	err = huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select skills to install").
				Options(skillOptions...).
				Value(&selectedSkills),
		),
	).Run()
	if err != nil {
		return err
	}

	// --- Required vs Optional ---

	var requiredSkills []string
	if len(selectedSkills) > 0 {
		var reqOptions []huh.Option[string]
		for _, name := range selectedSkills {
			reqOptions = append(reqOptions, huh.NewOption(name, name))
		}

		// Default: verification and tdd are required if selected
		reqDefaults := map[string]bool{"verification": true, "tdd": true}
		for _, name := range selectedSkills {
			if reqDefaults[name] {
				requiredSkills = append(requiredSkills, name)
			}
		}

		err = huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Which skills should be required? (always injected into prompts)").
					Description("Remaining selected skills will be optional (available on-demand).").
					Options(reqOptions...).
					Value(&requiredSkills),
			),
		).Run()
		if err != nil {
			return err
		}
	}

	// Compute optional = selected - required
	reqSet := make(map[string]bool)
	for _, r := range requiredSkills {
		reqSet[r] = true
	}
	var optionalSkills []string
	for _, s := range selectedSkills {
		if !reqSet[s] {
			optionalSkills = append(optionalSkills, s)
		}
	}

	// --- Build config ---

	repos := splitRepos(reposInput)

	// Use $GITHUB_TOKEN reference if the token matches env
	tokenValue := token
	if envToken != "" && token == envToken {
		tokenValue = "$GITHUB_TOKEN"
	}

	cfg := buildInitConfig(tokenValue, repos, agentCmd, requiredSkills, optionalSkills)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(cfgFile, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", cfgFile, err)
	}

	// --- Extract skills ---

	if len(selectedSkills) > 0 {
		if err := os.MkdirAll("skills", 0755); err != nil {
			return fmt.Errorf("creating skills directory: %w", err)
		}
		for _, name := range selectedSkills {
			if err := embedded.ExtractSkill(name, "skills"); err != nil {
				return fmt.Errorf("extracting skill %q: %w", name, err)
			}
		}
	}

	// --- Create workspaces dir ---

	if err := os.MkdirAll("workspaces", 0755); err != nil {
		return fmt.Errorf("creating workspaces directory: %w", err)
	}

	// --- Summary ---

	fmt.Println()
	fmt.Printf("Created %s\n", cfgFile)
	if len(selectedSkills) > 0 {
		fmt.Printf("Extracted %d skills to skills/\n", len(selectedSkills))
	}
	fmt.Println("Created workspaces/")
	fmt.Println()
	fmt.Printf("Edit %s to adjust polling, concurrency, dashboard, and other settings.\n", cfgFile)
	fmt.Println("Run `gopilot setup` to create required GitHub labels on your repos.")
	fmt.Println()

	return nil
}

// buildInitConfig creates a config map for YAML marshaling with sensible defaults.
func buildInitConfig(token string, repos []string, agentCmd string, required, optional []string) map[string]any {
	model := "claude-sonnet-4.6"
	if agentCmd == "claude" || agentCmd == "claude-code" {
		model = "claude-sonnet-4-6"
	}

	cfg := map[string]any{
		"github": map[string]any{
			"token": token,
			"repos": repos,
			"eligible_labels": []string{"gopilot"},
			"excluded_labels": []string{"blocked", "needs-design", "wontfix"},
		},
		"polling": map[string]any{
			"interval_ms":           30000,
			"max_concurrent_agents": 3,
		},
		"workspace": map[string]any{
			"root":            "./workspaces",
			"hook_timeout_ms": 60000,
			"hooks": map[string]any{
				"after_create": "git clone --branch main https://x-access-token:${GITHUB_TOKEN}@github.com/{{repo}}.git .\n",
				"before_run":   "git fetch origin\ngit checkout -B gopilot/issue-{{issue_id}} origin/main\n",
			},
		},
		"agent": map[string]any{
			"command":                agentCmd,
			"model":                  model,
			"max_autopilot_continues": 20,
			"turn_timeout_ms":        1800000,
			"stall_timeout_ms":       300000,
			"max_retry_backoff_ms":   300000,
			"max_retries":            3,
		},
		"skills": map[string]any{
			"dir":      "./skills",
			"required": required,
			"optional": optional,
		},
		"dashboard": map[string]any{
			"enabled": true,
			"addr":    ":3000",
		},
	}

	return cfg
}

func splitRepos(s string) []string {
	var repos []string
	for _, r := range strings.Split(s, ",") {
		r = strings.TrimSpace(r)
		if r != "" {
			repos = append(repos, r)
		}
	}
	return repos
}
```

---

### Task 6: Rewrite main.go

**Files:**
- Modify: `cmd/gopilot/main.go`

**Step 1: Replace main.go with cobra+fang entry point**

Replace the entire contents of `cmd/gopilot/main.go` with:

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/fang"
)

var Version = "dev"

func main() {
	cmd := newRootCmd()
	cmd.Version = Version

	if err := fang.Execute(context.Background(), cmd); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 2: Verify it compiles**

Run: `go build -o gopilot ./cmd/gopilot/`
Expected: Clean build, binary produced.

**Step 3: Verify commands work**

Run: `./gopilot --help`
Expected: Styled help output showing `init`, `setup` subcommands and all flags.

Run: `./gopilot --version`
Expected: `gopilot dev`

Run: `./gopilot init --help`
Expected: Shows init command help.

**Step 4: Commit**

```bash
git add cmd/gopilot/ embedded/
git commit -m "feat: migrate CLI to cobra+fang with interactive init wizard"
```

---

### Task 7: Test Embedded Skills

**Step 1: Run embedded skills tests**

Run: `go test -race ./embedded/...`
Expected: PASS (3 tests — ListSkills, ExtractSkill, ExtractSkillNotFound)

**Step 2: Run all existing tests**

Run: `task test`
Expected: All tests pass. No regressions.

**Step 3: Run linter**

Run: `task lint`
Expected: Clean. Fix any issues.

---

### Task 8: Manual Smoke Test

**Step 1: Test init wizard end-to-end**

```bash
mkdir /tmp/gopilot-test && cd /tmp/gopilot-test
/home/debian/gopilot/gopilot init
```

Walk through the wizard:
- Enter a dummy token
- Enter `owner/repo`
- Select agent
- Select skills, assign required/optional
- Verify `gopilot.yaml` was created with correct values
- Verify `skills/` dir contains selected skill directories with full contents
- Verify `workspaces/` dir was created

**Step 2: Test overwrite protection**

Run `gopilot init` again in the same directory.
Expected: Prompt to confirm overwrite.

**Step 3: Clean up**

```bash
rm -rf /tmp/gopilot-test
```

---

### Task 9: Update Documentation

**Files:**
- Modify: `docs/cli.md` — update command reference for cobra flags (`--` prefix), add `init` wizard description
- Modify: `docs/getting-started.md` — update first-run instructions to describe interactive wizard
- Modify: `README.md` — update quick-start section if it references old `init` behavior
- Modify: `CLAUDE.md` — update Build & Development Commands if needed (should be fine, flags just got `--` prefix)

**Step 1: Update docs**

Update each file to reflect:
- `gopilot init` is now an interactive wizard (not just "writes default yaml")
- Flags use `--` prefix (e.g., `--config` not `-config`)
- Skills are embedded and selectable during init
- `gopilot --version` replaces `gopilot version`

**Step 2: Commit**

```bash
git add docs/ README.md CLAUDE.md
git commit -m "docs: update CLI reference for cobra migration and init wizard"
```

---

### Task 10: Final Verification

**Step 1: Full test suite**

Run: `task test`
Expected: All pass.

**Step 2: Lint**

Run: `task lint`
Expected: Clean.

**Step 3: Build**

Run: `task build`
Expected: Binary builds successfully with embedded skills.
