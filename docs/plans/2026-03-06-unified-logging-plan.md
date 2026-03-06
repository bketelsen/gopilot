# Unified slog JSON Logging - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace chi's default text HTTP logger with a custom slog-based logger so all logs are structured JSON.

**Architecture:** Implement chi's `middleware.RequestLogger` interface (`LogFormatter` + `LogEntry`) in a new file. The formatter creates log entries that write through Go's `slog` package. Static asset requests are logged at DEBUG level; other requests use status-based leveling (INFO/WARN/ERROR).

**Tech Stack:** Go stdlib `log/slog`, `github.com/go-chi/chi/v5/middleware`

---

### Task 1: Write the slog HTTP log formatter

**Files:**
- Create: `internal/web/httplog.go`
- Test: `internal/web/httplog_test.go`

**Step 1: Write the failing test**

Create `internal/web/httplog_test.go`:

```go
package web

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func TestSlogLogFormatter_InfoForOK(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	r := chi.NewRouter()
	r.Use(middleware.RequestLogger(&slogLogFormatter{}))
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("User-Agent", "TestAgent")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log JSON: %v\nbuf: %s", err, buf.String())
	}
	if entry["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", entry["level"])
	}
	if entry["msg"] != "http request" {
		t.Errorf("msg = %v, want 'http request'", entry["msg"])
	}
	if entry["method"] != "GET" {
		t.Errorf("method = %v, want GET", entry["method"])
	}
	if entry["path"] != "/test" {
		t.Errorf("path = %v, want /test", entry["path"])
	}
	if status, ok := entry["status"].(float64); !ok || int(status) != 200 {
		t.Errorf("status = %v, want 200", entry["status"])
	}
	if entry["user_agent"] != "TestAgent" {
		t.Errorf("user_agent = %v, want TestAgent", entry["user_agent"])
	}
}

func TestSlogLogFormatter_WarnFor4xx(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	r := chi.NewRouter()
	r.Use(middleware.RequestLogger(&slogLogFormatter{}))
	r.Get("/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	req := httptest.NewRequest("GET", "/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}
	if entry["level"] != "WARN" {
		t.Errorf("level = %v, want WARN", entry["level"])
	}
}

func TestSlogLogFormatter_ErrorFor5xx(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	r := chi.NewRouter()
	r.Use(middleware.RequestLogger(&slogLogFormatter{}))
	r.Get("/fail", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	req := httptest.NewRequest("GET", "/fail", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}
	if entry["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", entry["level"])
	}
}

func TestSlogLogFormatter_StaticAssetIsDebug(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	r := chi.NewRouter()
	r.Use(middleware.RequestLogger(&slogLogFormatter{}))
	r.Get("/static/*", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/static/styles.css", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}
	if entry["level"] != "DEBUG" {
		t.Errorf("level = %v, want DEBUG", entry["level"])
	}
}

func TestSlogLogFormatter_FaviconIsDebug(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	r := chi.NewRouter()
	r.Use(middleware.RequestLogger(&slogLogFormatter{}))
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/favicon.ico", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}
	if entry["level"] != "DEBUG" {
		t.Errorf("level = %v, want DEBUG", entry["level"])
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -race -run TestSlogLogFormatter ./internal/web/...`
Expected: FAIL — `slogLogFormatter` not defined

**Step 3: Write the implementation**

Create `internal/web/httplog.go`:

```go
package web

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// slogLogFormatter implements chi's middleware.LogFormatter using slog.
type slogLogFormatter struct{}

func (f *slogLogFormatter) NewLogEntry(r *http.Request) middleware.LogEntry {
	return &slogLogEntry{
		method:    r.Method,
		path:      r.URL.Path,
		remoteAddr: r.RemoteAddr,
		userAgent: r.UserAgent(),
		isStatic:  strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/favicon.ico",
	}
}

// slogLogEntry implements chi's middleware.LogEntry using slog.
type slogLogEntry struct {
	method     string
	path       string
	remoteAddr string
	userAgent  string
	isStatic   bool
}

func (e *slogLogEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra interface{}) {
	level := slog.LevelInfo
	switch {
	case e.isStatic:
		level = slog.LevelDebug
	case status >= 500:
		level = slog.LevelError
	case status >= 400:
		level = slog.LevelWarn
	}

	slog.Log(nil, level, "http request",
		"method", e.method,
		"path", e.path,
		"status", status,
		"duration_ms", float64(elapsed.Microseconds())/1000.0,
		"remote_addr", e.remoteAddr,
		"bytes", bytes,
		"user_agent", e.userAgent,
	)
}

func (e *slogLogEntry) Panic(v interface{}, stack []byte) {
	slog.Error("http request panic",
		"method", e.method,
		"path", e.path,
		"panic", v,
		"stack", string(stack),
	)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -race -run TestSlogLogFormatter ./internal/web/...`
Expected: PASS (all 5 tests)

**Step 5: Commit**

```bash
git add internal/web/httplog.go internal/web/httplog_test.go
git commit -m "feat: add slog-based HTTP request logger for chi middleware"
```

---

### Task 2: Wire the new logger into the server

**Files:**
- Modify: `internal/web/server.go:74`

**Step 1: Replace chi's default logger**

In `internal/web/server.go`, change line 74 from:

```go
r.Use(middleware.Logger)
```

to:

```go
r.Use(middleware.RequestLogger(&slogLogFormatter{}))
```

**Step 2: Run all web tests**

Run: `go test -race ./internal/web/...`
Expected: PASS

**Step 3: Run full test suite**

Run: `go test -race ./...`
Expected: PASS — no regressions

**Step 4: Commit**

```bash
git add internal/web/server.go
git commit -m "feat: replace chi text logger with slog JSON request logger"
```

---

### Task 3: Manual smoke test

**Step 1: Build and run**

Run: `task build && ./gopilot --debug 2>&1 | head -20`

**Step 2: Verify JSON output**

Hit the dashboard in a browser or with curl:

```bash
curl -s http://localhost:3000/api/v1/health
```

Check stderr output — should see a JSON log line like:
```json
{"time":"...","level":"INFO","msg":"http request","method":"GET","path":"/api/v1/health","status":200,...}
```

**Step 3: Verify static assets are DEBUG only**

```bash
curl -s http://localhost:3000/static/styles.css > /dev/null
```

With `--debug`: should see a DEBUG-level JSON log.
Without `--debug`: should see no log for this request.

**Step 4: Commit docs**

```bash
git add docs/plans/
git commit -m "docs: add unified logging design and implementation plan"
```
