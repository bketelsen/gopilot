# Section 1: Templ/HTMX Toolchain & Dashboard Foundation

---

### Task 1: Add templ as a Go tool dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add templ tool dependency**

Run:
```bash
go get -tool github.com/a-h/templ/cmd/templ@latest
```

**Step 2: Verify templ is available via go tool**

Run: `go tool templ version`
Expected: Prints templ version (e.g., `v0.3.x`)

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat: add templ as go tool dependency"
```

---

### Task 2: Download Tailwind CSS v4 standalone CLI

**Files:**
- Create: `bin/.gitkeep` (directory)
- Modify: `.gitignore`

**Step 1: Create bin directory and download tailwindcss**

```bash
mkdir -p bin
curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64
chmod +x tailwindcss-linux-x64
mv tailwindcss-linux-x64 bin/tailwindcss
```

**Step 2: Add bin/ to .gitignore (keep binary local)**

Add to `.gitignore`:
```
bin/
```

**Step 3: Verify tailwindcss works**

Run: `bin/tailwindcss --help`
Expected: Prints usage info

**Step 4: Commit**

```bash
git add .gitignore
git commit -m "chore: add tailwindcss binary setup and gitignore bin/"
```

---

### Task 3: Create Tailwind CSS v4 input file

**Files:**
- Create: `internal/web/static/input.css`

**Step 1: Create the CSS input file with Tailwind v4 import**

```css
@import "tailwindcss";
```

Note: Tailwind CSS v4 uses `@import "tailwindcss"` instead of the v3 `@tailwind` directives.

**Step 2: Run tailwind to generate output**

```bash
bin/tailwindcss -i internal/web/static/input.css -o internal/web/static/styles.css --minify
```

Expected: Creates `internal/web/static/styles.css`

**Step 3: Commit**

```bash
git add internal/web/static/input.css internal/web/static/styles.css
git commit -m "feat: add Tailwind CSS v4 input and generated stylesheet"
```

---

### Task 4: Update Taskfile.yml with full build chain

**Files:**
- Modify: `Taskfile.yml`

**Step 1: Update Taskfile.yml**

Replace the existing `generate` and `css` tasks, add `deps` to `build`:

```yaml
version: '3'

vars:
  VERSION:
    sh: git describe --tags --always --dirty 2>/dev/null || echo "dev"

tasks:
  generate:
    desc: Generate templ files
    cmds:
      - go tool templ generate

  css:
    desc: Build Tailwind CSS
    cmds:
      - bin/tailwindcss -i internal/web/static/input.css -o internal/web/static/styles.css --minify

  build:
    desc: Build the gopilot binary
    deps: [generate, css]
    cmds:
      - go build -ldflags "-X main.Version={{.VERSION}}" -o gopilot ./cmd/gopilot

  test:
    desc: Run all tests
    cmds:
      - go test -race ./...

  lint:
    desc: Run go vet
    cmds:
      - go vet ./...

  clean:
    desc: Remove build artifacts
    cmds:
      - rm -f gopilot

  dev:
    desc: Build and run
    deps: [build]
    cmds:
      - ./gopilot {{.CLI_ARGS}}
```

**Step 2: Verify build chain works**

Run: `task generate && task css`
Expected: Both tasks succeed (templ generate finds no .templ files yet, tailwind generates CSS)

**Step 3: Commit**

```bash
git add Taskfile.yml
git commit -m "feat: wire templ generate and tailwind into build chain"
```

---

### Task 5: Install templUI and components

**Files:**
- Modify: `.templui.json`
- Create: various component files under `components/` or per templUI's output

**Step 1: Install templUI CLI**

```bash
go install github.com/templui/templui@latest
```

Or if available as a go tool:
```bash
go get -tool github.com/templui/templui@latest
```

**Step 2: Initialize templUI**

```bash
templui init
```

This creates/updates `.templui.json`. Confirm defaults or adjust as needed.

**Step 3: Install required components**

```bash
templui add sidebar
templui add table
templui add card
templui add badge
templui add button
```

**Step 4: Verify components installed**

Run: `ls components/` (or wherever templUI places them)
Expected: Component .templ files present

**Step 5: Run templ generate to compile components**

Run: `go tool templ generate`
Expected: Generates _templ.go files for each component

**Step 6: Commit**

```bash
git add .templui.json components/ internal/
git commit -m "feat: install templUI with sidebar, table, card, badge, button components"
```

---

### Task 6: Create base layout template

**Files:**
- Create: `internal/web/templates/layouts/base.templ`

**Step 1: Create the base layout**

```go
package layouts

