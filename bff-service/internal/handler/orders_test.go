package handler

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bff-service/internal/client/order"
	"bff-service/internal/client/product"
	"bff-service/internal/service"
)

// orderJSON is a realistic Order Service response with two lines.
const orderJSON = `{"id":"o1","customerId":"customer-42","status":"CREATED","items":[
{"productId":"p1","productName":"Keyboard (old name)","unitPrice":"300.00","currency":"PLN","quantity":2,"lineTotal":"600.00"},
{"productId":"p2","productName":"Mouse","unitPrice":"20.00","currency":"PLN","quantity":1,"lineTotal":"20.00"}],
"totalAmount":"620.00","currency":"PLN","createdAt":"2026-01-15T11:00:00Z","updatedAt":"2026-01-15T11:00:00Z"}`

const lookupBothJSON = `{"products":[
{"id":"p1","name":"Mechanical Keyboard","price":"349.99","currency":"PLN","available":false},
{"id":"p2","name":"Mouse Pro","price":"25.00","currency":"PLN","available":true}],"missingIds":[]}`

// snapshotItem* are the exact client-facing snapshot parts of every GET /api/orders/o1 response.
const snapshotItem0 = `"productSnapshot":{"productId":"p1","name":"Keyboard (old name)","unitPrice":"300.00","currency":"PLN"},` +
	`"quantity":2,"lineTotal":"600.00"`
const snapshotItem1 = `"productSnapshot":{"productId":"p2","name":"Mouse","unitPrice":"20.00","currency":"PLN"},` +
	`"quantity":1,"lineTotal":"20.00"`
const orderHead = `{"id":"o1","customerId":"customer-42","status":"CREATED","items":[`
const orderTail = `],"totalAmount":"620.00","currency":"PLN","createdAt":"2026-01-15T11:00:00Z","updatedAt":"2026-01-15T11:00:00Z",`

type orderApp struct {
	handler http.Handler
	orders  *fakeProductService // generic recording fake (Order Service)
	product *fakeProductService // Product Service
}

func newOrderAppWith(t *testing.T, orderURL, productURL string, orderTimeout, productTimeout time.Duration) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	productClient := product.New(productURL, productTimeout)
	orderClient := order.New(orderURL, orderTimeout)
	return NewRouter(Deps{
		Products: service.NewProducts(productClient, logger),
		Orders:   service.NewOrders(orderClient, productClient, logger),
		Readiness: service.NewReadiness(logger,
			service.ReadinessCheck{Name: "productService", Check: productClient.Ready},
			service.ReadinessCheck{Name: "orderService", Check: orderClient.Ready}),
		Logger: logger,
	})
}

func newOrderApp(t *testing.T) *orderApp {
	t.Helper()
	orders := newFakeProductService(t)
	products := newFakeProductService(t)
	return &orderApp{
		handler: newOrderAppWith(t, orders.server.URL, products.server.URL, time.Second, time.Second),
		orders:  orders,
		product: products,
	}
}

func (a *orderApp) do(t *testing.T, method, target, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	return (&testApp{handler: a.handler}).do(t, method, target, body, headers...)
}

// ---- create ----

func TestCreateOrder_ForwardsRequestUnchanged(t *testing.T) {
	app := newOrderApp(t)
	app.orders.respond(replyJSON(201, orderJSON))
	body := `{"customerId":"customer-42","items":[{"productId":"p1","quantity":2},{"productId":"p2","quantity":1}]}`

	rec := app.do(t, http.MethodPost, "/api/orders", body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/api/orders/o1" {
		t.Errorf("Location = %q", rec.Header().Get("Location"))
	}
	if app.orders.method != http.MethodPost || app.orders.path != "/api/orders" || app.orders.body != body {
		t.Errorf("downstream = %s %s %s", app.orders.method, app.orders.path, app.orders.body)
	}
	want := orderHead + "{" + snapshotItem0 + "},{" + snapshotItem1 + "}" + strings.TrimSuffix(orderTail, ",") + "}"
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
	if app.product.callCount() != 0 {
		t.Error("the BFF must not call the Product Service when creating an order (no prices, no checks)")
	}
	if app.orders.callCount() != 1 {
		t.Errorf("POST sent %d times, want 1", app.orders.callCount())
	}
}

