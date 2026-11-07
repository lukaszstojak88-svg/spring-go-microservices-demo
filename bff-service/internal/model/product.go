package model

// ---- Client-facing contract (BFF API) ----

// Product is the client-facing product representation. The price is a decimal string in the
// separate currency field; it is never converted to a floating-point number.
type Product struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Price       string  `json:"price"`
	Currency    string  `json:"currency"`
	Available   bool    `json:"available"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

// ProductPage is a page of client-facing products.
type ProductPage struct {
	Content       []Product `json:"content"`
	Page          int       `json:"page"`
	Size          int       `json:"size"`
	TotalElements int64     `json:"totalElements"`
	TotalPages    int       `json:"totalPages"`
}

// ProductWrite is the client-facing body of POST and PUT /api/products. Pointers keep "missing"
// distinct from "empty", so the Product Service can report required fields.
type ProductWrite struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Price       *string `json:"price"`
	Currency    *string `json:"currency"`
	Available   *bool   `json:"available"`
}

// ---- Product Service contract (the BFF's own copy) ----

// ServiceProduct is a product as returned by the Product Service. Timestamps are kept as the
// original strings so they are passed through unchanged.
type ServiceProduct struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Price       string  `json:"price"`
	Currency    string  `json:"currency"`
	Available   bool    `json:"available"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

// IsComplete reports whether the mandatory fields are present.
func (p ServiceProduct) IsComplete() bool {
	return p.ID != "" && p.Name != "" && p.Price != "" && p.Currency != ""
}

// ServiceProductPage is a page as returned by the Product Service.
type ServiceProductPage struct {
	Content       []ServiceProduct `json:"content"`
	Page          int              `json:"page"`
	Size          int              `json:"size"`
	TotalElements int64            `json:"totalElements"`
	TotalPages    int              `json:"totalPages"`
}

// ServiceProductWrite is the Product Service body of POST and PUT /api/products.
// Missing values are sent as null; the Product Service owns all validation.
type ServiceProductWrite struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Price       *string `json:"price"`
	Currency    *string `json:"currency"`
	Available   *bool   `json:"available"`
}

// ReadinessResponse is the body of GET /ready.
type ReadinessResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}
