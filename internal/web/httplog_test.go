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

// helper builds a chi router with the slog formatter and a single route,
// sends a request, and returns the parsed JSON log line.
func doRequest(t *testing.T, method, path string, handler http.HandlerFunc, logLevel slog.Level) map[string]any {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	r := chi.NewRouter()
	r.Use(middleware.RequestLogger(&slogLogFormatter{}))
	r.Get(path, handler)

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("User-Agent", "TestAgent")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if buf.Len() == 0 {
		t.Fatal("expected log output but buffer is empty")
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("failed to parse log JSON: %v\nraw: %s", err, buf.String())
	}
	return m
}

func TestSlogLogFormatter_InfoForOK(t *testing.T) {
	m := doRequest(t, http.MethodGet, "/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}, slog.LevelDebug)

	if m["level"] != "INFO" {
		t.Errorf("expected level=INFO, got %v", m["level"])
	}
	if m["msg"] != "http request" {
		t.Errorf("expected msg='http request', got %v", m["msg"])
	}
	if m["method"] != "GET" {
		t.Errorf("expected method=GET, got %v", m["method"])
	}
	if m["path"] != "/test" {
		t.Errorf("expected path=/test, got %v", m["path"])
	}
	// status comes as float64 from JSON
	if status, ok := m["status"].(float64); !ok || int(status) != 200 {
		t.Errorf("expected status=200, got %v", m["status"])
	}
	if m["user_agent"] != "TestAgent" {
		t.Errorf("expected user_agent=TestAgent, got %v", m["user_agent"])
	}
}

func TestSlogLogFormatter_WarnFor4xx(t *testing.T) {
	m := doRequest(t, http.MethodGet, "/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}, slog.LevelDebug)

	if m["level"] != "WARN" {
		t.Errorf("expected level=WARN, got %v", m["level"])
	}
}

func TestSlogLogFormatter_ErrorFor5xx(t *testing.T) {
	m := doRequest(t, http.MethodGet, "/fail", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}, slog.LevelDebug)

	if m["level"] != "ERROR" {
		t.Errorf("expected level=ERROR, got %v", m["level"])
	}
}

func TestSlogLogFormatter_StaticAssetIsDebug(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	r := chi.NewRouter()
	r.Use(middleware.RequestLogger(&slogLogFormatter{}))
	r.Get("/static/*", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/static/styles.css", nil)
	req.Header.Set("User-Agent", "TestAgent")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if buf.Len() == 0 {
		t.Fatal("expected log output but buffer is empty")
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("failed to parse log JSON: %v\nraw: %s", err, buf.String())
	}

	if m["level"] != "DEBUG" {
		t.Errorf("expected level=DEBUG, got %v", m["level"])
	}
}

func TestSlogLogFormatter_FaviconIsDebug(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	r := chi.NewRouter()
	r.Use(middleware.RequestLogger(&slogLogFormatter{}))
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	req.Header.Set("User-Agent", "TestAgent")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if buf.Len() == 0 {
		t.Fatal("expected log output but buffer is empty")
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("failed to parse log JSON: %v\nraw: %s", err, buf.String())
	}

	if m["level"] != "DEBUG" {
		t.Errorf("expected level=DEBUG, got %v", m["level"])
	}
}
