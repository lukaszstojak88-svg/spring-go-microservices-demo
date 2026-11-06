# Architecture

This document describes the target architecture. The normative
requirements are in [PROJECT_SPEC.md](PROJECT_SPEC.md); this document
explains the design, flows, data model, decisions, and risks.

## 1. Overview

Three independent services in one monorepo, one MongoDB instance with two
logical databases.

| Service | Tech | Port | Owns | Database |
|---|---|---|---|---|
| Product Service | Java 21, Spring Boot, Maven, MapStruct, Spring Data MongoDB | 8081 | Products | `products_db` |
| Order Service | Java 21, Spring Boot, Maven, MapStruct, Spring Data MongoDB | 8082 | Orders | `orders_db` |
| BFF Service | Go, `net/http` | 8080 | Nothing (aggregation only) | none |

Only the BFF is published to the host by default.

## 2. Component diagram

``` mermaid
flowchart LR
    Client([Client])

    subgraph compose[Docker Compose private network]
        BFF["BFF Service (Go)<br/>:8080"]
        PS["Product Service (Java)<br/>:8081"]
        OS["Order Service (Java)<br/>:8082"]

        subgraph mongo[MongoDB - single instance]
            PDB[(products_db)]
            ODB[(orders_db)]
        end
    end

    Client -->|REST| BFF
    BFF -->|REST| PS
    BFF -->|REST| OS
    OS -->|REST: batch lookup| PS
    PS --> PDB
    OS --> ODB
```

Allowed dependencies: BFF → Product, BFF → Order, Order → Product.
Nothing else. No service accesses another service's database.

## 3. Services

### 3.1 Product Service

-   Packages: `controller`, `service`, `repository`, `mapper`,
    `model.entity`, `model.dto`, `exception`, `config`.
-   Collection `products` in `products_db`.
-   CRUD plus `POST /api/products/lookup` (batch by ids; returns
    `products` and `missingIds`; limit 100 ids).
-   Knows nothing about orders or the BFF.

### 3.2 Order Service

-   Same layers as the Product Service plus `client` (`ProductClient`
    interface and a `RestClient`-based implementation with explicit
    connect/read timeouts).
-   Collection `orders` in `orders_db`.
-   On creation: validate request → one batch lookup in Product Service
    → check existence, availability, currency → build item snapshots →
    compute totals → persist.
-   Orders are immutable snapshots; only `CREATED → CANCELLED` is allowed.
-   Readiness depends on its own MongoDB only, not on the Product
    Service.

### 3.3 BFF Service

-   Layout: `cmd/server`, `internal/{config, handler, service,
    client/product, client/order, model, middleware}`.
-   `net/http` with method-aware `ServeMux` patterns.
-   Middleware chain: recover → request ID → access log.
-   `service` layer aggregates and transforms only (no business rules).
-   Graceful shutdown on `SIGTERM`/`SIGINT`.
-   `/health` is liveness only; `/ready` calls the readiness endpoints of
    both downstream services.

## 4. Communication flows

### 4.1 Create order

``` mermaid
sequenceDiagram
    participant C as Client
    participant B as BFF
    participant O as Order Service
    participant P as Product Service
    participant DB as orders_db

    C->>B: POST /api/orders {items}
    B->>O: POST /api/orders (X-Request-Id)
    O->>P: POST /api/products/lookup {ids} (X-Request-Id)
    P-->>O: 200 {products, missingIds}
    O->>O: verify existence, availability, currency<br/>build snapshot, compute total
    O->>DB: insert order
    O-->>B: 201 Created (order)
    B-->>C: 201 Created (client-facing order)
```

Failure mapping:

| Situation | Order Service | BFF |
|---|---|---|
| Invalid request body | `400 VALIDATION_ERROR` | `400` |
| Product missing | `422 PRODUCT_NOT_FOUND` | `422` |
| Product unavailable | `422 PRODUCT_UNAVAILABLE` | `422` |
| Mixed currencies | `422 CURRENCY_MISMATCH` | `422` |
| Product Service down / timeout / 5xx | `503 PRODUCT_SERVICE_UNAVAILABLE` | `503` |
| Order Service down / timeout | -- | `503`/`504` |

### 4.2 Get order with aggregation

``` mermaid
sequenceDiagram
    participant C as Client
    participant B as BFF
    participant O as Order Service
    participant P as Product Service

    C->>B: GET /api/orders/{id}
    B->>O: GET /api/orders/{id}
    O-->>B: 200 order (snapshot)
    B->>P: POST /api/products/lookup {item productIds}
    alt Product Service OK
        P-->>B: 200 {products, missingIds}
        B-->>C: 200 order + items[].currentProduct
    else Product Service failure
        P--xB: timeout / 5xx
        B-->>C: 200 order, currentProduct = null, warnings[]
    end
```

The order is always read from the snapshot. Current product data is
supplementary; its absence degrades the response, it does not fail it. A
product removed since the order was placed appears in `missingIds` and
yields `currentProduct: null`.

### 4.3 Products (pass-through with transformation)

Client → BFF → Product Service. The BFF translates the service contract
into the client-facing contract (for example `price` as `{amount,
currency}`) and maps errors to its own error structure.

### 4.4 Readiness and start order

``` mermaid
flowchart LR
    mongo[(mongo healthy)] --> product[product-service ready]
    mongo --> order[order-service ready]
    product --> order
    product --> bff[bff-service ready]
    order --> bff
```

Compose `depends_on: condition: service_healthy` governs start order only.
At runtime the Order Service tolerates a temporarily unavailable Product
Service (it returns `503` on order creation) and stays ready.

