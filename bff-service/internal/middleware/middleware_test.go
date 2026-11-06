package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(NewContextHandler(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
}

// logLines parses JSON log output into one map per line.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid log line %q: %v", line, err)
		}
		lines = append(lines, m)
	}
	return lines
}

func TestRequestID_ReusesValidIncomingID(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = RequestIDFrom(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderRequestID, "abc-123._X")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if seen != "abc-123._X" {
		t.Errorf("context id = %q, want incoming id", seen)
	}
	if got := rec.Header().Get(HeaderRequestID); got != "abc-123._X" {
		t.Errorf("response header = %q, want incoming id", got)
	}
}

func TestRequestID_GeneratesWhenMissingOrInvalid(t *testing.T) {
	for name, incoming := range map[string]string{
		"missing":       "",
		"invalid chars": "bad id\r\nX-Injected: 1",
		"too long":      strings.Repeat("a", 129),
	} {
		t.Run(name, func(t *testing.T) {
			var seen string
			h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				seen = RequestIDFrom(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if incoming != "" {
				req.Header[HeaderRequestID] = []string{incoming}
			}
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if !uuidV4.MatchString(seen) {
				t.Errorf("generated id %q is not a UUIDv4", seen)
			}
			if rec.Header().Get(HeaderRequestID) != seen {
				t.Errorf("response header %q != context id %q", rec.Header().Get(HeaderRequestID), seen)
			}
		})
	}
}

func TestRequestID_GeneratesUniqueIDs(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := newRequestID()
		if seen[id] {
			t.Fatalf("duplicate id %s", id)
		}
		seen[id] = true
	}
}

func TestContextHandler_AddsRequestIDToLogs(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf).With("component", "test")

	logger.InfoContext(WithRequestID(t.Context(), "req-42"), "hello")
	logger.InfoContext(t.Context(), "no id")

	lines := logLines(t, &buf)
	if lines[0]["requestId"] != "req-42" || lines[0]["component"] != "test" {
		t.Errorf("first line = %v, want requestId and component", lines[0])
	}
	if _, ok := lines[1]["requestId"]; ok {
		t.Errorf("second line must not have requestId: %v", lines[1])
	}
}

func TestAccessLog_LogsRequestWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
	}), RequestID, AccessLog(logger))
	req := httptest.NewRequest(http.MethodPost, "/api/products", nil)
	req.Header.Set(HeaderRequestID, "req-7")

	h.ServeHTTP(httptest.NewRecorder(), req)

	lines := logLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("want 1 access log line, got %d", len(lines))
	}
	l := lines[0]
	if l["msg"] != "http request" || l["method"] != "POST" || l["path"] != "/api/products" ||
		l["status"] != float64(201) || l["bytes"] != float64(5) || l["requestId"] != "req-7" || l["level"] != "INFO" {
		t.Errorf("unexpected access log line: %v", l)
	}
	if _, ok := l["durationMs"]; !ok {
		t.Error("durationMs missing")
	}
}

func TestAccessLog_ImplicitOKAndProbesAtDebug(t *testing.T) {
	var buf bytes.Buffer
	h := AccessLog(newTestLogger(&buf))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	l := logLines(t, &buf)[0]
	if l["status"] != float64(200) || l["level"] != "DEBUG" {
		t.Errorf("unexpected line: %v", l)
	}
}

func TestRecover_Returns500JSONWithoutInternals(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret internal failure at db-host:27017")
	}), Recover(logger), RequestID, AccessLog(logger))
	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	req.Header.Set(HeaderRequestID, "req-panic")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	if strings.Contains(body, "secret") || strings.Contains(body, "goroutine") || strings.Contains(body, "27017") {
		t.Errorf("response leaks internals: %s", body)
	}
	var e map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if e["code"] != "INTERNAL_ERROR" || e["status"] != float64(500) || e["requestId"] != "req-panic" ||
		e["path"] != "/api/products" {
		t.Errorf("unexpected error body: %v", e)
	}

	lines := logLines(t, &buf)
	var sawPanic, sawAccess bool
	for _, l := range lines {
		if l["msg"] == "panic recovered" {
			sawPanic = l["requestId"] == "req-panic" && strings.Contains(l["stack"].(string), "goroutine")
		}
		if l["msg"] == "http request" {
			sawAccess = l["status"] == float64(500)
		}
	}
	if !sawPanic || !sawAccess {
		t.Errorf("want panic log with stack and access log with 500; got %v", lines)
	}
}

func TestRecover_RepanicsAbortHandler(t *testing.T) {
	h := Recover(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler", v)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestChain_FirstMiddlewareIsOutermost(t *testing.T) {
	var order []string
	mw := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { order = append(order, "handler") }),
		mw("a"), mw("b"))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if strings.Join(order, ",") != "a,b,handler" {
		t.Errorf("order = %v, want a,b,handler", order)
	}
}
