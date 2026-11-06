# Project Specification -- Demo Microservices

## 1. Project overview

Build a demo microservices application as a **monorepo** containing three
independent services that communicate over REST:

1.  **Product Service** (Java) -- owns the product domain, its business
    rules, and persistence in `products_db`.
2.  **Order Service** (Java) -- owns the order domain, its business rules,
    and persistence in `orders_db`. Calls the Product Service over REST.
3.  **BFF Service** (Go) -- the client-facing Backend For Frontend. Calls
    the Product Service and the Order Service over REST, aggregates and
    transforms their responses. Has no database and no domain logic.

A single MongoDB instance hosts two separate logical databases:
`products_db` and `orders_db`. All components must be runnable locally
with Docker Compose.

Keep the implementation focused on demonstrating service boundaries,
REST communication, maintainable code, testing, and containerization. Do
not add infrastructure that is not required by this specification.

## 2. Technology stack

### Java services (Product Service and Order Service)

Both services use the same stack. Each is an independent Maven project
(no parent POM, no multi-module build, no shared library).

-   Java 21
-   Spring Boot 3.x
-   Maven
-   Spring Web
-   Spring Data MongoDB
-   MapStruct
-   Jakarta Bean Validation
-   Spring Boot Actuator
-   OpenAPI documentation (springdoc-openapi)
-   JUnit 5
-   Mockito
-   Spring Boot Test
-   Testcontainers (MongoDB module and JUnit Jupiter integration)

Order Service additionally uses a Spring HTTP client (`RestClient`) to
call the Product Service.

### BFF Service (Go)

-   Go (use a current stable release compatible with the chosen
    dependencies; Go 1.22+ is required for method-aware `ServeMux`
    patterns)
-   Standard library `net/http` for HTTP routing, server, and client
-   Standard `testing` and `net/http/httptest` packages

### Infrastructure

-   Docker
-   Docker Compose
-   MongoDB (one instance, two logical databases)
-   Multi-stage Docker builds

Pin compatible dependency and container image versions. Avoid floating
`latest` tags for application dependencies.

## 3. Service responsibilities and boundaries

### 3.1 Product Service

The Product Service is the **single source of truth** for product data.

Responsibilities:

-   Create, read, update, and delete products.
-   Provide a batch lookup of products by id for other services.
-   Apply product business rules and request validation.
-   Persist product data in its own database `products_db`.
-   Expose a documented REST API.
-   Return consistent HTTP status codes and error responses.

The Product Service must not depend on the Order Service or the BFF
Service, and must not access `orders_db`.

### 3.2 Order Service

The Order Service is the **single source of truth** for order data.

Responsibilities:

-   Create and read orders, list orders, and cancel orders.
-   On order creation, verify products, availability, and prices by
    calling the Product Service over REST. Prices are always taken from
    the Product Service; clients never supply prices.
-   Store a **snapshot** of the relevant product data (id, name, unit
    price, currency) in the order, together with the quantity. Historical
    orders must not depend on later changes to products and must not
    require calls to the Product Service to be read.
-   Compute the order total from the snapshot.
-   Persist order data in its own database `orders_db`.
-   Expose a documented REST API.
-   Map Product Service failures to explicit, stable error responses.

The Order Service may depend only on the **REST contract** of the Product
Service. It must not read `products_db`, share code with the Product
Service, or know about the BFF Service.

### 3.3 BFF Service (Go)

The BFF Service is the only service exposed to clients.

Responsibilities:

-   Expose the client-facing HTTP API.
-   Route requests to the Product Service and the Order Service.
-   Aggregate responses from both services where it benefits the client
    (for example, an order with the current state of its products).
-   Transform downstream responses into client-facing responses.
-   Handle downstream connection errors, timeouts, and non-success HTTP
    responses; degrade gracefully where a partial response is useful.
-   Propagate request cancellation using `context.Context`.
-   Generate or propagate a correlation/request ID.
-   Expose liveness (`/health`) and readiness (`/ready`) endpoints.

The BFF Service must **not**:

-   connect to MongoDB or have any database of its own;
-   implement product or order domain logic (pricing rules, availability
    rules, order totals, status transitions, and similar);
-   duplicate product or order persistence or caches that act as a source
    of truth.

The BFF may only validate request *shape* (valid JSON, syntactically
required fields). Business validation belongs to the owning service.

### 3.4 Communication

-   Use HTTP REST with JSON payloads.
-   Allowed call directions only:
    -   BFF → Product Service
    -   BFF → Order Service
    -   Order Service → Product Service
