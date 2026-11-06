# Project Specification -- Demo Microservices

## 1. Project overview

Build a demo microservices application as a **monorepo** containing two
independent services that communicate over REST:

1.  **Java Service** -- owns the product domain, business rules, and
    persistence.
2.  **Go Service** -- exposes its own HTTP API and communicates with the
    Java Service over HTTP/REST.

MongoDB is the persistence layer for the Java Service. All components
must be runnable locally with Docker Compose.

Keep the implementation focused on demonstrating service boundaries,
REST communication, maintainable code, testing, and containerization. Do
not add infrastructure that is not required by this specification.

## 2. Technology stack

### Java Service

-   Java 21
-   Spring Boot 3.x
-   Maven
-   Spring Web
-   Spring Data MongoDB
-   MapStruct
-   Jakarta Bean Validation
-   Spring Boot Actuator
-   JUnit 5
-   Mockito
-   Spring Boot Test
-   Testcontainers (MongoDB module and JUnit Jupiter integration)

### Go Service

-   Go (use a current stable release compatible with the chosen
    dependencies)
-   Prefer the standard library `net/http` for HTTP routing and server
    functionality
-   Standard `testing` and `net/http/httptest` packages

### Infrastructure

-   Docker
-   Docker Compose
-   MongoDB
-   Multi-stage Docker builds

Pin compatible dependency and container image versions. Avoid floating
`latest` tags for application dependencies.

## 3. Service responsibilities and boundaries

### 3.1 Java Service -- Product Service

The Java Service is the **single source of truth** for product data.

Responsibilities:

-   Create, read, update, and delete products.
-   Apply product business rules and request validation.
-   Persist product data in MongoDB.
-   Expose a documented REST API.
-   Return consistent HTTP status codes and error responses.

The Java Service must not depend on the Go Service.

### 3.2 Go Service -- API Gateway / BFF

The Go Service exposes an HTTP API to clients and calls the Java Service
through its REST API.

Responsibilities:

-   Expose product endpoints.
-   Translate incoming requests into calls to the Java Service.
-   Translate downstream responses into client-facing responses.
-   Handle downstream connection errors, timeouts, and non-success HTTP
    responses.
-   Propagate request cancellation using `context.Context`.
-   Generate or propagate a correlation/request ID.
-   Expose a health endpoint.

The Go Service must **not** connect directly to MongoDB or duplicate
product persistence.

### 3.3 Communication

-   Use HTTP REST with JSON payloads.
-   The Go Service calls the Java Service using the Docker Compose
    service name and internal port, for example
    `http://java-service:8080`.
-   Configure the Java Service base URL through an environment variable.
-   Set explicit connection and request timeouts in the Go HTTP client.
-   Do not hardcode service URLs.
-   Do not introduce Kafka, RabbitMQ, gRPC, or another communication
    mechanism.

## 4. Product domain

Use the following product fields:

-   `id` -- generated identifier
-   `name` -- required product name
-   `description` -- optional description
-   `price` -- non-negative decimal value
-   `currency` -- currency code, for example `PLN`
-   `available` -- availability flag
-   `createdAt` -- creation timestamp
-   `updatedAt` -- last update timestamp

Use appropriate Java types for monetary values; do not use binary
floating-point types for money. Define and document the JSON
representation consistently across Java and Go.

Example response:

``` json
{
  "id": "673abc123",
  "name": "Mechanical Keyboard",
  "description": "Compact mechanical keyboard",
  "price": 349.99,
  "currency": "PLN",
  "available": true,
  "createdAt": "2026-01-15T10:30:00Z",
  "updatedAt": "2026-01-15T10:30:00Z"
}
```

The example is illustrative. The implementation must choose a consistent
representation and test it.

## 5. REST API

Implement the following operations in the Java Service and expose
corresponding operations through the Go Service:

  Method     Path                   Description
  ---------- ---------------------- ------------------
  `GET`      `/api/products`        List products
  `GET`      `/api/products/{id}`   Get a product
  `POST`     `/api/products`        Create a product
  `PUT`      `/api/products/{id}`   Update a product
  `DELETE`   `/api/products/{id}`   Delete a product

Also expose:

-   `GET /health` on the Go Service.
-   Appropriate health/readiness endpoints on the Java Service through
    Spring Boot Actuator.

Requirements:

-   Use correct HTTP status codes, including `201 Created`, `200 OK`,
    `204 No Content`, `400 Bad Request`, and `404 Not Found` where
    appropriate.
-   Validate request bodies.
-   Return JSON error responses with a stable structure.
-   Do not expose MongoDB persistence entities directly from
    controllers.
-   Document API contracts and error responses using OpenAPI.

## 6. Java Service architecture

Use a simple layered architecture:

-   `controller` -- REST endpoints
-   `service` -- business logic
-   `repository` -- Spring Data MongoDB repositories
-   `mapper` -- MapStruct mappers
-   `model.entity` -- MongoDB documents
-   `model.dto` -- request and response DTOs
-   `exception` -- domain exceptions and centralized exception handling
-   `config` -- application configuration

Requirements:

-   Use constructor-based dependency injection.
-   Use MapStruct for mapping between persistence models and DTOs.
-   Keep controllers thin; business logic belongs in services.
-   Use Jakarta Bean Validation for input validation.
-   Implement centralized exception handling with
    `@RestControllerAdvice`.
-   Use clear package boundaries and avoid unnecessary abstractions.
-   Configure MongoDB using environment variables.
-   Use UTC timestamps and a consistent serialization format.
-   Add pagination to the product list endpoint if it can be implemented
    without making the demo unnecessarily complex; document the chosen
    API contract.

## 7. Java testing -- Testcontainers is required

Use **Testcontainers with a real MongoDB container** for repository and
integration tests. Do not use an in-memory substitute for MongoDB
integration tests.

Required test categories:

### 7.1 Unit tests

-   Test service-layer business logic with JUnit 5 and Mockito.
-   Cover successful operations and relevant failure cases.
-   Keep unit tests independent of Docker and external infrastructure.

### 7.2 Repository integration tests

-   Use Testcontainers MongoDB.
-   Start a MongoDB container for the test lifecycle.
-   Configure Spring Data MongoDB to connect to the container.
-   Verify repository persistence, retrieval, update, and delete
    behavior against real MongoDB.
-   Ensure tests clean up or isolate test data so they are repeatable.

### 7.3 Spring application integration tests

-   Use `@SpringBootTest` and Testcontainers to test the application
    with a real MongoDB instance.
-   Test REST endpoints, validation, serialization, and error handling.
-   Use MockMvc or an appropriate HTTP test client.
-   Include tests for at least successful product creation and
    retrieval, validation failure, missing product, and deletion.

### 7.4 Testcontainers implementation guidance

-   Add compatible Testcontainers dependencies to `pom.xml`, including
    the MongoDB module and JUnit Jupiter integration.
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
-   Verify the dependency versions are compatible with the selected
    Spring Boot and Java versions.

## 8. Go Service architecture

Use a small, idiomatic Go project layout:

-   `cmd/server` -- application entry point
-   `internal/handler` -- HTTP handlers
-   `internal/service` -- orchestration/business-facing logic
-   `internal/client` -- dedicated Java Service HTTP client
-   `internal/model` -- request/response models
-   `internal/config` -- environment-based configuration

Requirements:

-   Use `net/http` unless there is a clear reason to introduce a router
    dependency.
-   Keep handlers, downstream client, and configuration separate.
-   Use `context.Context` for request cancellation.
-   Configure explicit HTTP client timeouts.
-   Close response bodies.
-   Handle non-2xx responses from Java deliberately; do not blindly
    return successful responses.
-   Avoid leaking internal exception details to API clients.
-   Use structured logging with the standard library or a small
    well-supported logging package.
-   Implement graceful shutdown on process termination.
-   Read configuration from environment variables.

## 9. Go testing

Use the standard `testing` package and `net/http/httptest`.

Required tests:

-   Handler tests for successful requests.
-   Handler tests for invalid input.
-   Handler tests for downstream Java errors.
-   Java client tests using `httptest.Server`.
-   Timeout and connection failure handling tests where practical.
-   Ensure tests do not require a running Java Service or MongoDB.

## 10. Docker and Docker Compose

Create:

-   `java-service/Dockerfile`
-   `go-service/Dockerfile`
-   root `docker-compose.yml`
-   `.env.example`

Requirements:

-   Use multi-stage builds.
-   Run application containers as non-root users where practical.
-   Keep runtime images minimal.
-   Do not bake secrets into images.
-   Configure services through environment variables.
-   Use a named volume for MongoDB data persistence.
-   Put services on the default or a clearly named private Compose
    network.
