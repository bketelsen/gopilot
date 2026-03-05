package skills

import (
	"fmt"
	"strings"
)

func InjectSkills(allSkills []*Skill, required []string, optional []string) string {
	byName := make(map[string]*Skill)
	for _, s := range allSkills {
		byName[s.Name] = s
	}

	var parts []string

	for _, name := range required {
		if skill, ok := byName[name]; ok {
			parts = append(parts, formatSkill(skill))
		}
	}

	for _, name := range optional {
		if skill, ok := byName[name]; ok {
			parts = append(parts, formatSkill(skill))
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return "# Skills\n\n" + strings.Join(parts, "\n\n---\n\n")
}

func formatSkill(skill *Skill) string {
	return fmt.Sprintf("## Skill: %s (%s)\n\n%s", skill.Name, skill.Type, skill.Content)
}
