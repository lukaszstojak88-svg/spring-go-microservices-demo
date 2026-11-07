package model

// ---- Client-facing contract (BFF API) ----

// OrderCreateItem is one line of a create-order request: a product reference and a quantity,
// never a price.
type OrderCreateItem struct {
	ProductID *string `json:"productId"`
	Quantity  *int    `json:"quantity"`
}

// OrderCreate is the client-facing body of POST /api/orders. It is forwarded unchanged.
type OrderCreate struct {
	CustomerID *string           `json:"customerId"`
	Items      []OrderCreateItem `json:"items"`
}

// ProductSnapshot is the product data stored in the order at creation time (immutable).
// Amounts are decimal strings; the currency is a separate field.
type ProductSnapshot struct {
	ProductID string `json:"productId"`
	Name      string `json:"name"`
	UnitPrice string `json:"unitPrice"`
	Currency  string `json:"currency"`
}

// OrderItem is one order line as stored by the Order Service. LineTotal is in the snapshot
// currency.
type OrderItem struct {
	ProductSnapshot ProductSnapshot `json:"productSnapshot"`
	Quantity        int             `json:"quantity"`
	LineTotal       string          `json:"lineTotal"`
}

// Order is the client-facing order (create, list and cancel responses).
type Order struct {
	ID          string      `json:"id"`
	CustomerID  *string     `json:"customerId"`
	Status      string      `json:"status"`
	Items       []OrderItem `json:"items"`
	TotalAmount string      `json:"totalAmount"`
	Currency    string      `json:"currency"`
	CreatedAt   string      `json:"createdAt"`
	UpdatedAt   string      `json:"updatedAt"`
}

// OrderPage is a page of client-facing orders.
type OrderPage struct {
	Content       []Order `json:"content"`
	Page          int     `json:"page"`
	Size          int     `json:"size"`
	TotalElements int64   `json:"totalElements"`
	TotalPages    int     `json:"totalPages"`
}

// Current product statuses of an aggregated order item.
const (
	CurrentProductOK       = "OK"
	CurrentProductNotFound = "NOT_FOUND"
	CurrentProductUnknown  = "UNKNOWN"
)

// Warning codes of a degraded order response.
const (
	WarningProductNotFound        = "PRODUCT_NOT_FOUND"
	WarningProductDataUnavailable = "PRODUCT_DATA_UNAVAILABLE"
)

// CurrentProduct is the product data as it is NOW (supplementary, from the Product Service).
type CurrentProduct struct {
	Name      string `json:"name"`
	Price     string `json:"price"`
	Currency  string `json:"currency"`
	Available bool   `json:"available"`
}

// OrderDetailsItem is an order line with the snapshot and the current product side by side;
// the snapshot is never overwritten or merged with current data.
type OrderDetailsItem struct {
	OrderItem
	CurrentProduct       *CurrentProduct `json:"currentProduct"`
	CurrentProductStatus string          `json:"currentProductStatus"`
}

// Warning explains why part of a response is missing.
type Warning struct {
	Code      string  `json:"code"`
	Message   string  `json:"message"`
	ProductID *string `json:"productId,omitempty"`
}

// OrderDetails is the response of GET /api/orders/{id}.
type OrderDetails struct {
	ID          string             `json:"id"`
	CustomerID  *string            `json:"customerId"`
	Status      string             `json:"status"`
	Items       []OrderDetailsItem `json:"items"`
	TotalAmount string             `json:"totalAmount"`
	Currency    string             `json:"currency"`
	CreatedAt   string             `json:"createdAt"`
	UpdatedAt   string             `json:"updatedAt"`
	Warnings    []Warning          `json:"warnings"`
}

// ---- Order Service contract (the BFF's own copy) ----

// ServiceOrderItem is an order line as returned by the Order Service.
type ServiceOrderItem struct {
	ProductID   string `json:"productId"`
	ProductName string `json:"productName"`
	UnitPrice   string `json:"unitPrice"`
	Currency    string `json:"currency"`
	Quantity    int    `json:"quantity"`
	LineTotal   string `json:"lineTotal"`
}

// ServiceOrder is an order as returned by the Order Service.
type ServiceOrder struct {
	ID          string             `json:"id"`
	CustomerID  *string            `json:"customerId"`
	Status      string             `json:"status"`
	Items       []ServiceOrderItem `json:"items"`
	TotalAmount string             `json:"totalAmount"`
	Currency    string             `json:"currency"`
	CreatedAt   string             `json:"createdAt"`
	UpdatedAt   string             `json:"updatedAt"`
}

// IsComplete reports whether the mandatory fields are present.
func (o ServiceOrder) IsComplete() bool {
	if o.ID == "" || o.Status == "" || o.TotalAmount == "" || o.Currency == "" || o.Items == nil {
		return false
	}
	for _, it := range o.Items {
		if it.ProductID == "" || it.UnitPrice == "" || it.Currency == "" || it.LineTotal == "" {
			return false
		}
	}
	return true
}

// ServiceOrderPage is a page as returned by the Order Service.
type ServiceOrderPage struct {
	Content       []ServiceOrder `json:"content"`
	Page          int            `json:"page"`
	Size          int            `json:"size"`
	TotalElements int64          `json:"totalElements"`
	TotalPages    int            `json:"totalPages"`
}

// ---- Product Service batch lookup contract ----

// ServiceLookupRequest is the body of POST /api/products/lookup.
type ServiceLookupRequest struct {
	IDs []string `json:"ids"`
}

// ServiceLookupResponse lists found products and the ids that do not exist.
type ServiceLookupResponse struct {
	Products   []ServiceProduct `json:"products"`
	MissingIDs []string         `json:"missingIds"`
}
