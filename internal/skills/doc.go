// Package skills loads SKILL.md files and injects them into agent prompts.
//
// Skills are markdown documents with YAML frontmatter that define reusable
// instructions for agents. This package discovers skill files from configured
// directories, parses their metadata and content, and formats them for
// inclusion in rendered prompts.
package skills
