package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"bff-service/internal/client/order"
	"bff-service/internal/client/product"
	"bff-service/internal/service"
)

const keyboardJSON = `{"id":"p1","name":"Mechanical Keyboard","description":"Compact","price":"349.99",
"currency":"PLN","available":true,"createdAt":"2026-01-15T10:30:00Z","updatedAt":"2026-01-15T10:30:00Z"}`

// fakeProductService records requests and replies with the configured handler.
type fakeProductService struct {
	server  *httptest.Server
	mu      sync.Mutex
	reply   http.HandlerFunc
	calls   int
	method  string
	path    string
	query   string
	body    string
	headers http.Header
}

func newFakeProductService(t *testing.T) *fakeProductService {
	t.Helper()
	f := &fakeProductService{reply: replyJSON(500, `{}`)}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.calls++
		f.method, f.path, f.query, f.body, f.headers = r.Method, r.URL.EscapedPath(), r.URL.RawQuery, string(b), r.Header.Clone()
		reply := f.reply
		f.mu.Unlock()
		reply(w, r)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeProductService) respond(h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reply = h
}

func (f *fakeProductService) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func replyJSON(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

type testApp struct {
	handler http.Handler
	product *fakeProductService
	logs    *bytes.Buffer
}

// newTestApp wires the real router, services and clients against a fake Product Service.
// The Order Service URL points to a closed port (orders are not used by product endpoints).
func newTestApp(t *testing.T, productURL string, productTimeout time.Duration) *testApp {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	productClient := product.New(productURL, productTimeout)
	orderClient := order.New(closedURL(t), time.Second)
	return &testApp{
		handler: NewRouter(Deps{
			Products: service.NewProducts(productClient, logger),
			Readiness: service.NewReadiness(logger,
				service.ReadinessCheck{Name: "productService", Check: productClient.Ready},
				service.ReadinessCheck{Name: "orderService", Check: orderClient.Ready}),
			Logger: logger,
		}),
		logs: logs,
	}
}

func newProductApp(t *testing.T) *testApp {
	t.Helper()
	fake := newFakeProductService(t)
	app := newTestApp(t, fake.server.URL, time.Second)
	app.product = fake
	return app
}

func closedURL(t *testing.T) string {
	t.Helper()
	s := httptest.NewServer(http.NotFoundHandler())
	u := s.URL
	s.Close()
	return u
}

func (a *testApp) do(t *testing.T, method, target, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not a JSON object: %v: %s", err, rec.Body.String())
	}
	return m
}

func assertError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) map[string]any {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, status, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	e := decode(t, rec)
	if e["code"] != code || e["status"] != float64(status) {
		t.Errorf("error body = %v, want code %s", e, code)
	}
	for _, field := range []string{"timestamp", "message", "path", "requestId", "details"} {
		if _, ok := e[field]; !ok {
			t.Errorf("error body misses %q: %v", field, e)
		}
	}
	return e
}

// ---- successful requests and response mapping ----

