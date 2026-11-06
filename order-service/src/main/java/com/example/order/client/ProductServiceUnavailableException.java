package com.example.order.client;

/**
 * The Product Service could not provide a usable answer (unreachable, timeout, error status, or
 * invalid response). Mapped to {@code 503 PRODUCT_SERVICE_UNAVAILABLE}; the message is for logs
 * only and is never returned to clients.
 */
public class ProductServiceUnavailableException extends RuntimeException {

  public ProductServiceUnavailableException(String message, Throwable cause) {
    super(message, cause);
  }

  public ProductServiceUnavailableException(String message) {
    super(message);
  }
}
