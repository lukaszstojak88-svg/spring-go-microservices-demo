package service

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"testing"

	"bff-service/internal/client"
	"bff-service/internal/model"
)

type fakeOrderAPI struct {
	order     model.ServiceOrder
	page      model.ServiceOrderPage
	err       error
	gotCreate model.OrderCreate
	gotQuery  url.Values
	gotID     string
}

func (f *fakeOrderAPI) Create(_ context.Context, b model.OrderCreate) (model.ServiceOrder, error) {
	f.gotCreate = b
	return f.order, f.err
}

func (f *fakeOrderAPI) Get(_ context.Context, id string) (model.ServiceOrder, error) {
	f.gotID = id
	return f.order, f.err
}

func (f *fakeOrderAPI) List(_ context.Context, q url.Values) (model.ServiceOrderPage, error) {
	f.gotQuery = q
	return f.page, f.err
}

func (f *fakeOrderAPI) Cancel(_ context.Context, id string) (model.ServiceOrder, error) {
	f.gotID = id
	return f.order, f.err
}

type fakeLookup struct {
	resp   model.ServiceLookupResponse
	err    error
	calls  int
	gotIDs []string
}

func (f *fakeLookup) Lookup(_ context.Context, ids []string) (model.ServiceLookupResponse, error) {
	f.calls++
	f.gotIDs = ids
	return f.resp, f.err
}

// storedOrder has two lines; the snapshot data deliberately differs from the current products.
func storedOrder() model.ServiceOrder {
	return model.ServiceOrder{
		ID: "o1", CustomerID: ptr("customer-42"), Status: "CREATED",
		Items: []model.ServiceOrderItem{
			{ProductID: "p1", ProductName: "Keyboard (old name)", UnitPrice: "300.00", Currency: "PLN", Quantity: 2, LineTotal: "600.00"},
			{ProductID: "p2", ProductName: "Mouse", UnitPrice: "20.00", Currency: "PLN", Quantity: 1, LineTotal: "20.00"},
		},
		TotalAmount: "620.00", Currency: "PLN",
		CreatedAt: "2026-01-15T11:00:00Z", UpdatedAt: "2026-01-15T11:00:00Z",
	}
}

func currentKeyboard() model.ServiceProduct {
	return model.ServiceProduct{ID: "p1", Name: "Mechanical Keyboard", Price: "349.99", Currency: "PLN", Available: false}
}

func currentMouse() model.ServiceProduct {
	return model.ServiceProduct{ID: "p2", Name: "Mouse Pro", Price: "25.00", Currency: "PLN", Available: true}
}

// expectedSnapshot is what every response must contain regardless of the Product Service.
func expectedSnapshot() []model.OrderItem {
	return []model.OrderItem{
		{ProductSnapshot: model.ProductSnapshot{ProductID: "p1", Name: "Keyboard (old name)",
			UnitPrice: "300.00", Currency: "PLN"},
			Quantity: 2, LineTotal: "600.00"},
		{ProductSnapshot: model.ProductSnapshot{ProductID: "p2", Name: "Mouse",
			UnitPrice: "20.00", Currency: "PLN"},
			Quantity: 1, LineTotal: "20.00"},
	}
}

func assertSnapshotUnchanged(t *testing.T, d model.OrderDetails) {
	t.Helper()
	var got []model.OrderItem
	for _, it := range d.Items {
		got = append(got, it.OrderItem)
	}
	if !reflect.DeepEqual(got, expectedSnapshot()) {
		t.Errorf("snapshot changed:\n got %+v\nwant %+v", got, expectedSnapshot())
	}
	if d.TotalAmount != "620.00" || d.Currency != "PLN" || d.Status != "CREATED" || d.ID != "o1" {
		t.Errorf("order header changed: %+v", d)
	}
}