func TestCreateOrder_InvalidBodyIsRejectedBeforeOrderService(t *testing.T) {
	for name, body := range map[string]string{
		"malformed":           `{"items":[`,
		"items not an array":  `{"items":{"productId":"p1"}}`,
		"fractional quantity": `{"items":[{"productId":"p1","quantity":1.5}]}`,
		"quantity as string":  `{"items":[{"productId":"p1","quantity":"2"}]}`,
		"not an object":       `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			app := newOrderApp(t)

			rec := app.do(t, http.MethodPost, "/api/orders", body)

			assertError(t, rec, http.StatusBadRequest, "VALIDATION_ERROR")
			if app.orders.callCount() != 0 {
				t.Error("Order Service must not be called")
			}
		})
	}
}

func TestCreateOrder_TypeErrorUsesDownstreamFieldNotation(t *testing.T) {
	app := newOrderApp(t)

	rec := app.do(t, http.MethodPost, "/api/orders",
		`{"items":[{"productId":"p1","quantity":1},{"productId":"p2","quantity":1.5}]}`)

	e := assertError(t, rec, http.StatusBadRequest, "VALIDATION_ERROR")
	d := e["details"].([]any)[0].(map[string]any)
	if d["field"] != "items[1].quantity" || d["message"] != "must be an integer" {
		t.Errorf("detail = %v, want items[1].quantity / must be an integer", d)
	}
}

func TestFieldPath(t *testing.T) {
	for in, want := range map[string]string{
		"items.0.quantity":   "items[0].quantity",
		"items.12.productId": "items[12].productId",
		"customer.name":      "customer.name",
		"available":          "available",
	} {
		if got := fieldPath(in); got != want {
			t.Errorf("fieldPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCreateOrder_MapsOrderServiceErrors(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantCode   string
	}{
		{"validation", 400, `{"code":"VALIDATION_ERROR","message":"Request validation failed",
"details":[{"field":"items","message":"must not be empty"}]}`, 400, "VALIDATION_ERROR"},
		{"product not found", 422, `{"code":"PRODUCT_NOT_FOUND","message":"Product(s) not found: p2",
"details":[{"field":"items[1].productId","message":"Product p2 does not exist"}]}`, 422, "PRODUCT_NOT_FOUND"},
		{"product unavailable", 422, `{"code":"PRODUCT_UNAVAILABLE","message":"Product(s) not available: p1"}`, 422, "PRODUCT_UNAVAILABLE"},
		{"currency mismatch", 422, `{"code":"CURRENCY_MISMATCH","message":"All products in an order must share one currency"}`, 422, "CURRENCY_MISMATCH"},
		{"product service down behind order service", 503, `{"code":"PRODUCT_SERVICE_UNAVAILABLE","message":"x"}`, 503, "PRODUCT_SERVICE_UNAVAILABLE"},
		{"order service 500", 500, `{"code":"INTERNAL_ERROR","message":"NullPointerException"}`, 503, "SERVICE_UNAVAILABLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newOrderApp(t)
			app.orders.respond(replyJSON(tt.status, tt.body))

			rec := app.do(t, http.MethodPost, "/api/orders", `{"items":[{"productId":"p1","quantity":1}]}`)

			e := assertError(t, rec, tt.wantStatus, tt.wantCode)
			assertNoLeak(t, rec, "NullPointer", app.orders.server.URL)
			if tt.wantCode == "PRODUCT_NOT_FOUND" {
				details := e["details"].([]any)
				if details[0].(map[string]any)["field"] != "items[1].productId" {
					t.Errorf("details = %v", details)
				}
			}
		})
	}
}

// ---- list ----

func TestListOrders(t *testing.T) {
	app := newOrderApp(t)
	app.orders.respond(replyJSON(200, `{"content":[`+orderJSON+`],"page":1,"size":1,"totalElements":2,"totalPages":2}`))

	rec := app.do(t, http.MethodGet, "/api/orders?page=1&size=1&status=CREATED&sort=createdAt,desc&x=1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	content := body["content"].([]any)
	first := content[0].(map[string]any)
	if body["totalElements"] != float64(2) || first["totalAmount"] != "620.00" || first["currency"] != "PLN" {
		t.Errorf("body = %v", body)
	}
	if _, aggregated := first["items"].([]any)[0].(map[string]any)["currentProduct"]; aggregated {
		t.Error("list items must not be aggregated")
	}
	if !strings.Contains(app.orders.query, "status=CREATED") || strings.Contains(app.orders.query, "x=1") {
		t.Errorf("forwarded query = %q", app.orders.query)
	}
	if app.product.callCount() != 0 {
		t.Error("listing must not call the Product Service")
	}
}

// ---- get with aggregation ----

func TestGetOrder_AggregatesCurrentProductData(t *testing.T) {
	app := newOrderApp(t)
	app.orders.respond(replyJSON(200, orderJSON))
	app.product.respond(replyJSON(200, lookupBothJSON))

	rec := app.do(t, http.MethodGet, "/api/orders/o1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	want := orderHead +
		"{" + snapshotItem0 + `,"currentProduct":{"name":"Mechanical Keyboard","price":"349.99","currency":"PLN","available":false},"currentProductStatus":"OK"},` +
		"{" + snapshotItem1 + `,"currentProduct":{"name":"Mouse Pro","price":"25.00","currency":"PLN","available":true},"currentProductStatus":"OK"}` +
		orderTail + `"warnings":[]}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
	if app.product.method != http.MethodPost || app.product.path != "/api/products/lookup" ||
		app.product.body != `{"ids":["p1","p2"]}` || app.product.callCount() != 1 {
		t.Errorf("lookup = %s %s %s (%d calls)", app.product.method, app.product.path, app.product.body, app.product.callCount())
	}
}