-   Forbidden: Product Service → anything; Order Service → BFF;
    any service → another service's database.
-   Services call each other using Docker Compose service names and
    internal ports, for example `http://product-service:8081` and
    `http://order-service:8082`.
-   Configure base URLs through environment variables
    (`PRODUCT_SERVICE_URL`, `ORDER_SERVICE_URL`). Do not hardcode service
    URLs.
-   Set explicit connection and read/request timeouts in every HTTP
    client (BFF and Order Service).
-   Do not retry non-idempotent requests automatically.
-   Do not introduce Kafka, RabbitMQ, gRPC, or another communication
    mechanism. Do not introduce distributed transactions, sagas, or an
    outbox; consistency between services is eventual and limited as
    described in section 5.4.

## 4. Domain

### 4.1 Product

-   `id` -- generated identifier
-   `name` -- required product name
-   `description` -- optional description
-   `price` -- non-negative decimal value
-   `currency` -- currency code, for example `PLN`
-   `available` -- availability flag
-   `createdAt` -- creation timestamp
-   `updatedAt` -- last update timestamp

### 4.2 Order

-   `id` -- generated identifier
-   `customerId` -- optional customer reference (opaque string; there is
    no customer service)
-   `status` -- `CREATED` or `CANCELLED`
-   `items` -- non-empty list of order items
-   `totalAmount` -- sum of `unitPrice × quantity` over items
-   `currency` -- order currency (all items must share one currency)
-   `createdAt`, `updatedAt` -- timestamps

Order item (product snapshot):

-   `productId`
-   `productName` -- snapshot at order time
-   `unitPrice` -- snapshot at order time
-   `currency` -- snapshot at order time
-   `quantity` -- positive integer
-   `lineTotal` -- `unitPrice × quantity`

Allowed status transition: `CREATED → CANCELLED`. Cancelling an already
cancelled order is a conflict. Orders are otherwise immutable: there is
no update or delete operation.

### 4.3 Representation rules

-   Money is represented in JSON as a **decimal string** (for example
    `"349.99"`) in all services, to avoid binary floating-point errors in
    Java and Go. Use `BigDecimal` in Java (stored as `Decimal128` in
    MongoDB) and keep the value as a string (or a decimal type) in Go;
    never use `float64` for money.
-   Timestamps are ISO-8601 in UTC (for example `2026-01-15T10:30:00Z`).
-   Identifiers are opaque strings.
-   The representation must be consistent across all three services and
    covered by tests.

Example product (Product Service):

``` json
{
  "id": "673abc123",
  "name": "Mechanical Keyboard",
  "description": "Compact mechanical keyboard",
  "price": "349.99",
  "currency": "PLN",
  "available": true,
  "createdAt": "2026-01-15T10:30:00Z",
  "updatedAt": "2026-01-15T10:30:00Z"
}
```

Example order (Order Service):

