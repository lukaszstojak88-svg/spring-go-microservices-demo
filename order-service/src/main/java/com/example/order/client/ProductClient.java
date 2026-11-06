package com.example.order.client;

import java.util.Collection;

/**
 * Port to the Product Service REST contract. Isolated behind an interface so that the service layer
 * can be unit tested with Mockito.
 */
public interface ProductClient {

  /**
   * Looks up products by id in a single batch call.
   *
   * @return found products and missing ids (missing ids are not an error at this level)
   * @throws ProductServiceUnavailableException when the Product Service is unreachable, times out,
   *     returns an error status, or returns an invalid response
   */
  ProductLookupResponse lookup(Collection<String> ids);
}
