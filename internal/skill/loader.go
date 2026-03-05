package skill

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Skill represents a loaded behavioral contract.
type Skill struct {
	Name    string // derived from filename: "tdd" from "tdd.md"
	Path    string // absolute path to the SKILL.md file
	Content string // raw markdown content
}

// Loader discovers and loads skill files from configured directories.
type Loader struct {
	dirs []string
}

func NewLoader(dirs []string) *Loader {
	return &Loader{dirs: dirs}
}

// LoadAll discovers and loads all .md files from skill directories.
// Later directories override earlier ones if skill names collide.
func (l *Loader) LoadAll() ([]Skill, error) {
	seen := make(map[string]int) // name -> index in result
	var skills []Skill

	for _, dir := range l.dirs {
		dirSkills, err := l.loadDir(dir)
		if err != nil {
			slog.Warn("failed to load skills from dir", "dir", dir, "error", err)
			continue
		}

		for _, s := range dirSkills {
			if idx, exists := seen[s.Name]; exists {
				// Override with later directory
				skills[idx] = s
				slog.Debug("skill overridden", "name", s.Name, "path", s.Path)
			} else {
				seen[s.Name] = len(skills)
				skills = append(skills, s)
			}
		}
	}

	slog.Info("loaded skills", "count", len(skills))
	return skills, nil
}

// LoadByNames loads only the named skills. Returns error if a required skill is missing.
func (l *Loader) LoadByNames(names []string) ([]Skill, error) {
	all, err := l.LoadAll()
	if err != nil {
		return nil, err
	}

	index := make(map[string]Skill, len(all))
	for _, s := range all {
		index[s.Name] = s
	}

	var result []Skill
	for _, name := range names {
		s, ok := index[name]
		if !ok {
			return nil, fmt.Errorf("skill %q not found in directories: %v", name, l.dirs)
		}
		result = append(result, s)
	}

	return result, nil
}

func (l *Loader) loadDir(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // directory doesn't exist yet, that's ok
		}
		return nil, fmt.Errorf("read skill dir %s: %w", dir, err)
	}

	var skills []Skill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("failed to read skill file", "path", path, "error", err)
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".md")
		skills = append(skills, Skill{
			Name:    name,
			Path:    path,
			Content: string(content),
		})

		slog.Debug("loaded skill", "name", name, "path", path)
	}

	return skills, nil
}

// FormatForPrompt formats a set of skills as a single string for injection into prompts.
func FormatForPrompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n## Skills & Behavioral Contracts\n\n")
	b.WriteString("Follow these behavioral contracts for this task:\n\n")

	for _, s := range skills {
		b.WriteString(fmt.Sprintf("### Skill: %s\n\n", s.Name))
		b.WriteString(s.Content)
		b.WriteString("\n\n---\n\n")
	}

	return b.String()
}