func TestGetOrder_MissingProduct(t *testing.T) {
	app := newOrderApp(t)
	app.orders.respond(replyJSON(200, orderJSON))
	app.product.respond(replyJSON(200, `{"products":[{"id":"p2","name":"Mouse Pro","price":"25.00","currency":"PLN","available":true}],"missingIds":["p1"]}`))

	rec := app.do(t, http.MethodGet, "/api/orders/o1", "")

	body := strings.TrimSpace(rec.Body.String())
	if rec.Code != http.StatusOK ||
		!strings.Contains(body, "{"+snapshotItem0+`,"currentProduct":null,"currentProductStatus":"NOT_FOUND"}`) ||
		!strings.Contains(body, `"currentProductStatus":"OK"`) ||
		!strings.Contains(body, `"warnings":[{"code":"PRODUCT_NOT_FOUND","message":"Product p1 no longer exists; only the order snapshot is available","productId":"p1"}]`) {
		t.Errorf("status = %d, body = %s", rec.Code, body)
	}
}

func TestGetOrder_ProductServiceUnavailableDegrades(t *testing.T) {
	degraded := orderHead +
		"{" + snapshotItem0 + `,"currentProduct":null,"currentProductStatus":"UNKNOWN"},` +
		"{" + snapshotItem1 + `,"currentProduct":null,"currentProductStatus":"UNKNOWN"}` +
		orderTail + `"warnings":[{"code":"PRODUCT_DATA_UNAVAILABLE","message":"Current product data is temporarily unavailable; the order snapshot is complete"}]}`

	scenarios := map[string]func(t *testing.T) (orderURL, productURL string, productTimeout time.Duration){
		"5xx": func(t *testing.T) (string, string, time.Duration) {
			o, p := newFakeProductService(t), newFakeProductService(t)
			o.respond(replyJSON(200, orderJSON))
			p.respond(replyJSON(503, `{"status":503}`))
			return o.server.URL, p.server.URL, time.Second
		},
		"unreachable": func(t *testing.T) (string, string, time.Duration) {
			o := newFakeProductService(t)
			o.respond(replyJSON(200, orderJSON))
			return o.server.URL, closedURL(t), time.Second
		},
		"timeout": func(t *testing.T) (string, string, time.Duration) {
			o, p := newFakeProductService(t), newFakeProductService(t)
			o.respond(replyJSON(200, orderJSON))
			p.respond(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
			return o.server.URL, p.server.URL, 100 * time.Millisecond
		},
		"invalid response": func(t *testing.T) (string, string, time.Duration) {
			o, p := newFakeProductService(t), newFakeProductService(t)
			o.respond(replyJSON(200, orderJSON))
			p.respond(replyJSON(200, `{"oops":true}`))
			return o.server.URL, p.server.URL, time.Second
		},
	}
	for name, setup := range scenarios {
		t.Run(name, func(t *testing.T) {
			orderURL, productURL, productTimeout := setup(t)
			h := newOrderAppWith(t, orderURL, productURL, time.Second, productTimeout)

			start := time.Now()
			rec := (&testApp{handler: h}).do(t, http.MethodGet, "/api/orders/o1", "")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (degraded): %s", rec.Code, rec.Body.String())
			}
			if got := strings.TrimSpace(rec.Body.String()); got != degraded {
				t.Errorf("body =\n%s\nwant\n%s", got, degraded)
			}
			if time.Since(start) > 2*time.Second {
				t.Errorf("took %v, want it bounded by the Product Service timeout", time.Since(start))
			}
		})
	}
}

