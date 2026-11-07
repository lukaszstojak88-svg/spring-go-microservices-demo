package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"bff-service/internal/client"
	"bff-service/internal/model"
)

type fakeProductAPI struct {
	err       error
	product   model.ServiceProduct
	page      model.ServiceProductPage
	gotQuery  url.Values
	gotWrite  model.ServiceProductWrite
	gotID     string
	callCount int
}

func (f *fakeProductAPI) List(_ context.Context, q url.Values) (model.ServiceProductPage, error) {
	f.callCount++
	f.gotQuery = q
	return f.page, f.err
}

func (f *fakeProductAPI) Get(_ context.Context, id string) (model.ServiceProduct, error) {
	f.callCount++
	f.gotID = id
	return f.product, f.err
}

func (f *fakeProductAPI) Create(_ context.Context, w model.ServiceProductWrite) (model.ServiceProduct, error) {
	f.callCount++
	f.gotWrite = w
	return f.product, f.err
}

func (f *fakeProductAPI) Update(_ context.Context, id string, w model.ServiceProductWrite) (model.ServiceProduct, error) {
	f.callCount++
	f.gotID, f.gotWrite = id, w
	return f.product, f.err
}

func (f *fakeProductAPI) Delete(_ context.Context, id string) error {
	f.callCount++
	f.gotID = id
	return f.err
}

func ptr[T any](v T) *T { return &v }

var discard = slog.New(slog.DiscardHandler)

var keyboard = model.ServiceProduct{
	ID: "p1", Name: "Mechanical Keyboard", Description: ptr("Compact"), Price: "349.99", Currency: "PLN",
	Available: true, CreatedAt: "2026-01-15T10:30:00Z", UpdatedAt: "2026-01-15T10:31:00.123Z",
}

