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

		skillFile := "skills/" + entry.Name() + "/SKILL.md"
		data, err := fs.ReadFile(skills, skillFile)
		if err != nil {
			continue
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
	skillRoot := "skills/" + name

	if _, err := fs.Stat(skills, skillRoot); err != nil {
		return fmt.Errorf("skill %q not found in embedded assets", name)
	}

	return fs.WalkDir(skills, skillRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

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
