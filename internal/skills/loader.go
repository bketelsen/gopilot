package skills

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill represents a loaded SKILL.md definition following the agentskills.io spec.
type Skill struct {
	Name          string
	Description   string
	Content       string
	Dir           string
	Location      string
	License       string
	Compatibility string
	Metadata      map[string]string
	AllowedTools  string
}

type frontmatter struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license"`
	Compatibility string            `yaml:"compatibility"`
	Metadata      map[string]string `yaml:"metadata"`
	AllowedTools  string            `yaml:"allowed-tools"`
}

// maxDepth is the maximum directory depth to walk when discovering skills.
const maxDepth = 5

// Discover loads SKILL.md files from multiple directory trees.
// Later directories override earlier ones when skill names collide.
// Parse errors are logged as warnings and do not fail the entire discovery.
func Discover(dirs []string) ([]*Skill, error) {
	byName := make(map[string]*Skill)

	for _, dir := range dirs {
		if dir == "" {
			continue
		}

		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			if d.IsDir() {
				name := d.Name()
				// Skip .git, node_modules, and hidden dirs (except .agents)
				if name == ".git" || name == "node_modules" {
					return filepath.SkipDir
				}
				if strings.HasPrefix(name, ".") && name != ".agents" && path != dir {
					return filepath.SkipDir
				}
			}

			rel, _ := filepath.Rel(dir, path)
			depth := len(strings.Split(rel, string(filepath.Separator)))
			if depth > maxDepth {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if d.IsDir() || d.Name() != "SKILL.md" {
				return nil
			}

			skill, err := parseSkillFile(path)
			if err != nil {
				slog.Warn("skipping skill file", "path", path, "error", err)
				return nil
			}
			skill.Dir = filepath.Dir(path)

			if existing, ok := byName[skill.Name]; ok {
				slog.Warn("skill shadowed", "name", skill.Name, "old", existing.Location, "new", skill.Location)
			}
			byName[skill.Name] = skill
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	result := make([]*Skill, 0, len(byName))
	for _, s := range byName {
		result = append(result, s)
	}
	return result, nil
}

// LoadFromDir loads all SKILL.md files from a single directory tree.
// Deprecated: Use Discover instead.
func LoadFromDir(dir string) ([]*Skill, error) {
	return Discover([]string{dir})
}

// LoadFromDirs loads SKILL.md files from multiple directory trees.
// Deprecated: Use Discover instead.
func LoadFromDirs(dirs []string) ([]*Skill, error) {
	return Discover(dirs)
}

func parseSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	skill := &Skill{}

	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content[3:], "---", 2)
		if len(parts) == 2 {
			var fm frontmatter
			if err := yaml.Unmarshal([]byte(parts[0]), &fm); err != nil {
				return nil, fmt.Errorf("invalid frontmatter YAML in %s: %w", path, err)
			}
			skill.Name = fm.Name
			skill.Description = fm.Description
			skill.License = fm.License
			skill.Compatibility = fm.Compatibility
			skill.Metadata = fm.Metadata
			skill.AllowedTools = fm.AllowedTools
			skill.Content = strings.TrimSpace(parts[1])
		}
	}

	if skill.Name == "" {
		return nil, fmt.Errorf("skill at %s has no name in frontmatter", path)
	}

	if skill.Description == "" {
		return nil, fmt.Errorf("skill at %s has no description in frontmatter", path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	skill.Location = absPath

	return skill, nil
}