func TestToClientProduct_CopiesAllFieldsUnchanged(t *testing.T) {
	got := ToClientProduct(keyboard)

	want := model.Product{
		ID: "p1", Name: "Mechanical Keyboard", Description: ptr("Compact"),
		Price: "349.99", Currency: "PLN",
		Available: true, CreatedAt: "2026-01-15T10:30:00Z", UpdatedAt: "2026-01-15T10:31:00.123Z",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestToClientProduct_KeepsAmountStringExactly(t *testing.T) {
	// Values that a float64 round-trip would change or reformat.
	for _, amount := range []string{"0.10", "20.00", "1500", "12345678901234567.89", "0.30"} {
		p := keyboard
		p.Price = amount
		if got := ToClientProduct(p).Price; got != amount {
			t.Errorf("amount %q became %q", amount, got)
		}
	}
}

func TestToServiceWrite(t *testing.T) {
	got := ToServiceWrite(model.ProductWrite{
		Name: ptr("A"), Description: ptr("d"), Price: ptr("20"), Currency: ptr("PLN"), Available: ptr(false),
	})
	want := model.ServiceProductWrite{
		Name: ptr("A"), Description: ptr("d"), Price: ptr("20"), Currency: ptr("PLN"), Available: ptr(false),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	empty := ToServiceWrite(model.ProductWrite{})
	if empty.Price != nil || empty.Currency != nil || empty.Name != nil {
		t.Errorf("missing values must stay null: %+v", empty)
	}
}

func TestList_ForwardsOnlyPagingParamsAndMapsPage(t *testing.T) {
	api := &fakeProductAPI{page: model.ServiceProductPage{
		Content: []model.ServiceProduct{keyboard}, Page: 2, Size: 10, TotalElements: 21, TotalPages: 3,
	}}

	page, err := NewProducts(api, discard).List(t.Context(), url.Values{
		"page": {"2"}, "size": {"10"}, "sort": {"price,desc"}, "debug": {"true"}, "host": {"x"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(api.gotQuery, url.Values{"page": {"2"}, "size": {"10"}, "sort": {"price,desc"}}) {
		t.Errorf("forwarded query = %v", api.gotQuery)
	}
	if page.Page != 2 || page.Size != 10 || page.TotalElements != 21 || page.TotalPages != 3 ||
		len(page.Content) != 1 || page.Content[0].Price != "349.99" || page.Content[0].Currency != "PLN" {
		t.Errorf("unexpected page: %+v", page)
	}
}

func TestList_EmptyContentIsEmptyArray(t *testing.T) {
	api := &fakeProductAPI{page: model.ServiceProductPage{Content: []model.ServiceProduct{}}}

	page, _ := NewProducts(api, discard).List(t.Context(), nil)

	if page.Content == nil {
		t.Error("content must be [] not null")
	}
}

func TestCreateAndUpdate_TransformRequest(t *testing.T) {
	api := &fakeProductAPI{product: keyboard}
	svc := NewProducts(api, discard)
	body := model.ProductWrite{Name: ptr("A"), Price: ptr("1.00"), Currency: ptr("EUR")}

	if _, err := svc.Create(t.Context(), body); err != nil || *api.gotWrite.Price != "1.00" || *api.gotWrite.Currency != "EUR" {
		t.Errorf("create: err = %v, write = %+v", err, api.gotWrite)
	}
	if _, err := svc.Update(t.Context(), "p1", body); err != nil || api.gotID != "p1" || *api.gotWrite.Price != "1.00" {
		t.Errorf("update: err = %v, id = %s", err, api.gotID)
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{"validation", &client.Error{Service: "product-service", Kind: client.KindStatus, Status: 400,
			Code: "VALIDATION_ERROR", Message: "Request validation failed"},
			400, "VALIDATION_ERROR", "Request validation failed"},
		{"not found", &client.Error{Service: "product-service", Kind: client.KindStatus, Status: 404,
			Code: "NOT_FOUND", Message: "Product p9 not found"},
			404, "NOT_FOUND", "Product p9 not found"},
		{"not found without body", &client.Error{Service: "product-service", Kind: client.KindStatus, Status: 404},
			404, "NOT_FOUND", "Resource not found"},
		{"unexpected 4xx is a contract error", &client.Error{Service: "product-service", Kind: client.KindStatus, Status: 415},
			503, "SERVICE_UNAVAILABLE", "Product Service is temporarily unavailable"},
		{"5xx", &client.Error{Service: "product-service", Kind: client.KindUnavailable, Status: 500},
			503, "SERVICE_UNAVAILABLE", "Product Service is temporarily unavailable"},
		{"unreachable", &client.Error{Service: "product-service", Kind: client.KindUnavailable,
			Err: errors.New("dial tcp 10.0.0.7:8081: connection refused")},
			503, "SERVICE_UNAVAILABLE", "Product Service is temporarily unavailable"},
		{"invalid response", &client.Error{Service: "product-service", Kind: client.KindInvalidResponse},
			503, "SERVICE_UNAVAILABLE", "Product Service is temporarily unavailable"},
		{"timeout", &client.Error{Service: "product-service", Kind: client.KindTimeout},
			504, "SERVICE_UNAVAILABLE", "Product Service did not respond in time"},
		{"canceled", &client.Error{Service: "product-service", Kind: client.KindCanceled},
			503, "SERVICE_UNAVAILABLE", "Product Service is temporarily unavailable"},
		{"unknown error", errors.New("boom at product-service:8081"),
			500, "INTERNAL_ERROR", "An unexpected error occurred"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewProducts(&fakeProductAPI{err: tt.err}, discard).Get(t.Context(), "p1")

			var se *Error
			if !errors.As(err, &se) {
				t.Fatalf("err = %v, want *service.Error", err)
			}
			if se.Status != tt.wantStatus || se.Code != tt.wantCode || se.Message != tt.wantMsg {
				t.Errorf("got %d %s %q, want %d %s %q", se.Status, se.Code, se.Message, tt.wantStatus, tt.wantCode, tt.wantMsg)
			}
			if strings.Contains(se.Message, "8081") || strings.Contains(se.Message, "dial") {
				t.Errorf("message leaks internals: %q", se.Message)
			}
		})
	}
}

func TestErrorMapping_KeepsValidationFieldNames(t *testing.T) {
	// The client contract uses the Product Service field names, so details pass through unchanged.
	details := []model.FieldDetail{
		{Field: "price", Message: "must have at most 2 fraction digits"},
		{Field: "currency", Message: "must be a valid ISO-4217 code"},
		{Field: "name", Message: "must not be blank"},
	}
	api := &fakeProductAPI{err: &client.Error{Service: "product-service", Kind: client.KindStatus, Status: 400,
		Code: "VALIDATION_ERROR", Message: "Request validation failed", Details: details}}

	_, err := NewProducts(api, discard).Create(t.Context(), model.ProductWrite{})

	var se *Error
	if !errors.As(err, &se) || !reflect.DeepEqual(se.Details, details) {
		t.Errorf("err = %v, details = %+v, want %+v", err, se, details)
	}
}

func TestErrorMapping_DetailsAreNeverNil(t *testing.T) {
	api := &fakeProductAPI{err: &client.Error{Kind: client.KindStatus, Status: 400}}

	_, err := NewProducts(api, discard).Create(t.Context(), model.ProductWrite{})

	var se *Error
	if !errors.As(err, &se) || se.Details == nil {
		t.Errorf("details must be [] not null: %+v", se)
	}
}

func TestDelete(t *testing.T) {
	api := &fakeProductAPI{}
	if err := NewProducts(api, discard).Delete(t.Context(), "p1"); err != nil || api.gotID != "p1" {
		t.Errorf("err = %v, id = %s", err, api.gotID)
	}
	api.err = &client.Error{Kind: client.KindStatus, Status: http.StatusNotFound}
	var se *Error
	if err := NewProducts(api, discard).Delete(t.Context(), "p1"); !errors.As(err, &se) || se.Status != 404 {
		t.Errorf("err = %v, want 404", err)
	}
}

func TestReadiness(t *testing.T) {
	ok := func(context.Context) error { return nil }
	fail := func(context.Context) error { return errors.New("down") }

	all := NewReadiness(discard, ReadinessCheck{"productService", ok}, ReadinessCheck{"orderService", ok}).Check(t.Context())
	if all.Status != "UP" || all.Checks["productService"] != "UP" || all.Checks["orderService"] != "UP" {
		t.Errorf("all up: %+v", all)
	}

	one := NewReadiness(discard, ReadinessCheck{"productService", ok}, ReadinessCheck{"orderService", fail}).Check(t.Context())
	if one.Status != "DOWN" || one.Checks["productService"] != "UP" || one.Checks["orderService"] != "DOWN" {
		t.Errorf("one down: %+v", one)
	}
}

func TestReadiness_RunsChecksConcurrently(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	check := func(context.Context) error {
		started <- struct{}{}
		<-release
		return nil
	}
	done := make(chan model.ReadinessResponse)
	go func() {
		done <- NewReadiness(discard, ReadinessCheck{"a", check}, ReadinessCheck{"b", check}).Check(t.Context())
	}()

	<-started
	<-started // both checks are running at the same time
	close(release)
	if r := <-done; r.Status != "UP" {
		t.Errorf("status = %s", r.Status)
	}
}
