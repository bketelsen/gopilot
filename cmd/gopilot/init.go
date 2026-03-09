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

func buildInitConfig(token string, repos []string, agentCmd string, required, optional []string) map[string]any {
	model := "claude-sonnet-4.6"
	if agentCmd == "claude" || agentCmd == "claude-code" {
		model = "claude-sonnet-4-6"
	}

	cfg := map[string]any{
		"github": map[string]any{
			"token":           token,
			"repos":           repos,
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
			"command":                 agentCmd,
			"model":                   model,
			"max_autopilot_continues": 20,
			"turn_timeout_ms":         1800000,
			"stall_timeout_ms":        300000,
			"max_retry_backoff_ms":    300000,
			"max_retries":             3,
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