``` json
{
  "id": "674def456",
  "customerId": "customer-42",
  "status": "CREATED",
  "items": [
    {
      "productId": "673abc123",
      "productName": "Mechanical Keyboard",
      "unitPrice": "349.99",
      "currency": "PLN",
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

The examples are illustrative; the implementation must keep the
representation consistent and test it.

## 5. REST API

### 5.1 Product Service (`/api/products`, port 8081)

  Method     Path                      Description
  ---------- ------------------------- ------------------------------------
  `GET`      `/api/products`           List products (paginated)
  `GET`      `/api/products/{id}`      Get a product
  `POST`     `/api/products`           Create a product
  `PUT`      `/api/products/{id}`      Update a product
  `DELETE`   `/api/products/{id}`      Delete a product
  `POST`     `/api/products/lookup`    Batch lookup by ids

Batch lookup:

-   Request: `{ "ids": ["...", "..."] }` (at most 100 ids).
-   Response `200`: `{ "products": [ ... ], "missingIds": [ ... ] }`.
    Missing products are **not** an error at this level; the caller
    decides what missing products mean.

List response: `{ "content": [...], "page": 0, "size": 20,
"totalElements": 1, "totalPages": 1 }` with query parameters `page`,
`size`, and `sort`.

### 5.2 Order Service (`/api/orders`, port 8082)

  Method     Path                        Description
  ---------- --------------------------- --------------------------------
  `POST`     `/api/orders`               Create an order
  `GET`      `/api/orders/{id}`          Get an order
  `GET`      `/api/orders`               List orders (paginated, `status` filter)
  `POST`     `/api/orders/{id}/cancel`   Cancel an order

Create request: `{ "customerId": "optional", "items": [ { "productId":
"...", "quantity": 2 } ] }`. The request contains **no prices**.

Order creation rules (enforced by the Order Service):

-   `items` must be non-empty; `quantity` must be positive; duplicate
    `productId` entries are rejected (or merged -- the chosen behaviour
    must be documented and tested).
-   All products must exist (otherwise `422 PRODUCT_NOT_FOUND`).
-   All products must be `available` (otherwise `422 PRODUCT_UNAVAILABLE`).
-   All products must share one currency (otherwise `422
    CURRENCY_MISMATCH`).
-   If the Product Service is unreachable, times out, or returns 5xx,
    respond `503 PRODUCT_SERVICE_UNAVAILABLE` and create nothing.
-   On success, store the snapshot and total and return `201 Created`
    with a `Location` header.

### 5.3 BFF Service (client-facing API, port 8080)

  Method     Path                        Description
  ---------- --------------------------- --------------------------------------
  `GET`      `/api/products`             List products
  `GET`      `/api/products/{id}`        Get a product
  `POST`     `/api/products`             Create a product
  `PUT`      `/api/products/{id}`        Update a product
  `DELETE`   `/api/products/{id}`        Delete a product
  `POST`     `/api/orders`               Create an order
  `GET`      `/api/orders`               List orders
  `GET`      `/api/orders/{id}`          Get an order, aggregated with current product data
  `POST`     `/api/orders/{id}/cancel`   Cancel an order
  `GET`      `/health`                   Liveness
  `GET`      `/ready`                    Readiness (checks both downstream services)

The BFF contract is a client-facing contract and **may differ** from the
service contracts (for example, price as `{ "amount": "349.99",
"currency": "PLN" }`). The BFF contract is documented separately from the
Java service contracts. `GET /api/orders/{id}` returns the order snapshot
plus, per item, a `currentProduct` object (current name, price,
availability) fetched from the Product Service. If the Product Service
call fails, the BFF returns the order with `currentProduct: null` and a
`warnings` entry instead of failing the whole response. If the Order
Service call fails, the BFF returns an error.

### 5.4 Consistency model

-   There are no distributed transactions. Verification of price and
    availability during order creation is a point-in-time check; the
    product may change right after it. The snapshot makes the order
    consistent with what was verified.
-   Stock is **not reserved**. `available` is a flag, not an inventory
    counter. Inventory management is out of scope.
-   `POST /api/orders` is not idempotent. An `Idempotency-Key` header is a
    possible future extension and is out of scope.

### 5.5 Health, readiness, and documentation

-   BFF: `GET /health` (liveness, does not call downstream) and
    `GET /ready` (calls the readiness endpoints of Product Service and
    Order Service).
-   Product Service and Order Service: Spring Boot Actuator with liveness
    and readiness probes (`/actuator/health/liveness`,
    `/actuator/health/readiness`).
-   The Order Service readiness must **not** depend on the Product
    Service, to avoid cascading readiness failures. Its readiness depends
    on its own MongoDB connectivity only.
-   Product Service and Order Service document their APIs with OpenAPI
    (`/v3/api-docs`). The BFF API is documented in `docs/` (an OpenAPI
    file or an equivalent description).

### 5.6 General API requirements

-   Use correct HTTP status codes, including `201 Created`, `200 OK`,
    `204 No Content`, `400 Bad Request`, `404 Not Found`,
    `409 Conflict`, `422 Unprocessable Entity`, `502/503/504` for
    downstream failures.
-   Validate request bodies.
-   Do not expose MongoDB persistence entities directly from
    controllers.
-   All three services return JSON errors with a stable structure:

``` json
{
  "timestamp": "2026-01-15T10:30:00Z",
  "status": 422,
  "code": "PRODUCT_UNAVAILABLE",
  "message": "Product 673abc123 is not available",
  "path": "/api/orders",
  "requestId": "b1c2d3",
  "details": [ { "field": "items[0].productId", "message": "..." } ]
}
```

-   Machine-readable error codes include `VALIDATION_ERROR`,
    `NOT_FOUND`, `CONFLICT`, `PRODUCT_NOT_FOUND`, `PRODUCT_UNAVAILABLE`,
    `CURRENCY_MISMATCH`, `PRODUCT_SERVICE_UNAVAILABLE`,
    `SERVICE_UNAVAILABLE`, and `INTERNAL_ERROR`.
-   Internal details (stack traces, downstream hostnames, exception
    messages) must not leak to clients. The BFF maps downstream errors to
    its own error structure.

## 6. Java service architecture

Both Java services use the same simple layered architecture, each in its
own base package (for example `com.example.product` and
`com.example.order`):

-   `controller` -- REST endpoints
-   `service` -- business logic
-   `repository` -- Spring Data MongoDB repositories
-   `mapper` -- MapStruct mappers
-   `model.entity` -- MongoDB documents
-   `model.dto` -- request and response DTOs
-   `exception` -- domain exceptions and centralized exception handling
-   `config` -- application configuration

The Order Service additionally has:

-   `client` -- the Product Service REST client (`ProductClient`),
    isolated behind an interface so that the service layer can be unit
    tested with Mockito. The client translates downstream responses and
    failures into domain exceptions and owns the Product Service DTOs
    (the Order Service keeps its own copy of the contract; no shared
    library).

Requirements:

-   Use constructor-based dependency injection.
-   Use MapStruct for mapping between persistence models and DTOs.
-   Keep controllers thin; business logic belongs in services.
-   Use Jakarta Bean Validation for input validation.
-   Implement centralized exception handling with
    `@RestControllerAdvice`.
-   Use clear package boundaries and avoid unnecessary abstractions.
-   Configure MongoDB using environment variables; each service connects
    only to its own database (`products_db` or `orders_db`).
-   Use UTC timestamps and a consistent serialization format.
-   Propagate the `X-Request-Id` header into logging (MDC), and from the
    Order Service into outgoing Product Service calls.
-   Add pagination to list endpoints; document the chosen contract.

## 7. Java testing -- Testcontainers is required

Both Java services must be tested as described below. Use
**Testcontainers with a real MongoDB container** for repository and
integration tests. Do not use an in-memory substitute for MongoDB
integration tests.

Required test categories (for each Java service):

### 7.1 Unit tests

-   Test service-layer business logic with JUnit 5 and Mockito.
-   Cover successful operations and relevant failure cases.
-   Order Service: mock `ProductClient` and cover missing product,
    unavailable product, currency mismatch, downstream failure, total
    calculation, snapshot contents, and status transitions.
-   Keep unit tests independent of Docker and external infrastructure.

### 7.2 Repository integration tests

-   Use Testcontainers MongoDB.
-   Start a MongoDB container for the test lifecycle.
-   Configure Spring Data MongoDB to connect to the container.
-   Verify repository persistence, retrieval, update, and delete
    behavior against real MongoDB (including decimal money round-trip).
-   Ensure tests clean up or isolate test data so they are repeatable.

### 7.3 Spring application integration tests

-   Use `@SpringBootTest` and Testcontainers to test the application
    with a real MongoDB instance.
-   Test REST endpoints, validation, serialization, and error handling.
-   Use MockMvc or an appropriate HTTP test client.
-   Product Service: at least successful product creation and retrieval,
    validation failure, missing product, deletion, and batch lookup
    including missing ids.
-   Order Service: at least successful order creation with snapshot,
    validation failure, missing order, cancellation, invalid cancellation
    (conflict), and mapped Product Service errors. The Product Service is
    replaced by a stub HTTP server (for example MockWebServer or
    WireMock) or a mocked bean; Order Service tests must not require a
    running Product Service.

### 7.4 Product client tests (Order Service)

-   Test `ProductClient` against a stub HTTP server: successful lookup,
    missing ids, 4xx, 5xx, timeout, and connection failure.
-   Verify the `X-Request-Id` header is forwarded.

### 7.5 Testcontainers implementation guidance

-   Add compatible Testcontainers dependencies to each `pom.xml`,
    including the MongoDB module and JUnit Jupiter integration.
-   Prefer Spring Boot's supported Testcontainers integration where
    compatible with the selected Spring Boot version; otherwise use
    `@Testcontainers`, `@Container`, and `@DynamicPropertySource` to
    provide the MongoDB connection URI.
-   Do not assume a fixed host port. Use the connection URI supplied by
    the container.
-   Make the test setup easy to run with Maven.
-   Document that Docker must be available when running Testcontainers
    tests.
-   Keep fast unit tests runnable without Docker; integration tests may
    require Docker.
-   Consider a shared (singleton) container per service test run to keep
    test time reasonable.
-   Verify the dependency versions are compatible with the selected
    Spring Boot and Java versions.

## 8. BFF Service (Go) architecture

Use a small, idiomatic Go project layout:

-   `cmd/server` -- application entry point
-   `internal/handler` -- HTTP handlers
-   `internal/service` -- aggregation and transformation logic (no
    domain rules)
-   `internal/client/product` -- Product Service HTTP client
-   `internal/client/order` -- Order Service HTTP client
-   `internal/model` -- client-facing and downstream request/response
    models
-   `internal/middleware` -- request ID, logging, panic recovery
-   `internal/config` -- environment-based configuration

Requirements:

-   Use `net/http` (including method-aware `ServeMux` patterns) unless
    there is a clear reason to introduce a router dependency.
-   Keep handlers, downstream clients, and configuration separate.
-   Use `context.Context` for request cancellation and pass it to every
    downstream call.
-   Configure explicit HTTP client timeouts per downstream service.
-   Close response bodies.
-   Handle non-2xx responses from downstream services deliberately; do
    not blindly return successful responses.
-   Run independent downstream calls concurrently when aggregating, and
    degrade gracefully when only non-essential data is missing.
-   Avoid leaking internal error details to API clients.
-   Use structured logging with the standard library (`log/slog`) or a
    small well-supported logging package.
-   Implement graceful shutdown on process termination.
-   Read configuration from environment variables.
-   Support a self-check mode (for example a `-healthcheck` flag) if the
    runtime image has no shell or HTTP tools.

## 9. Go testing

Use the standard `testing` package and `net/http/httptest`.

Required tests:

-   Handler tests for successful requests (products and orders).
-   Handler tests for invalid input.
-   Handler tests for downstream errors (404, 4xx, 5xx, unreachable).
-   Client tests for both clients using `httptest.Server`.
-   Aggregation tests: full success, Product Service failure during
    order aggregation (degraded response), Order Service failure.
-   Timeout, context cancellation, and connection failure handling
    tests where practical.
-   Readiness tests (both downstream healthy, one unhealthy).
-   Request ID tests (generated when missing, propagated when present).
-   Ensure tests do not require running Java services or MongoDB.

## 10. Docker and Docker Compose

Create:

-   `product-service/Dockerfile`
-   `order-service/Dockerfile`
-   `bff-service/Dockerfile`
-   root `docker-compose.yml`
-   `.env.example`
-   `infra/mongo/` initialization script that creates `products_db` and
    `orders_db`

Requirements:

-   Use multi-stage builds.
-   Run application containers as non-root users where practical.
-   Keep runtime images minimal.
-   Do not bake secrets into images.
-   Configure services through environment variables.
-   Run **one** MongoDB instance with a named volume for data
    persistence. Each Java service connects to its own database only.
-   Put services on a clearly named private Compose network.
-   Publish only the ports needed for local development: the BFF port by
    default. Product Service and Order Service ports may be published
    only under an optional Compose profile for debugging. MongoDB is not
    published by default.
-   Configure MongoDB health checks.
-   Configure Product Service and Order Service health checks using
    Actuator readiness.
-   Configure BFF health checks.
-   Use Compose health-based dependencies:
    -   Product Service waits for MongoDB readiness;
    -   Order Service waits for MongoDB and Product Service readiness;
    -   BFF waits for Product Service and Order Service readiness.
-   Ensure services use internal Compose hostnames, not `localhost`.
-   Make `docker compose up --build` the documented startup command.
-   Make `docker compose down` stop the stack without deleting persisted
    data.
-   Document `docker compose down -v` as the command that also removes
    the MongoDB volume.

## 11. Configuration

Provide `.env.example` with documented, non-secret development defaults,
such as:

-   MongoDB connection settings
-   `PRODUCT_DB_NAME` (`products_db`) and `ORDER_DB_NAME` (`orders_db`)
-   Product Service, Order Service, and BFF ports
-   `PRODUCT_SERVICE_URL` and `ORDER_SERVICE_URL` (as seen from the
    containers that use them)
-   HTTP client timeouts for BFF → services and Order → Product Service

Do not commit real credentials or secrets. Ensure Compose configuration
works with the example environment file and clearly document how to
create a local `.env`.

## 12. Observability and reliability

Implement the following without overengineering:

-   Structured logs in all services.
-   `X-Request-Id` generated in the BFF when missing and propagated
    BFF → Product Service, BFF → Order Service, and Order Service →
    Product Service; included in logs and error responses.
-   Request-scoped context and cancellation in Go.
-   Explicit downstream HTTP timeouts in the BFF and in the Order
    Service.
-   Graceful shutdown in Go and Spring Boot.
-   Health and readiness endpoints in all services.
-   Clear and consistent error mapping between services.

A circuit breaker or an `Idempotency-Key` mechanism may be added only as
an optional extension after the basic implementation and tests work.

## 13. Documentation

Create a root `README.md` containing:

-   Project purpose and architecture.
-   Service responsibilities and communication flows.
-   Technology stack.
-   Repository structure.
-   Prerequisites, including Docker and Docker Compose.
-   How to configure local environment variables.
-   How to run the complete stack.
-   How to run Java unit tests (both services).
-   How to run Java Testcontainers integration tests (both services).
-   How to run Go tests.
-   API endpoint descriptions and example `curl` commands for products
    and orders (through the BFF).
-   Health check URLs.
-   How to stop the stack and how to remove persisted data.

Maintain `docs/architecture.md` with the architecture explanation,
Mermaid component and sequence diagrams, data model, error model,
decisions, and risks.

## 14. Repository structure

``` text
spring-go-microservices-demo/
├── docker-compose.yml
├── .env.example
├── .gitignore
├── README.md
├── docs/
│   ├── PROJECT_SPEC.md
│   ├── architecture.md
│   └── api/                      (optional: OpenAPI contracts)
├── infra/
│   └── mongo/
│       └── init-databases.js     (creates products_db and orders_db)
├── product-service/
│   ├── Dockerfile
│   ├── pom.xml
│   └── src/
│       ├── main/
│       │   ├── java/
│       │   └── resources/
│       └── test/
│           └── java/
├── order-service/
│   ├── Dockerfile
│   ├── pom.xml
│   └── src/
│       ├── main/
│       │   ├── java/
│       │   └── resources/
│       └── test/
│           └── java/
└── bff-service/
    ├── Dockerfile
    ├── go.mod
    ├── cmd/
    │   └── server/
    └── internal/
        ├── config/
        ├── handler/
        ├── client/
        │   ├── product/
        │   └── order/
        ├── middleware/
        ├── model/
        └── service/