func TestGetOrder_AggregatesCurrentProducts(t *testing.T) {
	lookup := &fakeLookup{resp: model.ServiceLookupResponse{
		Products: []model.ServiceProduct{currentMouse(), currentKeyboard()}, MissingIDs: []string{}}}
	orders := &fakeOrderAPI{order: storedOrder()}

	d, err := NewOrders(orders, lookup, discard).Get(t.Context(), "o1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSnapshotUnchanged(t, d)
	if lookup.calls != 1 || !reflect.DeepEqual(lookup.gotIDs, []string{"p1", "p2"}) {
		t.Errorf("lookup calls = %d, ids = %v; want one batch call", lookup.calls, lookup.gotIDs)
	}
	want := &model.CurrentProduct{Name: "Mechanical Keyboard", Price: "349.99", Currency: "PLN", Available: false}
	if !reflect.DeepEqual(d.Items[0].CurrentProduct, want) || d.Items[0].CurrentProductStatus != "OK" {
		t.Errorf("item 0 = %+v", d.Items[0])
	}
	if d.Items[1].CurrentProduct.Name != "Mouse Pro" || d.Items[1].CurrentProductStatus != "OK" {
		t.Errorf("item 1 = %+v", d.Items[1])
	}
	if d.Warnings == nil || len(d.Warnings) != 0 {
		t.Errorf("warnings = %v, want empty array", d.Warnings)
	}
}

func TestGetOrder_MissingProductIsNotFoundWithWarning(t *testing.T) {
	lookup := &fakeLookup{resp: model.ServiceLookupResponse{
		Products: []model.ServiceProduct{currentMouse()}, MissingIDs: []string{"p1"}}}

	d, err := NewOrders(&fakeOrderAPI{order: storedOrder()}, lookup, discard).Get(t.Context(), "o1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertSnapshotUnchanged(t, d)
	if d.Items[0].CurrentProduct != nil || d.Items[0].CurrentProductStatus != "NOT_FOUND" {
		t.Errorf("missing item = %+v", d.Items[0])
	}
	if d.Items[1].CurrentProductStatus != "OK" {
		t.Errorf("other item = %+v", d.Items[1])
	}
	if len(d.Warnings) != 1 || d.Warnings[0].Code != "PRODUCT_NOT_FOUND" || *d.Warnings[0].ProductID != "p1" {
		t.Errorf("warnings = %+v", d.Warnings)
	}
}

func TestGetOrder_ProductServiceFailureDegrades(t *testing.T) {
	for _, kind := range []client.Kind{client.KindUnavailable, client.KindTimeout, client.KindStatus, client.KindInvalidResponse} {
		t.Run(kind.String(), func(t *testing.T) {
			lookup := &fakeLookup{err: &client.Error{Service: "product-service", Kind: kind, Status: 500}}

			d, err := NewOrders(&fakeOrderAPI{order: storedOrder()}, lookup, discard).Get(t.Context(), "o1")

			if err != nil {
				t.Fatalf("a Product Service failure must not fail the request: %v", err)
			}
			assertSnapshotUnchanged(t, d)
			for _, it := range d.Items {
				if it.CurrentProduct != nil || it.CurrentProductStatus != "UNKNOWN" {
					t.Errorf("item = %+v, want currentProduct null and UNKNOWN", it)
				}
			}
			if len(d.Warnings) != 1 || d.Warnings[0].Code != "PRODUCT_DATA_UNAVAILABLE" || d.Warnings[0].ProductID != nil {
				t.Errorf("warnings = %+v", d.Warnings)
			}
		})
	}
}

func TestGetOrder_IncompleteLookupMarksItemUnknown(t *testing.T) {
	lookup := &fakeLookup{resp: model.ServiceLookupResponse{
		Products: []model.ServiceProduct{currentMouse()}, MissingIDs: []string{}}}

	d, _ := NewOrders(&fakeOrderAPI{order: storedOrder()}, lookup, discard).Get(t.Context(), "o1")

	if d.Items[0].CurrentProductStatus != "UNKNOWN" || d.Items[1].CurrentProductStatus != "OK" ||
		len(d.Warnings) != 1 || d.Warnings[0].Code != "PRODUCT_DATA_UNAVAILABLE" {
		t.Errorf("items = %+v, warnings = %+v", d.Items, d.Warnings)
	}
}

func TestGetOrder_LooksUpEachProductOnce(t *testing.T) {
	o := storedOrder()
	o.Items = append(o.Items, o.Items[0]) // defensive: a product id repeated in stored data
	lookup := &fakeLookup{resp: model.ServiceLookupResponse{Products: []model.ServiceProduct{}, MissingIDs: []string{"p1", "p2"}}}

	d, _ := NewOrders(&fakeOrderAPI{order: o}, lookup, discard).Get(t.Context(), "o1")

	if !reflect.DeepEqual(lookup.gotIDs, []string{"p1", "p2"}) {
		t.Errorf("ids = %v, want unique ids", lookup.gotIDs)
	}
	if len(d.Warnings) != 2 {
		t.Errorf("warnings = %+v, want one per missing product", d.Warnings)
	}
}

func TestGetOrder_OrderServiceFailureFailsAndSkipsProductService(t *testing.T) {
	tests := []struct {
		name       string
		err        *client.Error
		wantStatus int
		wantCode   string
	}{
		{"404", &client.Error{Service: "order-service", Kind: client.KindStatus, Status: 404, Code: "NOT_FOUND", Message: "Order o9 not found"}, 404, "NOT_FOUND"},
		{"400", &client.Error{Service: "order-service", Kind: client.KindStatus, Status: 400, Code: "VALIDATION_ERROR"}, 400, "VALIDATION_ERROR"},
		{"5xx", &client.Error{Service: "order-service", Kind: client.KindUnavailable, Status: 500}, 503, "SERVICE_UNAVAILABLE"},
		{"unreachable", &client.Error{Service: "order-service", Kind: client.KindUnavailable}, 503, "SERVICE_UNAVAILABLE"},
		{"timeout", &client.Error{Service: "order-service", Kind: client.KindTimeout}, 504, "SERVICE_UNAVAILABLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := &fakeLookup{}

			_, err := NewOrders(&fakeOrderAPI{err: tt.err}, lookup, discard).Get(t.Context(), "o9")

			assertServiceError(t, err, tt.wantStatus, tt.wantCode)
			if lookup.calls != 0 {
				t.Error("the Product Service must not be called when the order cannot be read")
			}
		})
	}
}

func TestCreateOrder_ForwardsBodyUnchanged(t *testing.T) {
	orders := &fakeOrderAPI{order: storedOrder()}
	body := model.OrderCreate{CustomerID: ptr("c1"), Items: []model.OrderCreateItem{{ProductID: ptr("p1"), Quantity: ptr(2)}}}

	o, err := NewOrders(orders, &fakeLookup{}, discard).Create(t.Context(), body)

	if err != nil || !reflect.DeepEqual(orders.gotCreate, body) {
		t.Fatalf("err = %v, forwarded = %+v", err, orders.gotCreate)
	}
	if o.TotalAmount != "620.00" || o.Currency != "PLN" || len(o.Items) != 2 || o.Items[0].LineTotal != "600.00" {
		t.Errorf("order = %+v, want the Order Service values unchanged", o)
	}
}

func TestOrderErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        *client.Error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{"409 conflict", &client.Error{Service: "order-service", Kind: client.KindStatus, Status: 409, Code: "CONFLICT",
			Message: "Order o1 is already cancelled"}, 409, "CONFLICT", "Order o1 is already cancelled"},
		{"422 product not found", &client.Error{Service: "order-service", Kind: client.KindStatus, Status: 422,
			Code: "PRODUCT_NOT_FOUND", Message: "Product(s) not found: p2"}, 422, "PRODUCT_NOT_FOUND", "Product(s) not found: p2"},
		{"422 unavailable", &client.Error{Service: "order-service", Kind: client.KindStatus, Status: 422,
			Code: "PRODUCT_UNAVAILABLE", Message: "Product(s) not available: p1"}, 422, "PRODUCT_UNAVAILABLE", "Product(s) not available: p1"},
		{"422 currency", &client.Error{Service: "order-service", Kind: client.KindStatus, Status: 422,
			Code: "CURRENCY_MISMATCH", Message: "All products in an order must share one currency"}, 422, "CURRENCY_MISMATCH", "All products in an order must share one currency"},
		{"422 unknown code is a contract error", &client.Error{Service: "order-service", Kind: client.KindStatus, Status: 422,
			Code: "SOMETHING_NEW"}, 503, "SERVICE_UNAVAILABLE", "Order Service is temporarily unavailable"},
		{"503 product service unavailable", &client.Error{Service: "order-service", Kind: client.KindUnavailable, Status: 503,
			Code: "PRODUCT_SERVICE_UNAVAILABLE", Message: "internal detail"}, 503, "PRODUCT_SERVICE_UNAVAILABLE",
			"Product Service is temporarily unavailable; no order was created"},
		{"500 with a code is generic", &client.Error{Service: "order-service", Kind: client.KindUnavailable, Status: 500,
			Code: "INTERNAL_ERROR"}, 503, "SERVICE_UNAVAILABLE", "Order Service is temporarily unavailable"},
		{"timeout on create warns about the outcome", &client.Error{Service: "order-service", Kind: client.KindTimeout},
			504, "SERVICE_UNAVAILABLE", "Order Service did not respond in time; the order may or may not have been created"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewOrders(&fakeOrderAPI{err: tt.err}, &fakeLookup{}, discard).Create(t.Context(), model.OrderCreate{})

			se := assertServiceError(t, err, tt.wantStatus, tt.wantCode)
			if se.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", se.Message, tt.wantMsg)
			}
		})
	}
}

