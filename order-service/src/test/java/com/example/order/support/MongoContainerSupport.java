package com.example.order.support;

import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.testcontainers.containers.MongoDBContainer;

/**
 * Base class for integration tests (*IT). Starts ONE real MongoDB container per test JVM (singleton
 * pattern) and lets Spring Boot connect to it via {@link ServiceConnection}, using the URI supplied
 * by the container (no fixed host port). Ryuk removes the container after the run.
 *
 * <p>Requires Docker. Unit tests (*Test) must not extend this class.
 */
public abstract class MongoContainerSupport {

  @ServiceConnection
  protected static final MongoDBContainer MONGO = new MongoDBContainer("mongo:8.0.34");

  static {
    MONGO.start();
  }
}
