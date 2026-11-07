package service

import (
	"context"
	"log/slog"
	"net/url"

	"bff-service/internal/model"
)

// OrderAPI is the subset of the Order Service client used here (an interface for tests).
type OrderAPI interface {
	Create(ctx context.Context, body model.OrderCreate) (model.ServiceOrder, error)
	Get(ctx context.Context, id string) (model.ServiceOrder, error)
	List(ctx context.Context, query url.Values) (model.ServiceOrderPage, error)
	Cancel(ctx context.Context, id string) (model.ServiceOrder, error)
}

// ProductLookupAPI is the batch lookup of the Product Service client.
type ProductLookupAPI interface {
	Lookup(ctx context.Context, ids []string) (model.ServiceLookupResponse, error)
}

// orderListParams are the only query parameters forwarded to the Order Service.
var orderListParams = []string{"page", "size", "status", "sort"}

var orderErrors = errorPolicy{
	conflict: true,
	ruleCodes: map[string]bool{
		model.CodeProductNotFound:    true,
		model.CodeProductUnavailable: true,
		model.CodeCurrencyMismatch:   true,
	},
	passUnavailableCodes: map[string]string{
		model.CodeProductServiceDown: "Product Service is temporarily unavailable; no order was created",
	},
}

// createErrors also warns that a timed-out POST may still have created the order (it is not
// idempotent and is never retried).
var createErrors = func() errorPolicy {
	p := orderErrors
	p.timeoutMessage = "Order Service did not respond in time; the order may or may not have been created"
	return p
}()

// Orders forwards order operations to the Order Service and, for a single order, adds current
// product data from the Product Service. All order logic (prices, totals, availability, status
// transitions) stays in the Order Service; the BFF only moves and combines data.
type Orders struct {
	orders   OrderAPI
	products ProductLookupAPI
	logger   *slog.Logger
}

// NewOrders creates the order service.
func NewOrders(orders OrderAPI, products ProductLookupAPI, logger *slog.Logger) *Orders {
	return &Orders{orders: orders, products: products, logger: logger}
}

// Create forwards the request unchanged.
func (s *Orders) Create(ctx context.Context, body model.OrderCreate) (model.Order, error) {
	o, err := s.orders.Create(ctx, body)
	if err != nil {
		return model.Order{}, mapDownstreamError(ctx, s.logger, err, createErrors)
	}
	return ToClientOrder(o), nil
}

// List returns a page of orders (snapshots only, no product lookup).
func (s *Orders) List(ctx context.Context, query url.Values) (model.OrderPage, error) {
	page, err := s.orders.List(ctx, pick(query, orderListParams))
	if err != nil {
		return model.OrderPage{}, mapDownstreamError(ctx, s.logger, err, orderErrors)
	}
	content := make([]model.Order, 0, len(page.Content))
	for _, o := range page.Content {
		content = append(content, ToClientOrder(o))
	}
	return model.OrderPage{
		Content:       content,
		Page:          page.Page,
		Size:          page.Size,
		TotalElements: page.TotalElements,
		TotalPages:    page.TotalPages,
	}, nil
}

// Cancel forwards the cancellation; the Order Service decides whether it is allowed.
func (s *Orders) Cancel(ctx context.Context, id string) (model.Order, error) {
	o, err := s.orders.Cancel(ctx, id)
	if err != nil {
		return model.Order{}, mapDownstreamError(ctx, s.logger, err, orderErrors)
	}
	return ToClientOrder(o), nil
}

// Get returns the order (primary source: Order Service) with current product data
// (supplementary source: Product Service). A failure of the primary source fails the request;
// a failure of the supplementary source only degrades the response (decision D8).
//
// The product lookup needs the product ids from the order, so the two calls are sequential;
// the lookup itself is a single batch call.
func (s *Orders) Get(ctx context.Context, id string) (model.OrderDetails, error) {
	o, err := s.orders.Get(ctx, id)
	if err != nil {
		return model.OrderDetails{}, mapDownstreamError(ctx, s.logger, err, orderErrors)
	}

	details := toOrderDetails(o)
	ids := uniqueProductIDs(o.Items)
	if len(ids) == 0 {
		return details, nil
	}

	lookup, err := s.products.Lookup(ctx, ids)
	if err != nil {
		s.logger.WarnContext(ctx, "current product data unavailable; returning degraded order",
			"orderId", o.ID, "error", err)
		details.Warnings = append(details.Warnings, model.Warning{
			Code:    model.WarningProductDataUnavailable,
			Message: "Current product data is temporarily unavailable; the order snapshot is complete",
		})
		return details, nil
	}

	applyCurrentProducts(&details, lookup)
	return details, nil
}

