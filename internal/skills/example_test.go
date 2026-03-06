package skills_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bketelsen/gopilot/internal/skills"
)

func ExampleLoadFromDir() {
	dir, _ := os.MkdirTemp("", "example-skills")
	defer os.RemoveAll(dir)

	skillDir := filepath.Join(dir, "tdd")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: tdd
description: Test-driven development workflow
type: rigid
---

Write tests before implementation.`), 0644)

	loaded, err := skills.LoadFromDir(dir)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println("count:", len(loaded))
	fmt.Println("name:", loaded[0].Name)
	fmt.Println("type:", loaded[0].Type)
	// Output:
	// count: 1
	// name: tdd
	// type: rigid
}