templ Base(title string) {
	<!DOCTYPE html>
	<html lang="en" class="dark">
	<head>
		<meta charset="UTF-8"/>
		<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
		<title>{ title } - Gopilot</title>
		<link rel="stylesheet" href="/static/styles.css"/>
		<script src="https://unpkg.com/htmx.org@2.0.4"></script>
		<script src="https://unpkg.com/htmx-ext-sse@2.2.2/sse.js"></script>
	</head>
	<body class="bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-gray-100 min-h-screen flex">
		<!-- Sidebar -->
		<nav class="w-64 bg-white dark:bg-gray-800 border-r border-gray-200 dark:border-gray-700 min-h-screen p-4">
			<div class="text-xl font-bold mb-8 px-2">Gopilot</div>
			<ul class="space-y-2">
				<li>
					<a href="/" class="block px-3 py-2 rounded-md hover:bg-gray-100 dark:hover:bg-gray-700">
						Dashboard
					</a>
				</li>
				<li>
					<a href="/sprint" class="block px-3 py-2 rounded-md hover:bg-gray-100 dark:hover:bg-gray-700">
						Sprint
					</a>
				</li>
				<li>
					<a href="/settings" class="block px-3 py-2 rounded-md hover:bg-gray-100 dark:hover:bg-gray-700">
						Settings
					</a>
				</li>
			</ul>
		</nav>
		<!-- Main content -->
		<main class="flex-1 p-6">
			{ children... }
		</main>
	</body>
	</html>
}
```

**Step 2: Run templ generate**

Run: `go tool templ generate`
Expected: Creates `internal/web/templates/layouts/base_templ.go`

**Step 3: Commit**

```bash
git add internal/web/templates/layouts/
git commit -m "feat: add base layout template with sidebar navigation"
```

---

### Task 7: Add static file serving and templ dashboard route

**Files:**
- Modify: `internal/web/server.go`

**Step 1: Update server.go to serve static files and render templ layout**

Add static file serving route and update the dashboard handler to use the templ base layout:

```go
// In buildRouter(), add before the "/" route:
r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("internal/web/static"))))

// Update handleDashboardPage to render templ:
func (s *Server) handleDashboardPage(w http.ResponseWriter, r *http.Request) {
	component := layouts.Base("Dashboard")
	component.Render(r.Context(), w)
}
```

Import the layouts package:
```go
"github.com/bketelsen/gopilot/internal/web/templates/layouts"
```

The base layout currently has no child content, which is fine — it will render the sidebar with an empty main area.

**Step 2: Run templ generate then go build**

Run: `go tool templ generate && go build ./cmd/gopilot/`
Expected: Compiles successfully

**Step 3: Run tests**

Run: `go test -race ./internal/web/...`
Expected: All tests pass

**Step 4: Commit**

```bash
git add internal/web/server.go
git commit -m "feat: serve static files and render templ base layout"
```

---

### Task 8: Smoke test — full build and manual verification

**Step 1: Run full build chain**

Run: `task build`
Expected: `templ generate` → `tailwindcss` → `go build` all succeed, produces `./gopilot` binary

**Step 2: Run all tests**

Run: `go test -race ./...`
Expected: All tests pass

**Step 3: Commit (if any adjustments were needed)**

```bash
git commit -am "chore: fix build chain issues from smoke test"
```

This completes Section 1. The templ/HTMX/Tailwind toolchain is working with a base layout.
