# BFF API (client-facing contract)

The BFF (`bff-service`, port 8080) is the only public API. Its contract is
**client-facing** and intentionally differs from the Java service contracts
(see `docs/PROJECT_SPEC.md` §5.3). This document covers the product
endpoints, health/readiness and the shared conventions. Order endpoints are
added in a later stage.

## Conventions

-   JSON only (`Content-Type: application/json`).
-   Money is an object `{ "amount": "349.99", "currency": "PLN" }`. The
    amount is a **decimal string**; the BFF never converts it to a number
    and never changes it (scale and rounding rules belong to the Product
    Service, see `docs/architecture.md` §9.1).
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

The BFF forwards the requests to the Product Service and translates the
representation. It holds no product rules: required fields, value ranges,
currency codes and money scale are validated by the Product Service.

### Product

```json
{
  "id": "673abc1234567890abcdef12",
  "name": "Mechanical Keyboard",
  "description": "Compact mechanical keyboard",
  "price": { "amount": "349.99", "currency": "PLN" },
  "available": true,
  "createdAt": "2026-01-15T10:30:00Z",
  "updatedAt": "2026-01-15T10:30:00Z"
}
```

Mapping from the Product Service: `price` (string) → `price.amount`,
`currency` → `price.currency`; all other fields are copied as is.

### Create / update request

```json
{
  "name": "Mechanical Keyboard",
  "description": "Compact mechanical keyboard",
  "price": { "amount": "349.99", "currency": "PLN" },
  "available": true
}
```

The BFF only checks the structure of the body before calling the Product
Service:

-   a single JSON object, at most 1 MiB, no trailing data;
-   fields have the expected JSON types (`price` is an object,
    `price.amount` and `price.currency` are strings, `available` is a
    boolean);
-   `price.amount`, when present, has a decimal shape (`-?digits[.digits]`).

Anything else (missing fields, negative prices, too many fraction digits,
unknown currencies) is reported by the Product Service and returned as
`400 VALIDATION_ERROR` with field names translated to this contract
(`price` → `price.amount`, `currency` → `price.currency`).

### List

Query parameters `page` (default 0), `size` (1-100, default 20) and
`sort` (`field,asc|desc`; fields `name`, `price`, `createdAt`,
`updatedAt`) are forwarded; other parameters are ignored.

```json
{ "content": [ { "...": "product" } ], "page": 0, "size": 20, "totalElements": 1, "totalPages": 1 }
```

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
  "details": [ { "field": "price.amount", "message": "must be a decimal string, e.g. \"349.99\"" } ]
}
```

Downstream error bodies are never forwarded verbatim; stack traces,
hostnames, ports and internal exception messages are never returned.

| Situation | BFF response |
|---|---|
| Malformed or structurally invalid body | `400 VALIDATION_ERROR` (Product Service not called) |
| Product Service `400` | `400 VALIDATION_ERROR` with translated `details` |
| Product Service `404` | `404 NOT_FOUND` |
| Product Service other `4xx` (contract error, logged) | `503 SERVICE_UNAVAILABLE` |
| Product Service `5xx`, unreachable, invalid response | `503 SERVICE_UNAVAILABLE` |
| Product Service timeout (`PRODUCT_CLIENT_TIMEOUT`) | `504 SERVICE_UNAVAILABLE` |
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