func TestOrder422KeepsDetails(t *testing.T) {
	details := []model.FieldDetail{{Field: "items[1].productId", Message: "Product p2 does not exist"}}
	orders := &fakeOrderAPI{err: &client.Error{Kind: client.KindStatus, Status: 422, Code: "PRODUCT_NOT_FOUND", Details: details}}

	_, err := NewOrders(orders, &fakeLookup{}, discard).Create(t.Context(), model.OrderCreate{})

	var se *Error
	if !errors.As(err, &se) || !reflect.DeepEqual(se.Details, details) {
		t.Errorf("err = %v, details = %+v", err, se)
	}
}

func TestCancelAndList(t *testing.T) {
	orders := &fakeOrderAPI{
		order: storedOrder(),
		page:  model.ServiceOrderPage{Content: []model.ServiceOrder{storedOrder()}, Page: 0, Size: 20, TotalElements: 1, TotalPages: 1},
	}
	svc := NewOrders(orders, &fakeLookup{}, discard)

	if o, err := svc.Cancel(t.Context(), "o1"); err != nil || orders.gotID != "o1" || o.Status != "CREATED" {
		t.Errorf("cancel: err = %v, id = %s, status = %s (status comes from the Order Service)", err, orders.gotID, o.Status)
	}

	page, err := svc.List(t.Context(), url.Values{"status": {"CREATED"}, "page": {"0"}, "debug": {"x"}})
	if err != nil || len(page.Content) != 1 || page.TotalElements != 1 {
		t.Fatalf("list: page = %+v, err = %v", page, err)
	}
	if !reflect.DeepEqual(orders.gotQuery, url.Values{"status": {"CREATED"}, "page": {"0"}}) {
		t.Errorf("forwarded query = %v", orders.gotQuery)
	}
}

func assertServiceError(t *testing.T, err error, status int, code string) *Error {
	t.Helper()
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want *service.Error", err)
	}
	if se.Status != status || se.Code != code {
		t.Fatalf("got %d %s (%q), want %d %s", se.Status, se.Code, se.Message, status, code)
	}
	return se
}
