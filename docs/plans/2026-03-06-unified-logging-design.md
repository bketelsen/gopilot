# Unified slog JSON Logging for HTTP Requests

## Problem

Gopilot has two logging formats:
- Application logs use `slog.NewJSONHandler` (structured JSON)
- HTTP request logs use chi's `middleware.Logger` (Apache-style text via Go's default `log` package)
- (Previously had stray `log.Printf` calls, now cleaned up)

## Decision

Unify all logging to structured JSON via slog. Replace chi's default text logger with a custom `middleware.RequestLogger` implementation that writes through slog.

## Approach

Use chi's `middleware.RequestLogger` interface (Approach 1 from brainstorming). This is idiomatic chi, handles response-writer wrapping and panic recovery, and requires no new dependencies.

## Changes

### 1. New file: `internal/web/httplog.go`

Implements chi's `middleware.RequestLogger` interface:

- **`slogLogFormatter`** (implements `LogFormatter`): Creates a log entry per request. Marks static asset paths (`/static/`, `/favicon.ico`) for suppression at INFO level.
- **`slogLogEntry`** (implements `LogEntry`): Logs when request completes with fields:
  - `method`, `path`, `status`, `duration_ms`, `remote_addr`, `bytes`, `user_agent`
  - Log level: INFO for 2xx/3xx, WARN for 4xx, ERROR for 5xx
  - Static asset requests logged at DEBUG level only

### 2. Modify `internal/web/server.go`

Replace `r.Use(middleware.Logger)` with `r.Use(middleware.RequestLogger(&slogLogFormatter{}))`.

## What doesn't change

- `internal/logging/logging.go` -- untouched
- All other packages -- already use slog
- `middleware.Recoverer` -- stays as-is

## Example output

```json
{"time":"2026-03-06T10:15:32Z","level":"INFO","msg":"http request","method":"GET","path":"/dashboard","status":200,"duration_ms":4.2,"remote_addr":"10.0.1.117","bytes":1234,"user_agent":"Mozilla/5.0..."}
```

Static assets (debug only):
```json
{"time":"2026-03-06T10:15:32Z","level":"DEBUG","msg":"http request","method":"GET","path":"/static/styles.css","status":200,"duration_ms":0.8,"bytes":5678}
```
