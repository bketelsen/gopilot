# Section 4: Metrics & Analytics

---

### Task 1: Session duration tracking

**Files:**
- Modify: `internal/metrics/counters.go` (add duration stats)
- Test: `internal/metrics/counters_test.go`
- Modify: `internal/orchestrator/orchestrator.go` (record duration on completion)

**Step 1: Write the failing test**

In `internal/metrics/counters_test.go`:

```go
func TestDurationStats(t *testing.T) {
	c := NewCounters()
	c.RecordDuration("session_duration", 30*time.Second)
	c.RecordDuration("session_duration", 60*time.Second)
	c.RecordDuration("session_duration", 90*time.Second)

	stats := c.DurationStats("session_duration")
	if stats.Count != 3 {
		t.Errorf("count = %d, want 3", stats.Count)
	}
	if stats.Min != 30*time.Second {
		t.Errorf("min = %v, want 30s", stats.Min)
	}
	if stats.Max != 90*time.Second {
		t.Errorf("max = %v, want 90s", stats.Max)
	}
	// Average should be 60s
	if stats.Avg < 59*time.Second || stats.Avg > 61*time.Second {
		t.Errorf("avg = %v, want ~60s", stats.Avg)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race ./internal/metrics/ -run TestDurationStats -v`
Expected: FAIL — `RecordDuration` and `DurationStats` don't exist

**Step 3: Implement duration stats**

In `internal/metrics/counters.go`:

```go
import (
	"sync"
	"time"
)

type DurationStat struct {
	Count int
	Min   time.Duration
	Max   time.Duration
	Avg   time.Duration
	Total time.Duration
}

type Counters struct {
	mu        sync.RWMutex
	vals      map[string]int64
	durations map[string][]time.Duration
}

func NewCounters() *Counters {
	return &Counters{
		vals:      make(map[string]int64),
		durations: make(map[string][]time.Duration),
	}
}

func (c *Counters) RecordDuration(name string, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.durations[name] = append(c.durations[name], d)
}

func (c *Counters) DurationStats(name string) DurationStat {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ds := c.durations[name]
	if len(ds) == 0 {
		return DurationStat{}
	}
	stat := DurationStat{
		Count: len(ds),
		Min:   ds[0],
		Max:   ds[0],
	}
	for _, d := range ds {
		stat.Total += d
		if d < stat.Min {
			stat.Min = d
		}
		if d > stat.Max {
			stat.Max = d
		}
	}
	stat.Avg = stat.Total / time.Duration(stat.Count)
	return stat
}
```

**Step 4: Run tests**

Run: `go test -race ./internal/metrics/ -v`
Expected: All pass

**Step 5: Record duration in orchestrator**

In `internal/orchestrator/orchestrator.go`, in `monitorAgent()`, after recording history:

```go
duration := finishedAt.Sub(entry.StartedAt)
o.metrics.RecordDuration("session_duration", duration)
```

**Step 6: Run all tests**

Run: `go test -race ./...`
Expected: All pass

**Step 7: Commit**

```bash
git add internal/metrics/counters.go internal/metrics/counters_test.go internal/orchestrator/orchestrator.go
git commit -m "feat: track session duration with min/max/avg stats"
```

---

### Task 2: GitHub API rate limit tracking

**Files:**
- Modify: `internal/github/rest.go` (parse rate limit headers)
- Modify: `internal/metrics/counters.go` (store rate limit)
- Test: `internal/github/rest_test.go`

**Step 1: Add rate limit fields to RESTClient**

In `internal/github/rest.go`, add fields and parsing:

```go
type RESTClient struct {
	cfg       config.GitHubConfig
	baseURL   string
	http      *http.Client
	rateLimit RateLimit
	mu        sync.RWMutex
}

type RateLimit struct {
	Remaining int
	Limit     int
	Reset     time.Time
}

func (c *RESTClient) GetRateLimit() RateLimit {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rateLimit
}
```

Add a helper to parse rate limit from response headers:

```go
func (c *RESTClient) updateRateLimit(resp *http.Response) {
	remaining := resp.Header.Get("X-RateLimit-Remaining")
	limit := resp.Header.Get("X-RateLimit-Limit")
	reset := resp.Header.Get("X-RateLimit-Reset")

	c.mu.Lock()
	defer c.mu.Unlock()

	if remaining != "" {
		if n, err := strconv.Atoi(remaining); err == nil {
			c.rateLimit.Remaining = n
		}
	}
	if limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			c.rateLimit.Limit = n
		}
	}
	if reset != "" {
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			c.rateLimit.Reset = time.Unix(ts, 0)
		}
	}
}
```