-   Publish only the ports needed for local development.
-   Configure MongoDB health checks.
-   Configure Java readiness/health checks using Actuator.
-   Configure Go health checks.
-   Use Compose health-based dependencies so Java waits for MongoDB
    readiness and Go waits for Java readiness.
-   Ensure the Go Service uses the internal Java service hostname, not
    `localhost`.
-   Make `docker compose up --build` the documented startup command.
-   Make `docker compose down` stop the stack without deleting persisted
    data.
-   Document `docker compose down -v` as the command that also removes
    the MongoDB volume.

## 11. Configuration

Provide `.env.example` with documented, non-secret development defaults,
such as:

-   MongoDB database name
-   MongoDB connection settings
-   Java Service port
-   Go Service port
-   Java Service base URL as seen from the Go container
-   Go HTTP client timeout

Do not commit real credentials or secrets. Ensure Compose configuration
works with the example environment file and clearly document how to
create a local `.env`.

## 12. Observability and reliability

Implement the following without overengineering:

-   Structured logs.
-   Correlation/request ID propagated from Go to Java.
-   Request-scoped context and cancellation in Go.
-   Explicit downstream HTTP timeouts.
-   Graceful shutdown in Go and Spring Boot.
-   Health and readiness endpoints.
-   Clear error mapping between services.

A circuit breaker may be added only as an optional extension after the
basic implementation and tests work.

## 13. Documentation

Create a root `README.md` containing:

-   Project purpose and architecture.
-   Service responsibilities and communication flow.
-   Technology stack.
-   Repository structure.
-   Prerequisites, including Docker and Docker Compose.
-   How to configure local environment variables.
-   How to run the complete stack.
-   How to run Java unit tests.
-   How to run Java Testcontainers integration tests.
-   How to run Go tests.
-   API endpoint descriptions and example `curl` commands.
-   Health check URLs.
-   How to stop the stack and how to remove persisted data.

Create `docs/architecture.md` with a concise architecture explanation
and a Mermaid component/sequence diagram if useful.

## 14. Suggested repository structure

``` text
demo-microservices/
├── docker-compose.yml
├── .env.example
├── .gitignore
├── README.md
├── PROJECT_SPEC.md
├── docs/
│   └── architecture.md
├── java-service/
│   ├── Dockerfile
│   ├── pom.xml
│   └── src/
│       ├── main/
│       │   ├── java/
│       │   └── resources/
│       └── test/
│           └── java/
└── go-service/
    ├── Dockerfile
    ├── go.mod
    ├── cmd/
    │   └── server/
    └── internal/
        ├── config/
        ├── handler/
        ├── client/
        ├── model/
        └── service/
```

Adapt package and directory names if needed, but preserve the service
boundaries.

## 15. Development workflow for Claude Code

Work incrementally. Do not generate the entire application in one pass.

Before changing files:

1.  Read this specification.
2.  Inspect the current repository and existing files.
3.  Summarize the proposed architecture and implementation plan.
4.  Identify ambiguities, risks, and dependency compatibility concerns.
5.  Ask for confirmation before making large structural changes.

Implement in these stages:

1.  Repository skeleton, documentation outline, and Compose design.
2.  Java Service domain, DTOs, MapStruct, repository, service,
    controller, validation, and exception handling.
3.  Java unit tests and Testcontainers-based MongoDB integration tests.
4.  Go Service HTTP server, handlers, Java client, configuration, and
    error mapping.
5.  Go unit and HTTP client tests.
6.  Dockerfiles and Docker Compose health/readiness configuration.
7.  End-to-end verification, documentation, and cleanup.

At the end of each stage:

-   Run the relevant formatter, build, and tests.
-   Fix failures before proceeding.
-   Summarize files changed and commands executed.
-   Do not claim tests passed unless they were actually run.
-   Keep the implementation consistent with this specification.

## 16. Out of scope

Do not add these unless explicitly requested:

-   Kubernetes
-   Kafka or RabbitMQ
-   Redis
-   Authentication/authorization
-   Frontend application
-   Distributed tracing infrastructure
-   Multiple databases
-   Additional microservices

The goal is a small, production-minded demo of Java 21, Spring Boot,
MongoDB, MapStruct, Go, REST communication, automated tests with
Testcontainers, and Docker Compose.
