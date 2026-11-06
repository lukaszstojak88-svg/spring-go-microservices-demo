# syntax=docker/dockerfile:1

# --- Build stage ---
FROM maven:3.9.16-eclipse-temurin-21 AS build
WORKDIR /workspace

# Resolve dependencies in a separate layer to reuse the cache when only sources change.
COPY pom.xml .
RUN mvn -B -q dependency:go-offline

COPY src ./src
RUN mvn -B -q package -DskipTests

# --- Runtime stage ---
FROM eclipse-temurin:21.0.13_9-jre
WORKDIR /app

RUN useradd --system --no-create-home --shell /usr/sbin/nologin app
COPY --from=build --chown=app:app /workspace/target/app.jar /app/app.jar

USER app
EXPOSE 8081
# curl is available in the base image and used by the Compose health check.
ENTRYPOINT ["java", "-XX:MaxRAMPercentage=75", "-jar", "/app/app.jar"]