func TestGetProduct_TransformsPrice(t *testing.T) {
	app := newProductApp(t)
	app.product.respond(replyJSON(200, keyboardJSON))

	rec := app.do(t, http.MethodGet, "/api/products/p1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	want := `{"id":"p1","name":"Mechanical Keyboard","description":"Compact","price":{"amount":"349.99","currency":"PLN"},` +
		`"available":true,"createdAt":"2026-01-15T10:30:00Z","updatedAt":"2026-01-15T10:30:00Z"}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
	if app.product.path != "/api/products/p1" || app.product.method != http.MethodGet {
		t.Errorf("downstream request = %s %s", app.product.method, app.product.path)
	}
}

func TestListProducts_ForwardsPagingAndMapsContent(t *testing.T) {
	app := newProductApp(t)
	app.product.respond(replyJSON(200, `{"content":[`+keyboardJSON+`],"page":0,"size":1,"totalElements":3,"totalPages":3}`))

	rec := app.do(t, http.MethodGet, "/api/products?page=0&size=1&sort=price,desc&internal=1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	content := body["content"].([]any)
	price := content[0].(map[string]any)["price"].(map[string]any)
	if price["amount"] != "349.99" || price["currency"] != "PLN" || body["totalElements"] != float64(3) ||
		body["totalPages"] != float64(3) || body["size"] != float64(1) {
		t.Errorf("unexpected body: %v", body)
	}
	q, _ := url.ParseQuery(app.product.query)
	if q.Get("page") != "0" || q.Get("size") != "1" || q.Get("sort") != "price,desc" || q.Has("internal") {
		t.Errorf("forwarded query = %q", app.product.query)
	}
}

func TestCreateProduct_TranslatesRequestAndReturnsLocation(t *testing.T) {
	app := newProductApp(t)
	app.product.respond(replyJSON(201, keyboardJSON))

	rec := app.do(t, http.MethodPost, "/api/products",
		`{"name":"Mechanical Keyboard","description":"Compact","price":{"amount":"349.99","currency":"PLN"},"available":true}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/api/products/p1" {
		t.Errorf("Location = %q, want the BFF path", loc)
	}
	wantDownstream := `{"name":"Mechanical Keyboard","description":"Compact","price":"349.99","currency":"PLN","available":true}`
	if app.product.method != http.MethodPost || app.product.body != wantDownstream {
		t.Errorf("downstream = %s %s", app.product.method, app.product.body)
	}
	if decode(t, rec)["price"].(map[string]any)["amount"] != "349.99" {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestUpdateProduct(t *testing.T) {
	app := newProductApp(t)
	app.product.respond(replyJSON(200, keyboardJSON))

	rec := app.do(t, http.MethodPut, "/api/products/p1",
		`{"name":"Mechanical Keyboard","price":{"amount":"329.99","currency":"PLN"},"available":false}`)

	if rec.Code != http.StatusOK || app.product.method != http.MethodPut || app.product.path != "/api/products/p1" ||
		!strings.Contains(app.product.body, `"price":"329.99"`) {
		t.Errorf("status = %d, downstream = %s %s %s", rec.Code, app.product.method, app.product.path, app.product.body)
	}
}

func TestDeleteProduct(t *testing.T) {
	app := newProductApp(t)
	app.product.respond(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	rec := app.do(t, http.MethodDelete, "/api/products/p1", "")

	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 || app.product.method != http.MethodDelete {
		t.Errorf("status = %d, body = %q, downstream = %s", rec.Code, rec.Body.String(), app.product.method)
	}
}

// ---- invalid input (the Product Service is never called) ----

func TestInvalidBodies(t *testing.T) {
	tests := map[string]struct {
		body      string
		wantField string
	}{
		"malformed json":       {`{"name":`, ""},
		"empty body":           {"", ""},
		"array instead of obj": {`[{"name":"A"}]`, ""},
		"null":                 {`null`, ""},
		"trailing data":        {`{"name":"A"} {"name":"B"}`, ""},
		"amount as number":     {`{"name":"A","price":{"amount":349.99,"currency":"PLN"}}`, "price.amount"},
		"price as string":      {`{"name":"A","price":"349.99"}`, "price"},
		"available as string":  {`{"name":"A","available":"yes"}`, "available"},
		"amount not a decimal": {`{"name":"A","price":{"amount":"12,50","currency":"PLN"}}`, "price.amount"},
		"amount with exponent": {`{"name":"A","price":{"amount":"1e3","currency":"PLN"}}`, "price.amount"},
	}
	for name, tt := range tests {
		for _, method := range []string{http.MethodPost, http.MethodPut} {
			t.Run(name+"/"+method, func(t *testing.T) {
				app := newProductApp(t)
				target := "/api/products"
				if method == http.MethodPut {
					target += "/p1"
				}

				rec := app.do(t, method, target, tt.body)

				e := assertError(t, rec, http.StatusBadRequest, "VALIDATION_ERROR")
				if tt.wantField != "" {
					details := e["details"].([]any)
					if len(details) != 1 || details[0].(map[string]any)["field"] != tt.wantField {
						t.Errorf("details = %v, want field %s", details, tt.wantField)
					}
				}
				if n := app.product.callCount(); n != 0 {
					t.Errorf("Product Service called %d times for an invalid body", n)
				}
			})
		}
	}
}

func TestBodyTooLarge(t *testing.T) {
	app := newProductApp(t)
	huge := `{"name":"` + strings.Repeat("a", maxRequestBytes) + `"}`

	rec := app.do(t, http.MethodPost, "/api/products", huge)

	assertError(t, rec, http.StatusBadRequest, "VALIDATION_ERROR")
	if app.product.callCount() != 0 {
		t.Error("Product Service must not be called")
	}
}

func TestBusinessValidationIsLeftToProductService(t *testing.T) {
	app := newProductApp(t)
	app.product.respond(replyJSON(400, `{"status":400,"code":"VALIDATION_ERROR","message":"Request validation failed",
"details":[{"field":"price","message":"must be greater than or equal to 0"},{"field":"name","message":"must not be blank"}]}`))

	// Structurally valid, but negative and without a name: only the Product Service decides.
	rec := app.do(t, http.MethodPost, "/api/products", `{"price":{"amount":"-1.00","currency":"PLN"}}`)

	e := assertError(t, rec, http.StatusBadRequest, "VALIDATION_ERROR")
	details := e["details"].([]any)
	if details[0].(map[string]any)["field"] != "price.amount" || details[1].(map[string]any)["field"] != "name" {
		t.Errorf("details = %v, want fields remapped to the BFF contract", details)
	}
	if app.product.callCount() != 1 {
		t.Errorf("Product Service called %d times, want 1", app.product.callCount())
	}
}

// ---- downstream failures ----

func TestProductService4xx(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantCode   string
	}{
		{"404", 404, `{"status":404,"code":"NOT_FOUND","message":"Product p9 not found"}`, 404, "NOT_FOUND"},
		{"405 contract error", 405, `{"code":"METHOD_NOT_ALLOWED"}`, 503, "SERVICE_UNAVAILABLE"},
		{"409 unexpected", 409, `{"code":"CONFLICT"}`, 503, "SERVICE_UNAVAILABLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newProductApp(t)
			app.product.respond(replyJSON(tt.status, tt.body))

			rec := app.do(t, http.MethodGet, "/api/products/p9", "")

			e := assertError(t, rec, tt.wantStatus, tt.wantCode)
			if e["path"] != "/api/products/p9" {
				t.Errorf("path = %v, want the BFF path", e["path"])
			}
		})
	}
}

func TestProductService5xx(t *testing.T) {
	for _, status := range []int{500, 502, 503} {
		app := newProductApp(t)
		app.product.respond(replyJSON(status, `{"message":"NullPointerException at com.example.Secret","trace":"..."}`))

		rec := app.do(t, http.MethodGet, "/api/products/p1", "")

		assertError(t, rec, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE")
		assertNoLeak(t, rec, app.product.server.URL, "NullPointer", "com.example", "trace")
	}
}

func TestProductServiceUnreachable(t *testing.T) {
	downURL := closedURL(t)
	app := newTestApp(t, downURL, time.Second)

	rec := app.do(t, http.MethodGet, "/api/products", "")

	e := assertError(t, rec, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE")
	if e["message"] != "Product Service is temporarily unavailable" {
		t.Errorf("message = %v", e["message"])
	}
	assertNoLeak(t, rec, downURL, "127.0.0.1", "dial", "refused")
}

func TestProductServiceTimeout(t *testing.T) {
	fake := newFakeProductService(t)
	fake.respond(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	})
	app := newTestApp(t, fake.server.URL, 100*time.Millisecond)

	start := time.Now()
	rec := app.do(t, http.MethodPost, "/api/products", `{"name":"A","price":{"amount":"1.00","currency":"PLN"},"available":true}`)

	assertError(t, rec, http.StatusGatewayTimeout, "SERVICE_UNAVAILABLE")
	if time.Since(start) > 2*time.Second {
		t.Errorf("request took %v, want it bounded by the client timeout", time.Since(start))
	}
	if fake.callCount() != 1 {
		t.Errorf("POST sent %d times, want exactly once (no retry)", fake.callCount())
	}
}

func TestProductServiceInvalidResponse(t *testing.T) {
	app := newProductApp(t)
	app.product.respond(replyJSON(200, `{"unexpected":true}`))

	rec := app.do(t, http.MethodGet, "/api/products/p1", "")

	assertError(t, rec, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE")
}

func TestClientCancellationCancelsDownstreamCall(t *testing.T) {
	downstreamCanceled := make(chan struct{})
	fake := newFakeProductService(t)
	fake.respond(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(downstreamCanceled)
	})
	app := newTestApp(t, fake.server.URL, 10*time.Second)

	ctx, cancel := context.WithCancel(t.Context())
	req := httptest.NewRequest(http.MethodGet, "/api/products/p1", nil).WithContext(ctx)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	app.handler.ServeHTTP(httptest.NewRecorder(), req)

	select {
	case <-downstreamCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("downstream request was not canceled with the incoming request")
	}
}

// ---- request ID ----

func TestRequestID_PropagatedDownstreamAndReturned(t *testing.T) {
	app := newProductApp(t)
	app.product.respond(replyJSON(200, keyboardJSON))

	rec := app.do(t, http.MethodGet, "/api/products/p1", "", "X-Request-Id", "client-req-1")

	if rec.Header().Get("X-Request-Id") != "client-req-1" {
		t.Errorf("response header = %q", rec.Header().Get("X-Request-Id"))
	}
	if app.product.headers.Get("X-Request-Id") != "client-req-1" {
		t.Errorf("downstream header = %q", app.product.headers.Get("X-Request-Id"))
	}
}

func TestRequestID_GeneratedAndUsedEverywhere(t *testing.T) {
	app := newProductApp(t)
	app.product.respond(replyJSON(404, `{"code":"NOT_FOUND","message":"Product p1 not found"}`))

	rec := app.do(t, http.MethodGet, "/api/products/p1", "")

	id := rec.Header().Get("X-Request-Id")
	if id == "" {
		t.Fatal("no request id in the response")
	}
	if app.product.headers.Get("X-Request-Id") != id {
		t.Errorf("downstream id %q != response id %q", app.product.headers.Get("X-Request-Id"), id)
	}
	if e := decode(t, rec); e["requestId"] != id {
		t.Errorf("error body requestId %v != %q", e["requestId"], id)
	}
}

// ---- routing ----

func TestUnknownRouteAndMethodAreJSON(t *testing.T) {
	app := newProductApp(t)

	notFound := app.do(t, http.MethodGet, "/api/unknown", "")
	assertError(t, notFound, http.StatusNotFound, "NOT_FOUND")

	notAllowed := app.do(t, http.MethodPatch, "/api/products/p1", "")
	assertError(t, notAllowed, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
	if allow := notAllowed.Header().Get("Allow"); !strings.Contains(allow, "GET") || !strings.Contains(allow, "PUT") {
		t.Errorf("Allow = %q", allow)
	}
	if app.product.callCount() != 0 {
		t.Error("Product Service must not be called")
	}
}

func assertNoLeak(t *testing.T, rec *httptest.ResponseRecorder, forbidden ...string) {
	t.Helper()
	body := rec.Body.String()
	for _, f := range forbidden {
		if f != "" && strings.Contains(body, strings.TrimPrefix(f, "http://")) {
			t.Errorf("response leaks %q: %s", f, body)
		}
	}
}