func TestGetOrder_OrderServiceFailures(t *testing.T) {
	tests := []struct {
		name       string
		reply      http.HandlerFunc
		closed     bool
		timeout    time.Duration
		wantStatus int
		wantCode   string
	}{
		{name: "404", reply: replyJSON(404, `{"code":"NOT_FOUND","message":"Order o9 not found"}`), wantStatus: 404, wantCode: "NOT_FOUND"},
		{name: "5xx", reply: replyJSON(500, `{"code":"INTERNAL_ERROR"}`), wantStatus: 503, wantCode: "SERVICE_UNAVAILABLE"},
		{name: "unreachable", closed: true, wantStatus: 503, wantCode: "SERVICE_UNAVAILABLE"},
		{name: "timeout", reply: func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() },
			timeout: 100 * time.Millisecond, wantStatus: 504, wantCode: "SERVICE_UNAVAILABLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			products := newFakeProductService(t)
			orderURL := closedURL(t)
			if !tt.closed {
				o := newFakeProductService(t)
				o.respond(tt.reply)
				orderURL = o.server.URL
			}
			timeout := tt.timeout
			if timeout == 0 {
				timeout = time.Second
			}
			h := newOrderAppWith(t, orderURL, products.server.URL, timeout, time.Second)

			rec := (&testApp{handler: h}).do(t, http.MethodGet, "/api/orders/o9", "")

			assertError(t, rec, tt.wantStatus, tt.wantCode)
			assertNoLeak(t, rec, orderURL, "dial", "refused")
			if products.callCount() != 0 {
				t.Error("the Product Service must not be called when the order cannot be read")
			}
		})
	}
}

// ---- cancel ----

func TestCancelOrder(t *testing.T) {
	app := newOrderApp(t)
	app.orders.respond(replyJSON(200, strings.Replace(orderJSON, `"CREATED"`, `"CANCELLED"`, 1)))

	rec := app.do(t, http.MethodPost, "/api/orders/o1/cancel", "")

	if rec.Code != http.StatusOK || decode(t, rec)["status"] != "CANCELLED" {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if app.orders.method != http.MethodPost || app.orders.path != "/api/orders/o1/cancel" {
		t.Errorf("downstream = %s %s", app.orders.method, app.orders.path)
	}
}

func TestCancelOrder_ConflictAndNotFound(t *testing.T) {
	for _, tt := range []struct {
		status   int
		body     string
		wantCode string
	}{
		{409, `{"code":"CONFLICT","message":"Order o1 is already cancelled"}`, "CONFLICT"},
		{404, `{"code":"NOT_FOUND","message":"Order o1 not found"}`, "NOT_FOUND"},
	} {
		app := newOrderApp(t)
		app.orders.respond(replyJSON(tt.status, tt.body))

		rec := app.do(t, http.MethodPost, "/api/orders/o1/cancel", "")

		e := assertError(t, rec, tt.status, tt.wantCode)
		if tt.status == 409 && e["message"] != "Order o1 is already cancelled" {
			t.Errorf("message = %v", e["message"])
		}
	}
}

// ---- context cancellation and request ID ----

func TestGetOrder_ClientCancellationCancelsDownstream(t *testing.T) {
	canceled := make(chan struct{})
	orders := newFakeProductService(t)
	orders.respond(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(canceled)
	})
	h := newOrderAppWith(t, orders.server.URL, closedURL(t), 10*time.Second, time.Second)

	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(50*time.Millisecond, cancel)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/orders/o1", nil).WithContext(ctx))

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("Order Service request was not canceled with the incoming request")
	}
}

func TestGetOrder_CancellationDuringLookupStopsLookup(t *testing.T) {
	canceled := make(chan struct{})
	app := newOrderApp(t)
	app.orders.respond(replyJSON(200, orderJSON))
	app.product.respond(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(canceled)
	})

	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(100*time.Millisecond, cancel)
	app.handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/orders/o1", nil).WithContext(ctx))

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("Product Service lookup was not canceled with the incoming request")
	}
}

func TestGetOrder_RequestIDPropagatedToBothServices(t *testing.T) {
	app := newOrderApp(t)
	app.orders.respond(replyJSON(200, orderJSON))
	app.product.respond(replyJSON(200, lookupBothJSON))

	rec := app.do(t, http.MethodGet, "/api/orders/o1", "", "X-Request-Id", "order-req-1")

	if rec.Header().Get("X-Request-Id") != "order-req-1" ||
		app.orders.headers.Get("X-Request-Id") != "order-req-1" ||
		app.product.headers.Get("X-Request-Id") != "order-req-1" {
		t.Errorf("response %q, order %q, product %q", rec.Header().Get("X-Request-Id"),
			app.orders.headers.Get("X-Request-Id"), app.product.headers.Get("X-Request-Id"))
	}
}

func TestOrderRoutes_UnsupportedMethod(t *testing.T) {
	app := newOrderApp(t)

	rec := app.do(t, http.MethodDelete, "/api/orders/o1", "")

	assertError(t, rec, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
	if app.orders.callCount() != 0 {
		t.Error("Order Service must not be called")
	}
}
