# syntax=docker/dockerfile:1

# --- Build stage ---
FROM golang:1.27.3-alpine AS build
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bff ./cmd/server

# --- Runtime stage: no shell, no package manager, non-root user ---
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bff /app/bff

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/bff"]