// applyCurrentProducts fills currentProduct per item from one lookup result.
func applyCurrentProducts(details *model.OrderDetails, lookup model.ServiceLookupResponse) {
	found := make(map[string]model.ServiceProduct, len(lookup.Products))
	for _, p := range lookup.Products {
		found[p.ID] = p
	}
	missing := make(map[string]bool, len(lookup.MissingIDs))
	for _, id := range lookup.MissingIDs {
		missing[id] = true
	}

	warned := map[string]bool{}
	unknownWarned := false
	for i := range details.Items {
		item := &details.Items[i]
		productID := item.ProductSnapshot.ProductID
		switch p, ok := found[productID]; {
		case ok:
			item.CurrentProduct = &model.CurrentProduct{
				Name:      p.Name,
				Price:     p.Price,
				Currency:  p.Currency,
				Available: p.Available,
			}
			item.CurrentProductStatus = model.CurrentProductOK
		case missing[productID]:
			item.CurrentProductStatus = model.CurrentProductNotFound
			if !warned[productID] {
				warned[productID] = true
				details.Warnings = append(details.Warnings, model.Warning{
					Code:      model.WarningProductNotFound,
					Message:   "Product " + productID + " no longer exists; only the order snapshot is available",
					ProductID: &productID,
				})
			}
		default:
			// Neither found nor reported missing: the lookup answer is incomplete for this item.
			item.CurrentProductStatus = model.CurrentProductUnknown
			if !unknownWarned {
				unknownWarned = true
				details.Warnings = append(details.Warnings, model.Warning{
					Code:    model.WarningProductDataUnavailable,
					Message: "Current product data is incomplete; the order snapshot is complete",
				})
			}
		}
	}
}

// ToClientOrder converts the Order Service representation into the client contract. Values
// (decimal-string amounts, currencies, statuses) are copied unchanged; nothing is recomputed.
func ToClientOrder(o model.ServiceOrder) model.Order {
	items := make([]model.OrderItem, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, toOrderItem(it))
	}
	return model.Order{
		ID:          o.ID,
		CustomerID:  o.CustomerID,
		Status:      o.Status,
		Items:       items,
		TotalAmount: o.TotalAmount,
		Currency:    o.Currency,
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   o.UpdatedAt,
	}
}

// toOrderDetails starts every item as UNKNOWN; product data is applied afterwards.
func toOrderDetails(o model.ServiceOrder) model.OrderDetails {
	items := make([]model.OrderDetailsItem, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, model.OrderDetailsItem{
			OrderItem:            toOrderItem(it),
			CurrentProductStatus: model.CurrentProductUnknown,
		})
	}
	return model.OrderDetails{
		ID:          o.ID,
		CustomerID:  o.CustomerID,
		Status:      o.Status,
		Items:       items,
		TotalAmount: o.TotalAmount,
		Currency:    o.Currency,
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   o.UpdatedAt,
		Warnings:    []model.Warning{},
	}
}

func toOrderItem(it model.ServiceOrderItem) model.OrderItem {
	return model.OrderItem{
		ProductSnapshot: model.ProductSnapshot{
			ProductID: it.ProductID,
			Name:      it.ProductName,
			UnitPrice: it.UnitPrice,
			Currency:  it.Currency,
		},
		Quantity:  it.Quantity,
		LineTotal: it.LineTotal,
	}
}

func uniqueProductIDs(items []model.ServiceOrderItem) []string {
	seen := make(map[string]bool, len(items))
	ids := make([]string, 0, len(items))
	for _, it := range items {
		if !seen[it.ProductID] {
			seen[it.ProductID] = true
			ids = append(ids, it.ProductID)
		}
	}
	return ids
}
