# BFF API (client-facing contract)

The BFF (`bff-service`, port 8080) is the only public API. Its contract is
**client-facing** and intentionally differs from the Java service contracts
(see `docs/PROJECT_SPEC.md` §5.3). This document covers the product and
order endpoints, health/readiness and the shared conventions.

## Conventions

-   JSON only (`Content-Type: application/json`).
-   Money is an amount field holding a **decimal string** (for example
    `"price": "349.99"`) plus a separate `currency` field (ISO-4217, for
    example `"currency": "PLN"`). The BFF never converts amounts to
    numbers and never changes them (scale and rounding rules belong to the
    Java services, see `docs/architecture.md` §9.1).
-   Timestamps are ISO-8601 UTC strings, passed through unchanged.
-   Identifiers are opaque strings.
-   `X-Request-Id`: if the request carries a valid value
    (`[A-Za-z0-9._-]{1,128}`) it is reused, otherwise a UUID is generated.
    The value is returned in the response header, forwarded to every
    downstream call, written to the logs and included in error bodies.

## Product endpoints

| Method | Path | Success | Description |
|---|---|---|---|
| `GET` | `/api/products?page=&size=&sort=` | `200` | List products (paginated) |
| `GET` | `/api/products/{id}` | `200` | Get a product |
| `POST` | `/api/products` | `201` + `Location: /api/products/{id}` | Create a product |
| `PUT` | `/api/products/{id}` | `200` | Replace the editable fields of a product |
| `DELETE` | `/api/products/{id}` | `204` | Delete a product |

The BFF forwards the requests to the Product Service. The product shape is
the same as in the Product Service; all fields are copied unchanged. The
BFF holds no product rules: required fields, value ranges, currency codes
and money scale are validated by the Product Service.

### Product

```json
{
  "id": "673abc1234567890abcdef12",
  "name": "Mechanical Keyboard",
  "description": "Compact mechanical keyboard",
  "price": "349.99",
  "currency": "PLN",
  "available": true,
  "createdAt": "2026-01-15T10:30:00Z",
  "updatedAt": "2026-01-15T10:30:00Z"
}
```

### Create / update request

```json
{
  "name": "Mechanical Keyboard",
  "description": "Compact mechanical keyboard",
  "price": "349.99",
  "currency": "PLN",
  "available": true
}
```

The BFF only checks the structure of the body before calling the Product
Service:

-   a single JSON object, at most 1 MiB, no trailing data;
-   fields have the expected JSON types (`price` and `currency` are
    strings, `available` is a boolean); a JSON number as `price` is
    rejected because amounts are decimal strings;
-   `price`, when present, has a decimal shape (`-?digits[.digits]`).

Anything else (missing fields, negative prices, too many fraction digits,
unknown currencies) is reported by the Product Service and returned as
`400 VALIDATION_ERROR` with its `details` (field names are the same in
both contracts).

### List

Query parameters `page` (default 0), `size` (1-100, default 20) and
`sort` (`field,asc|desc`; fields `name`, `price`, `createdAt`,
`updatedAt`) are forwarded; other parameters are ignored.

```json
{ "content": [ { "...": "product" } ], "page": 0, "size": 20, "totalElements": 1, "totalPages": 1 }
```

## Order endpoints

| Method | Path | Success | Description |
|---|---|---|---|
| `POST` | `/api/orders` | `201` + `Location: /api/orders/{id}` | Create an order |
| `GET` | `/api/orders?page=&size=&status=&sort=` | `200` | List orders (paginated, snapshots only) |
| `GET` | `/api/orders/{id}` | `200` | Get an order with current product data |
| `POST` | `/api/orders/{id}/cancel` | `200` | Cancel an order |

All order logic lives in the Order Service: product verification, prices,
line totals, the order total and status transitions. The BFF forwards the
requests, groups the stored product data into `productSnapshot` and never
computes, checks or changes anything.

### Create request

```json
{ "customerId": "customer-42", "items": [ { "productId": "673abc...", "quantity": 2 } ] }
```

The body contains no prices and is forwarded unchanged. The BFF only
checks the JSON structure (an object; `items` an array; `productId` a
string; `quantity` an integer, so `1.5` is rejected with
`items[0].quantity`). Empty items, non-positive quantities, duplicates,
missing or unavailable products and mixed currencies are reported by the
Order Service (`400`/`422`).

`POST /api/orders` is **not idempotent** and is never retried. If the
Order Service does not answer in time the BFF returns `504` and the order
may or may not have been created (see `PROJECT_SPEC.md` §5.4).

### Order (create, list and cancel responses)

```json
{
  "id": "674def4567890abcdef12345",
  "customerId": "customer-42",
  "status": "CREATED",
  "items": [
    {
      "productSnapshot": {
        "productId": "673abc1234567890abcdef12",
        "name": "Mechanical Keyboard",
        "unitPrice": "349.99",
        "currency": "PLN"
      },
      "quantity": 2,
      "lineTotal": "699.98"
    }
  ],
  "totalAmount": "699.98",
  "currency": "PLN",
  "createdAt": "2026-01-15T11:00:00Z",
  "updatedAt": "2026-01-15T11:00:00Z"
}
```

Mapping from the Order Service: `productId`, `productName` → `name`,
`unitPrice` and the item `currency` go into `productSnapshot`; `quantity`,
`lineTotal` (in the snapshot currency), `totalAmount` and `currency` keep
their names. Values are copied unchanged.

