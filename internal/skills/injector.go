package skills

import (
	"fmt"
	"strings"
)

// InjectSkills renders required skills as full content and optional skills as a
// lightweight catalog with file paths for on-demand file-read activation.
func InjectSkills(allSkills []*Skill, required []string, optional []string) string {
	byName := make(map[string]*Skill)
	for _, s := range allSkills {
		byName[s.Name] = s
	}

	var sections []string

	// Required skills: full content inline.
	var requiredParts []string
	for _, name := range required {
		if skill, ok := byName[name]; ok {
			requiredParts = append(requiredParts, formatSkill(skill))
		}
	}
	if len(requiredParts) > 0 {
		sections = append(sections, "# Skills\n\n"+strings.Join(requiredParts, "\n\n---\n\n"))
	}

	// Optional skills: catalog with description + location for file-read activation.
	var catalogEntries []string
	for _, name := range optional {
		if skill, ok := byName[name]; ok {
			catalogEntries = append(catalogEntries, formatCatalogEntry(skill))
		}
	}
	if len(catalogEntries) > 0 {
		catalog := "# Available Skills\n\n" +
			"The following skills provide specialized instructions for specific tasks.\n" +
			"When a task matches a skill's description, use your file-read tool to load\n" +
			"the SKILL.md at the listed location before proceeding.\n\n" +
			strings.Join(catalogEntries, "\n")
		sections = append(sections, catalog)
	}

	return strings.Join(sections, "\n\n")
}

func formatSkill(skill *Skill) string {
	return fmt.Sprintf("## Skill: %s\n\n%s", skill.Name, skill.Content)
}

func formatCatalogEntry(skill *Skill) string {
	return fmt.Sprintf("- **%s** — %s\n  Location: %s", skill.Name, skill.Description, skill.Location)
}