### 4.5 Correlation ID

`X-Request-Id` is generated in the BFF if absent, forwarded to both
downstream services, forwarded by the Order Service to the Product
Service, placed in logs (MDC in Java, `slog` attribute in Go), and
included in error bodies.

## 5. Data model

`products_db.products`

| Field | Type | Notes |
|---|---|---|
| `_id` | ObjectId / string | exposed as `id` |
| `name` | string | required |
| `description` | string | optional |
| `price` | Decimal128 | non-negative; JSON: decimal string |
| `currency` | string | e.g. `PLN` |
| `available` | boolean | |
| `createdAt`, `updatedAt` | date (UTC) | |

`orders_db.orders`

| Field | Type | Notes |
|---|---|---|
| `_id` | ObjectId / string | exposed as `id` |
| `customerId` | string | optional, opaque |
| `status` | string | `CREATED` / `CANCELLED` |
| `items[]` | embedded | `productId`, `productName`, `unitPrice` (Decimal128), `currency`, `quantity`, `lineTotal` |
| `totalAmount` | Decimal128 | |
| `currency` | string | |
| `createdAt`, `updatedAt` | date (UTC) | |

`items[].productId` is a **logical reference** only; there is no
cross-database foreign key, and no cross-database query is allowed.

## 6. Error model

All services return the same JSON structure (`timestamp`, `status`,
`code`, `message`, `path`, `requestId`, `details[]`). The BFF never
forwards downstream error bodies verbatim; it maps status and `code` to
its own response and hides internal details.

## 7. Infrastructure

-   Compose services: `mongo`, `product-service`, `order-service`,
    `bff-service`; private network (for example `backend`); named volume
    `mongo-data`.
-   `infra/mongo/init-databases.js` creates `products_db` and
    `orders_db` (and, optionally, one user per database).
-   Health checks: Mongo (ping), Java services (Actuator readiness), BFF
    (`/health`; self-check flag if the runtime image has no shell/HTTP
    tools).
-   Ports: BFF published; Product/Order only under an optional debug
    profile; Mongo not published.
-   Three multi-stage Dockerfiles, non-root runtime users.

## 8. Testing strategy

| Scope | Java (Product, Order) | Go (BFF) |
|---|---|---|
| Unit | JUnit 5 + Mockito (service layer) | `testing` (aggregation, mapping) |
| Persistence | Testcontainers MongoDB (repository) | n/a |
| API | `@SpringBootTest` + Testcontainers + MockMvc | `httptest` handler tests |
| Downstream client | Order: `ProductClient` against a stub HTTP server | Both clients against `httptest.Server` |
| Failure modes | Product down/timeout/5xx in Order | Degraded aggregation, timeouts, cancellation |

Contract drift between services is the main integration risk; client-side
tests use realistic example payloads taken from the OpenAPI contracts.

## 9. Design decisions

| # | Decision | Rationale |
|---|---|---|
| D1 | Money as decimal string in JSON, `Decimal128`/`BigDecimal` in Java | No floating-point errors; safe in Go |
| D2 | Order stores a product snapshot | Historical correctness; orders readable without Product Service |
| D3 | Prices come only from Product Service | Client cannot tamper with prices |
| D4 | Batch lookup endpoint in Product Service | One call instead of N; no chatty traffic |
| D5 | Separate `pom.xml` per Java service, no shared library | Independent deployability; accepts duplicated DTOs/config |
| D6 | No stock reservation, no sagas/outbox | Keeps demo scope small; limitation documented |
| D7 | Order readiness independent of Product Service | Avoids cascading readiness failures |
| D8 | BFF degrades instead of failing for supplementary data | Better client experience; BFF stays logic-free |
| D9 | One MongoDB instance, two databases | Simple local setup; logical isolation only |

## 10. Risks and limitations

1.  **No atomicity between Order and Product.** Price or availability may
    change right after verification; stock is not reserved. Accepted;
    the snapshot keeps the order self-consistent.
2.  **Temporal coupling Order → Product.** Product Service outage blocks
    order creation (reads still work). Mitigated by timeouts, `503`
    mapping, no automatic retry of `POST`; circuit breaker is a future
    option.
3.  **No idempotency for `POST /api/orders`.** A client or BFF retry may
    create duplicates. `Idempotency-Key` is a future extension.
4.  **Contract drift** between three independently evolving services.
    Mitigation: OpenAPI as the source of truth, client-side contract-like
    tests, an explicit "no shared code" rule.
5.  **Money representation** across Java, MongoDB, and Go must stay
    consistent; no `float64` for money.
6.  **Single MongoDB instance** is a single point of failure and
    isolation is only logical. Mitigation: separate databases (and
    optionally users); no cross-database access.
7.  **Cascading failures and readiness.** BFF `/ready` depends on both
    services; Order readiness must not depend on Product.
    `depends_on` protects start order only.
8.  **Domain logic creeping into the BFF** (totals, status rules).
    Mitigation: rule that the BFF only aggregates and transforms; review
    against the spec.
9.  **Error consistency and information leaks.** One shared error
    structure; the BFF maps and sanitizes downstream errors.
10. **Duplication cost.** Two Maven projects duplicate configuration and
    DTOs. Accepted for service independence.
11. **Testcontainers cost.** Docker required; two Java services start
    MongoDB containers, so tests are slower. Mitigation: singleton
    containers per run, fast unit tests without Docker.
12. **Health checks in minimal images.** Distroless/scratch images lack
    `curl`/`wget`; plan a self-check flag or choose a base image that
    includes a minimal tool.