```

Adapt package and directory names if needed, but preserve the service
boundaries. Do not introduce a shared code module between services.

## 15. Development workflow for Claude Code

Work incrementally. Do not generate the entire application in one pass.

Before changing files:

1.  Read this specification and `docs/architecture.md`.
2.  Inspect the current repository and existing files.
3.  Summarize the proposed architecture and implementation plan.
4.  Identify ambiguities, risks, and dependency compatibility concerns.
5.  Ask for confirmation before making large structural changes.

Implement in these stages:

1.  Repository skeleton, `.gitignore`, `.env.example`, README outline, and
    Compose with MongoDB plus the initialization script for both
    databases.
2.  Product Service domain, DTOs, MapStruct, repository, service,
    controller, validation, exception handling, pagination, and OpenAPI.
3.  Product Service unit tests and Testcontainers-based MongoDB
    integration tests, including the batch lookup endpoint.
4.  Order Service domain, snapshot model, DTOs, MapStruct, repository,
    service (with a mocked `ProductClient`), controller, and exception
    handling.
5.  Order Service `ProductClient`, verification logic, and tests (unit,
    Testcontainers, stubbed Product Service).
6.  BFF HTTP server, configuration, middleware, clients, product
    handlers, and graceful shutdown, with tests.
7.  BFF order handlers, aggregation with degraded responses, error
    mapping, and readiness, with tests.
8.  Three Dockerfiles and full Docker Compose health/readiness
    configuration.
9.  End-to-end verification (happy path, unavailable product, Product
    Service down), documentation, and cleanup.

At the end of each stage:

-   Run the relevant formatter, build, and tests.
-   Fix failures before proceeding.
-   Summarize files changed and commands executed.
-   Do not claim tests passed unless they were actually run.
-   Keep the implementation consistent with this specification.

## 16. Out of scope

Do not add these unless explicitly requested:

-   Kubernetes
-   Kafka or RabbitMQ (or any message broker)
-   Distributed transactions, sagas, or the outbox pattern
-   Stock reservation or inventory management
-   Redis or other caches
-   Authentication/authorization
-   Frontend application
-   Distributed tracing infrastructure
-   Additional database instances (one MongoDB instance with exactly two
    logical databases is the design)
-   Additional microservices beyond the three described here
-   Shared libraries or multi-module Maven builds between services

The goal is a small, production-minded demo of Java 21, Spring Boot,
MongoDB, MapStruct, Go, REST communication between three independent
services, automated tests with Testcontainers, and Docker Compose.
