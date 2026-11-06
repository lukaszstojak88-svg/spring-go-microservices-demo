# spring-go-microservices-demo

A small, production-minded demo of three independent services in one
monorepo, communicating over REST/JSON:

| Service | Directory | Stack | Port | Database |
|---|---|---|---|---|
| Product Service | `product-service/` | Java 21, Spring Boot 3.5, Maven, Spring Data MongoDB, MapStruct | 8081 (internal) | `products_db` |
| Order Service | `order-service/` | Java 21, Spring Boot 3.5, Maven, Spring Data MongoDB, MapStruct, `RestClient` | 8082 (internal) | `orders_db` |
| BFF Service | `bff-service/` | Go, standard library `net/http` | 8080 (published) | none |

One MongoDB instance hosts two logical databases; each Java service has its
own database user with access to its own database only.

Allowed calls: BFF → Product, BFF → Order, Order → Product. Nothing else.

- Requirements: [docs/PROJECT_SPEC.md](docs/PROJECT_SPEC.md)
- Architecture, flows, decisions, risks: [docs/architecture.md](docs/architecture.md)

> **Status:** project skeleton. Services start and report health; product
> and order endpoints are implemented in later stages.

## Repository structure

```text
.
├── docker-compose.yml        # whole stack
├── .env.example              # development configuration template
├── docs/                     # specification and architecture
├── infra/mongo/              # MongoDB init script (databases + users)
├── product-service/          # independent Maven project
├── order-service/            # independent Maven project
└── bff-service/              # Go module
```

## Prerequisites

- Docker with Docker Compose v2
- For local builds and tests: JDK 21+ and Maven 3.9+, Go 1.24+
- Docker must be running for Java integration tests (Testcontainers)

## Configuration

```bash
cp .env.example .env
```

All values are non-secret development defaults; `docker-compose.yml` uses the
same defaults, so the stack also starts without `.env`.

MongoDB users are created by `infra/mongo/init-databases.js` **only on the
first start with an empty volume**. After changing database credentials run
`docker compose down -v`.

## Running the stack

```bash
docker compose up --build          # start; only the BFF port (8080) is published
docker compose down                # stop, keep MongoDB data
docker compose down -v             # stop and delete the MongoDB volume
```

Debug profile – additionally publishes the Java services on `127.0.0.1`:

```bash
docker compose --profile debug up --build
```

Startup order is driven by health checks: MongoDB → Product Service →
Order Service → BFF. MongoDB is never published to the host.

## Health checks

| URL | Description |
|---|---|
| `http://localhost:8080/health` | BFF liveness |
| `http://localhost:8081/actuator/health/readiness` | Product Service readiness (debug profile) |
| `http://localhost:8082/actuator/health/readiness` | Order Service readiness (debug profile) |

BFF readiness (`/ready`) – TODO (later stage).

## Tests

Java (in `product-service/` or `order-service/`):

```bash
mvn spotless:apply   # format
mvn test             # unit tests (*Test), no Docker needed
mvn verify           # + integration tests (*IT) with Testcontainers, Docker required
```

Go (in `bff-service/`):

```bash
gofmt -l .
go vet ./...
go test ./...
```

## API

TODO (later stages): endpoint descriptions and example `curl` commands
through the BFF.