Add `"strconv"` and `"sync"` imports.

**Step 2: Call updateRateLimit after each API response**

In every method that calls the GitHub API (`fetchRepoIssues`, `FetchIssueState`, `AddComment`, `AddLabel`), add after getting the response:

```go
c.updateRateLimit(resp)
```

For example, in `fetchRepoIssues`, after `c.http.Do(req)`:
```go
resp, err := c.http.Do(req)
if err != nil {
	return nil, err
}
defer resp.Body.Close()
c.updateRateLimit(resp)
```

**Step 3: Write test**

In `internal/github/rest_test.go`:

```go
func TestRateLimitParsing(t *testing.T) {
	// Create a test server that returns rate limit headers
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	cfg := config.GitHubConfig{
		Token: "test",
		Repos: []string{"o/r"},
		EligibleLabels: []string{"gopilot"},
	}
	client := NewRESTClient(cfg, ts.URL+"/")
	client.FetchCandidateIssues(context.Background())

	rl := client.GetRateLimit()
	if rl.Remaining != 4999 {
		t.Errorf("remaining = %d, want 4999", rl.Remaining)
	}
	if rl.Limit != 5000 {
		t.Errorf("limit = %d, want 5000", rl.Limit)
	}
}
```

Add imports: `"context"`, `"net/http"`, `"net/http/httptest"`, `"testing"`.

**Step 4: Run tests**

Run: `go test -race ./internal/github/ -v`
Expected: All pass

**Step 5: Commit**

```bash
git add internal/github/rest.go internal/github/rest_test.go
git commit -m "feat: parse and track GitHub API rate limit from response headers"
```

---

### Task 3: Wire rate limit into settings page

**Files:**
- Modify: `internal/web/server.go` (pass rate limit to settings)
- Modify: `internal/github/client.go` (add GetRateLimit to interface, optional)

**Step 1: Add RateLimitProvider interface**

In `internal/web/server.go`:

```go
type RateLimitProvider interface {
	GetRateLimit() github.RateLimit
}
```

Or simpler: pass rate limit data directly when the server is created. Add a `rateLimitFn` callback:

```go
type Server struct {
	// ... existing fields ...
	rateLimitFn func() RateLimitData
}

type RateLimitData struct {
	Remaining int
	Limit     int
	Reset     time.Time
}
```

In the orchestrator, wire it:

```go
webSrv.SetRateLimitFunc(func() web.RateLimitData {
	rl := restClient.GetRateLimit()
	return web.RateLimitData{
		Remaining: rl.Remaining,
		Limit:     rl.Limit,
		Reset:     rl.Reset,
	}
})
```

Or keep it simpler — just expose rate limit through metrics. In the orchestrator, after each tick, update metrics:

```go
rl := restClient.GetRateLimit()
o.metrics.Set("github_rate_limit_remaining", int64(rl.Remaining))
```

Add a `Set` method to Counters:

```go
func (c *Counters) Set(name string, val int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vals[name] = val
}
```

Then the settings page can read `metrics.Get("github_rate_limit_remaining")`.

**Step 2: Update settings handler to include rate limit**

```go
data := pages.SettingsData{
	Config: s.cfg,
	Skills: skillDisplays,
	RateLimit: pages.RateLimitInfo{
		Remaining: int(s.metrics.Get("github_rate_limit_remaining")),
	},
	AgentValid: agentValid,
}
```

**Step 3: Run tests**

Run: `go test -race ./...`

**Step 4: Commit**

```bash
git add internal/web/server.go internal/metrics/counters.go internal/orchestrator/orchestrator.go
git commit -m "feat: wire rate limit metrics into settings page"
```

---

### Task 4: Final verification

**Step 1: Run full test suite**

Run: `go test -race ./...`
Expected: All packages pass

**Step 2: Run build**

Run: `task build`
Expected: templ generate → tailwindcss → go build all succeed

**Step 3: Run binary with --dry-run to verify it starts**

Run: `./gopilot --dry-run --config gopilot.yaml`
Expected: Runs and exits (may error on missing config, that's fine)

**Step 4: Commit any final adjustments**

```bash
git commit -am "chore: final adjustments from gap closure verification"
```

This completes Section 4 and the entire gap closure plan.
