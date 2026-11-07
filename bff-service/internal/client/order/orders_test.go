package order

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"bff-service/internal/client"
	"bff-service/internal/middleware"
	"bff-service/internal/model"
)

// orderJSON is a realistic Order Service response (see OrderResponse in order-service).
const orderJSON = `{"id":"o1","customerId":"customer-42","status":"CREATED","items":[
{"productId":"p1","productName":"Mechanical Keyboard","unitPrice":"349.99","currency":"PLN","quantity":2,"lineTotal":"699.98"}],
"totalAmount":"699.98","currency":"PLN","createdAt":"2026-01-15T11:00:00Z","updatedAt":"2026-01-15T11:00:00Z"}`

type recorded struct {
	calls  atomic.Int32
	method string
	path   string
	query  string
	body   string
	header http.Header
}

func serve(t *testing.T, status int, body string) (*httptest.Server, *recorded) {
	t.Helper()
	rec := &recorded{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.calls.Add(1)
		b, _ := io.ReadAll(r.Body)
		rec.method, rec.path, rec.query, rec.body, rec.header = r.Method, r.URL.EscapedPath(), r.URL.RawQuery, string(b), r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s, rec
}

func clientErr(t *testing.T, err error, kind client.Kind) *client.Error {
	t.Helper()
	var ce *client.Error
	if !errors.As(err, &ce) || ce.Kind != kind {
		t.Fatalf("err = %v, want kind %v", err, kind)
	}
	return ce
}

func ptr[T any](v T) *T { return &v }

func TestCreate_ForwardsBodyOnceWithRequestID(t *testing.T) {
	s, rec := serve(t, 201, orderJSON)

	o, err := New(s.URL, time.Second).Create(middleware.WithRequestID(t.Context(), "req-9"), model.OrderCreate{
		CustomerID: ptr("customer-42"),
		Items:      []model.OrderCreateItem{{ProductID: ptr("p1"), Quantity: ptr(2)}},
	})

	if err != nil || o.ID != "o1" || o.TotalAmount != "699.98" || o.Items[0].LineTotal != "699.98" {
		t.Fatalf("o = %+v, err = %v", o, err)
	}
	want := `{"customerId":"customer-42","items":[{"productId":"p1","quantity":2}]}`
	if rec.method != http.MethodPost || rec.path != "/api/orders" || rec.body != want {
		t.Errorf("request = %s %s %s", rec.method, rec.path, rec.body)
	}
	if rec.header.Get("X-Request-Id") != "req-9" || rec.calls.Load() != 1 {
		t.Errorf("request id = %q, calls = %d", rec.header.Get("X-Request-Id"), rec.calls.Load())
	}
}

func TestCreate_IsNotRetriedOnFailure(t *testing.T) {
	s, rec := serve(t, 503, `{"code":"PRODUCT_SERVICE_UNAVAILABLE","message":"Product Service is temporarily unavailable"}`)

	_, err := New(s.URL, time.Second).Create(t.Context(), model.OrderCreate{})

	ce := clientErr(t, err, client.KindUnavailable)
	if ce.Status != 503 || ce.Code != "PRODUCT_SERVICE_UNAVAILABLE" {
		t.Errorf("error = %+v, want the downstream code to be kept", ce)
	}
	if rec.calls.Load() != 1 {
		t.Errorf("POST sent %d times, want 1", rec.calls.Load())
	}
}

func TestGet(t *testing.T) {
	s, rec := serve(t, 200, orderJSON)

	o, err := New(s.URL, time.Second).Get(t.Context(), "o/1")

	if err != nil || o.Status != "CREATED" || *o.CustomerID != "customer-42" {
		t.Fatalf("o = %+v, err = %v", o, err)
	}
	if rec.method != http.MethodGet || rec.path != "/api/orders/o%2F1" {
		t.Errorf("request = %s %s (id must be escaped)", rec.method, rec.path)
	}
}

func TestList_ForwardsQuery(t *testing.T) {
	s, rec := serve(t, 200, `{"content":[`+orderJSON+`],"page":0,"size":20,"totalElements":1,"totalPages":1}`)

	page, err := New(s.URL, time.Second).List(t.Context(), url.Values{"status": {"CANCELLED"}})

	if err != nil || len(page.Content) != 1 || page.TotalElements != 1 {
		t.Fatalf("page = %+v, err = %v", page, err)
	}
	if rec.path != "/api/orders" || rec.query != "status=CANCELLED" {
		t.Errorf("request = %s?%s", rec.path, rec.query)
	}
}

func TestCancel(t *testing.T) {
	s, rec := serve(t, 200, orderJSON)

	_, err := New(s.URL, time.Second).Cancel(t.Context(), "o1")

	if err != nil || rec.method != http.MethodPost || rec.path != "/api/orders/o1/cancel" || rec.body != "" {
		t.Errorf("err = %v, request = %s %s body=%q", err, rec.method, rec.path, rec.body)
	}
}

func TestStatusErrorsKeepCodeAndDetails(t *testing.T) {
	tests := []struct {
		status int
		body   string
		code   string
	}{
		{404, `{"code":"NOT_FOUND","message":"Order o9 not found"}`, "NOT_FOUND"},
		{409, `{"code":"CONFLICT","message":"Order o1 is already cancelled"}`, "CONFLICT"},
		{422, `{"code":"PRODUCT_UNAVAILABLE","message":"Product(s) not available: p2",
"details":[{"field":"items[1].productId","message":"Product p2 is not available"}]}`, "PRODUCT_UNAVAILABLE"},
	}
	for _, tt := range tests {
		s, _ := serve(t, tt.status, tt.body)

		_, err := New(s.URL, time.Second).Cancel(t.Context(), "o1")

		ce := clientErr(t, err, client.KindStatus)
		if ce.Status != tt.status || ce.Code != tt.code {
			t.Errorf("error = %+v, want %d %s", ce, tt.status, tt.code)
		}
		if tt.status == 422 && (len(ce.Details) != 1 || ce.Details[0].Field != "items[1].productId") {
			t.Errorf("details = %+v", ce.Details)
		}
	}
}

func TestTimeoutAndCancellation(t *testing.T) {
	block := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer block.Close()

	_, err := New(block.URL, 100*time.Millisecond).Get(t.Context(), "o1")
	clientErr(t, err, client.KindTimeout)

	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err = New(block.URL, 5*time.Second).Get(ctx, "o1")
	clientErr(t, err, client.KindCanceled)
}

func TestInvalidOrderResponse(t *testing.T) {
	for name, body := range map[string]string{
		"malformed":       `{"id":`,
		"missing total":   `{"id":"o1","status":"CREATED","items":[],"currency":"PLN"}`,
		"incomplete item": `{"id":"o1","status":"CREATED","items":[{"productId":"p1"}],"totalAmount":"1.00","currency":"PLN"}`,
		"no items":        `{"id":"o1","status":"CREATED","totalAmount":"1.00","currency":"PLN"}`,
	} {
		t.Run(name, func(t *testing.T) {
			s, _ := serve(t, 200, body)

			_, err := New(s.URL, time.Second).Get(t.Context(), "o1")

			clientErr(t, err, client.KindInvalidResponse)
		})
	}
}