### Get order with current product data

`GET /api/orders/{id}` reads the order from the Order Service (primary
source), then fetches the current data of all its products with **one**
batch lookup in the Product Service (supplementary source). The lookup
needs the product ids from the order, so the two calls are sequential.
Every item contains the immutable `productSnapshot` and, side by side, the
current product; the snapshot is never overwritten or merged.

```json
{
  "id": "674def4567890abcdef12345",
  "status": "CREATED",
  "items": [
    {
      "productSnapshot": { "productId": "p1", "name": "Mechanical Keyboard", "unitPrice": "349.99", "currency": "PLN" },
      "quantity": 2,
      "lineTotal": "699.98",
      "currentProduct": { "name": "Mechanical Keyboard v2", "price": "399.00", "currency": "PLN", "available": false },
      "currentProductStatus": "OK"
    }
  ],
  "totalAmount": "699.98",
  "currency": "PLN",
  "createdAt": "2026-01-15T11:00:00Z",
  "updatedAt": "2026-01-15T11:00:00Z",
  "warnings": []
}
```

| Situation | Response |
|---|---|
| Both services OK | `200`, every item `currentProductStatus: "OK"` |
| A product no longer exists | `200`; that item `currentProduct: null`, `"NOT_FOUND"`; warning `PRODUCT_NOT_FOUND` with `productId` |
| Product Service down, timeout, `5xx`, unexpected `4xx` or invalid response | `200`; all items `currentProduct: null`, `"UNKNOWN"`; warning `PRODUCT_DATA_UNAVAILABLE` |
| Order Service `404` | `404 NOT_FOUND` (Product Service not called) |
| Order Service down or `5xx` | `503 SERVICE_UNAVAILABLE` (Product Service not called) |
| Order Service timeout (`ORDER_CLIENT_TIMEOUT`) | `504 SERVICE_UNAVAILABLE` |

`warnings` is always present (an empty array when the response is complete).

## Health and readiness

| Path | Description |
|---|---|
| `GET /health` | Liveness, `200 {"status":"UP"}`; never calls downstream services |
| `GET /ready` | Calls the readiness probes of the Product and Order Service concurrently; `200` when both are ready, otherwise `503` |

```json
{ "status": "DOWN", "checks": { "productService": "UP", "orderService": "DOWN" } }
```

## Errors

Every error uses the common structure:

```json
{
  "timestamp": "2026-01-15T10:30:00.123Z",
  "status": 400,
  "code": "VALIDATION_ERROR",
  "message": "Request validation failed",
  "path": "/api/products",
  "requestId": "b1c2d3",
  "details": [ { "field": "price", "message": "must be a decimal string, e.g. \"349.99\"" } ]
}
```

Downstream error bodies are never forwarded verbatim; stack traces,
hostnames, ports and internal exception messages are never returned.

| Situation | BFF response |
|---|---|
| Malformed or structurally invalid body | `400 VALIDATION_ERROR` (Product Service not called) |
| Product Service `400` | `400 VALIDATION_ERROR` with `details` |
| Product Service `404` | `404 NOT_FOUND` |
| Product Service other `4xx` (contract error, logged) | `503 SERVICE_UNAVAILABLE` |
| Product Service `5xx`, unreachable, invalid response | `503 SERVICE_UNAVAILABLE` |
| Product Service timeout (`PRODUCT_CLIENT_TIMEOUT`) | `504 SERVICE_UNAVAILABLE` |
| Order Service `400` | `400 VALIDATION_ERROR` with `details` |
| Order Service `404` | `404 NOT_FOUND` |
| Order Service `409` (e.g. order already cancelled) | `409 CONFLICT` |
| Order Service `422` `PRODUCT_NOT_FOUND` / `PRODUCT_UNAVAILABLE` / `CURRENCY_MISMATCH` | `422` with the same code, message and `details` |
| Order Service `503 PRODUCT_SERVICE_UNAVAILABLE` (order creation) | `503 PRODUCT_SERVICE_UNAVAILABLE`, no order was created |
| Order Service other `4xx`/`422` code (contract error, logged) | `503 SERVICE_UNAVAILABLE` |
| Order Service `5xx`, unreachable, invalid response | `503 SERVICE_UNAVAILABLE` |
| Order Service timeout (`ORDER_CLIENT_TIMEOUT`) | `504 SERVICE_UNAVAILABLE` |
| Unknown path / unsupported method | `404 NOT_FOUND` / `405 METHOD_NOT_ALLOWED` (with `Allow`) |
| Unexpected BFF failure (panic) | `500 INTERNAL_ERROR` |

Calls are made once: the BFF does not retry downstream requests.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `BFF_PORT` | `8080` | Listening port |
| `PRODUCT_SERVICE_URL` | required | Product Service base URL |
| `ORDER_SERVICE_URL` | required | Order Service base URL |
| `PRODUCT_CLIENT_TIMEOUT` | `3s` | Timeout of a whole Product Service call |
| `ORDER_CLIENT_TIMEOUT` | `5s` | Timeout of a whole Order Service call |

In Docker Compose the timeouts are set from `BFF_PRODUCT_TIMEOUT` and
`BFF_ORDER_TIMEOUT` in `.env`; `BFF_PORT` in `.env` is the published host
port (the container always listens on 8080).
