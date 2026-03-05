# Deep Dive: templUI

Researched: 2026-03-05
Source: https://github.com/templui/templui (v1.6.0, MIT License, by axadrn)
Docs: https://templui.io

## What It Is

templUI is the **shadcn/ui of Go** — a component library built on [templ](https://github.com/a-h/templ) (Go templating engine) and Tailwind CSS v4. Like shadcn/ui, it uses a **copy-paste / CLI-install model**: you don't import it as a Go dependency. A CLI tool copies component source into your project with rewritten import paths, giving you full code ownership.

**42 components**, server-side rendered, with vanilla JS for interactivity where needed. No React, no Node runtime, no frontend framework.

---

## Tech Stack

| Layer | Technology |
|-------|------------|
| **Language** | Go 1.24+ |
| **Templating** | [templ](https://github.com/a-h/templ) v0.3.1001 (`.templ` → Go code) |
| **CSS** | Tailwind CSS v4.1+ (standalone CLI or npx) |
| **CSS merging** | `tailwind-merge-go` (resolves class conflicts at runtime) |
| **Interactivity** | Vanilla JavaScript (minified with esbuild) |
| **Icons** | Lucide icons (generated as templ components) |
| **Charts** | Chart.js |
| **Build tooling** | [Task](https://taskfile.dev) (Taskfile.yml) |
| **HTMX** | Compatible but not required |

---

## How It Works

### Installation & Usage

```bash
# Install CLI
go install github.com/templui/templui/cmd/templui@latest

# Initialize project
templui init    # creates .templui.json config

# Add components
templui add button card dialog sidebar table

# Scaffold a full starter project
templui new myapp
```

Config (`.templui.json`):
```json
{
  "componentsDir": "components",
  "utilsDir": "utils",
  "moduleName": "github.com/youruser/yourapp",
  "jsDir": "assets/js",
  "jsPublicPath": "/assets/js"
}
```

### Component Pattern

Components are `.templ` files with typed `Props` structs:

```go
@button.Button(button.Props{
    Variant: button.VariantDestructive,
    Size:    button.SizeLg,
}) {
    Delete
}

@card.Card() {
    @card.Header() {
        @card.Title() { Agent Status }
        @card.Description() { Current run details }
    }
    @card.Content() { ... }
    @card.Footer() { ... }
}
```

Fully type-safe with IDE completion. The CLI auto-resolves component dependencies (e.g., adding `datepicker` pulls in `button`, `calendar`, `icon`, `popover`).

### Rendering Model

- **Server-side**: templ compiles to Go, renders HTML on the server
- **CSS**: Tailwind utility classes, `TwMerge()` resolves conflicts
- **JS**: 26 of 42 components include companion `.js`/`.min.js` files for interactivity
- **HTMX**: Works well for partial page updates — the docs site itself uses HTMX

---

## All 42 Components

### Form & Input (16)
| Component | JS | Notes |
|-----------|----|-------|
| **Button** | — | Variants: default, destructive, outline, secondary, ghost, link. Sizes: sm, default, lg, icon |
| **Calendar** | Yes | Date selection with navigation |
| **Checkbox** | Yes | Icon, indeterminate state, groups |
| **Date Picker** | Yes | Input + calendar + popover |
| **Form** | — | Container with validation |
| **Input** | Yes | Icons, password toggle |
| **Input OTP** | Yes | One-time password |
| **Label** | Yes | Form labels |
| **Radio** | — | Radio button groups |
| **Rating** | Yes | Star rating |
| **Select Box** | Yes | Searchable with fuzzy matching |
| **Slider** | Yes | Range slider |
| **Switch** | — | Toggle |
| **Tags Input** | Yes | Multi-tag with autocomplete |
| **Textarea** | Yes | Auto-grow |
| **Time Picker** | Yes | Hour/minute with auto-scroll |

### Layout & Navigation (6)
| Component | JS | Notes |
|-----------|----|-------|
| **Accordion** | — | Collapsible sections |
| **Breadcrumb** | — | Navigation trail |
| **Pagination** | — | Page navigation |
| **Separator** | — | Visual divider |
| **Sidebar** | Yes | Collapsible, variants: sidebar/floating/inset, keyboard shortcut, offcanvas/icon collapse |
| **Tabs** | Yes | Tabbed interface |

### Overlays & Dialogs (5)
| Component | JS | Notes |
|-----------|----|-------|
| **Dialog** | Yes | Modal with backdrop, ESC/click-away dismiss |
| **Dropdown** | Yes | Menu on popover |
| **Popover** | Yes | Floating positioned content |
| **Sheet** | — | Slide-out drawer (built on dialog) |
| **Tooltip** | — | Hover context (built on popover) |

### Feedback & Status (5)
| Component | JS | Notes |
|-----------|----|-------|
| **Alert** | — | Notification messages |
| **Badge** | — | Labels and status indicators |
| **Progress** | Yes | Animated progress bar |
| **Skeleton** | — | Loading placeholders |
| **Toast** | Yes | Notifications with position, duration, dismiss, progress |

### Display & Media (6)
| Component | JS | Notes |
|-----------|----|-------|
| **Aspect Ratio** | — | Responsive container |
| **Avatar** | Yes | Profile images with fallback |
| **Card** | — | Header/Title/Description/Content/Footer sub-components |
| **Carousel** | Yes | Image/content slider |
| **Charts** | Yes | Chart.js integration |
| **Table** | — | Header/body/footer/row/cell/caption |

### Misc (4)
| Component | JS | Notes |
|-----------|----|-------|
| **Code** | Yes | Syntax highlighting (Shiki) |
| **Collapsible** | Yes | Expandable content |
| **Copy Button** | Yes | Clipboard with feedback |
| **Icon** | — | Full Lucide icon set |

---

## Project Health

| Metric | Value |
|--------|-------|
| Stars | 1,363 |
| Forks | 78 |
| Open Issues | 8 |
| Contributors | ~24 |
| Created | October 2024 |
| Latest Release | v1.6.0 (March 2, 2026) |
| Release Cadence | ~1-2 per month |
| Primary Maintainer | axadrn (1,510 of ~1,600 commits) |

Actively maintained with consistent releases. Reached v1.0.0 on Dec 24, 2025 — mature enough for production use.

---

## Limitations & Gotchas

1. **Not importable as `go get`** — must use CLI to copy source files. Open feature request (#452) but not yet implemented.

2. **Updates overwrite customizations** — `templui --installed add` replaces your modified files. Backup before updating.

3. **26 of 42 components need JS** — each requires including its `Script()` function in your layout. Vanilla JS, no framework, but still client-side code to serve.

4. **Tailwind CSS v4.1+ required** — uses the new `@theme inline` syntax. Not backward-compatible with v3.

5. **Single primary maintainer** — axadrn has ~95% of commits. MIT license mitigates risk (you have the code).

6. **Table is presentation-only** — no built-in sorting, filtering, or pagination logic. Must implement server-side.

7. **No real-time/WebSocket component** — you'd pair with HTMX SSE or custom Go handlers for live updates.

8. **Known bugs**: Dialog-in-dropdown broken (#513), selectbox issues with HTMX dynamic replacement (#500).

---

## Comparison to Alternatives

templUI is essentially **the only serious component library in the Go templ ecosystem**.

| | templUI | gomponents | Custom templ + Tailwind |
|---|---------|------------|-------------------------|
| Approach | Pre-built shadcn-style | Pure Go HTML DSL | Build from scratch |
| Components | 42 | None (low-level DSL) | 0 |
| Installation | CLI copies code | `go get` | Manual |
| CSS | Tailwind v4 built-in | BYO | BYO |
| JS interactivity | Vanilla JS included | None | BYO |
| HTMX | Compatible | Has helper lib | Compatible |

There's nothing else close to templUI's scope in the Go ecosystem for pre-built UI components.

---

## Fit for Gopilot Dashboard

### Strong Fits

| Gopilot Need | templUI Component |
|--------------|-------------------|
| Dashboard shell/layout | **Sidebar** (collapsible, floating/inset variants) |
| Agent status list | **Table** + **Badge** (status indicators) |
| Agent detail panels | **Card** (header/content/footer) |
| Task progress | **Progress** bar |
| Real-time notifications | **Toast** (with position/duration/dismiss) |
| Sprint/project views | **Tabs** |
| Agent config/detail modals | **Dialog** / **Sheet** (slide-out drawer) |
| Metrics/analytics | **Charts** (Chart.js) |
| Issue hierarchy display | **Accordion** / **Collapsible** |
| Action menus | **Dropdown** |
| Loading states | **Skeleton** |
| Dark mode | Built-in theme support |

### Architecture Alignment

- **Pure Go server-side rendering** — no React/Node build chain, fits gopilot's Go stack
- **HTMX compatible** — add real-time dashboard updates via SSE without a SPA framework
- **Tailwind v4** — modern, consistent styling with dark mode
- **Code ownership** — customize components freely for gopilot-specific needs
- **Low dependency footprint** — templ + Tailwind + vanilla JS, no heavyweight frontend toolchain

### Gaps to Fill

1. **Real-time updates**: Use HTMX SSE extension or Go's `http.Flusher` for streaming agent status updates to the dashboard
2. **Table sorting/filtering**: Implement server-side with HTMX form submissions or URL query params
3. **WebSocket for live agent output**: Custom Go handler, render updates as templ fragments
4. **Authentication**: Not provided — use standard Go middleware (e.g., session cookies, OAuth)
5. **Data visualization beyond Chart.js**: May want a lighter-weight option for simple metrics

### Recommended Stack for Gopilot Web UI

```
Go (net/http or chi router)
  ├── templ (server-side HTML rendering)
  ├── templUI components (dashboard UI)
  ├── HTMX (partial page updates, SSE for real-time)
  ├── Tailwind CSS v4 (styling)
  └── Chart.js (metrics/analytics via templUI Charts component)
```

No Node.js runtime needed. Build chain: `templ generate` + `tailwindcss` CLI + `go build`.
