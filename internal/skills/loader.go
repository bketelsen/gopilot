package skills

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Skill struct {
	Name        string
	Description string
	Type        string
	Content     string
	Dir         string
}

func LoadFromDir(dir string) ([]*Skill, error) {
	return LoadFromDirs([]string{dir})
}

func LoadFromDirs(dirs []string) ([]*Skill, error) {
	byName := make(map[string]*Skill)

	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			rel, _ := filepath.Rel(dir, path)
			depth := len(strings.Split(rel, string(filepath.Separator)))
			if depth > 4 {
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
				return fmt.Errorf("parsing %s: %w", path, err)
			}
			skill.Dir = filepath.Dir(path)
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
			parseFrontmatter(parts[0], skill)
			skill.Content = strings.TrimSpace(parts[1])
		}
	}

	if skill.Name == "" {
		return nil, fmt.Errorf("skill at %s has no name in frontmatter", path)
	}

	return skill, nil
}

func parseFrontmatter(fm string, skill *Skill) {
	scanner := bufio.NewScanner(strings.NewReader(fm))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			switch key {
			case "name":
				skill.Name = val
			case "description":
				skill.Description = val
			case "type":
				skill.Type = val
			}
		}
	}
}
