package com.example.order.client;

import java.math.BigDecimal;

/**
 * The subset of the Product Service product representation that the Order Service needs. The price
 * arrives as a decimal string (e.g. {@code "349.99"}) and is parsed exactly into {@link
 * BigDecimal}. Unknown fields are ignored, so additive changes in the Product Service do not break
 * this client.
 */
public record ProductData(
    String id, String name, BigDecimal price, String currency, Boolean available) {

  boolean isComplete() {
    return id != null && name != null && price != null && currency != null && available != null;
  }
}
