package com.example.order.client;

import java.util.Collection;
import java.util.List;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.MediaType;
import org.springframework.web.client.ResourceAccessException;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;
import org.springframework.web.client.RestClientResponseException;

/**
 * {@link ProductClient} backed by Spring {@link RestClient}. Every failure is translated into
 * {@link ProductServiceUnavailableException}; details are logged, never exposed. No automatic
 * retries (single attempt).
 */
public class RestClientProductClient implements ProductClient {

  static final String LOOKUP_PATH = "/api/products/lookup";

  private static final Logger log = LoggerFactory.getLogger(RestClientProductClient.class);

  private final RestClient restClient;

  public RestClientProductClient(RestClient restClient) {
    this.restClient = restClient;
  }

  @Override
  public ProductLookupResponse lookup(Collection<String> ids) {
    ProductLookupResponse response;
    try {
      response =
          restClient
              .post()
              .uri(LOOKUP_PATH)
              .contentType(MediaType.APPLICATION_JSON)
              .accept(MediaType.APPLICATION_JSON)
              .body(new ProductLookupRequest(List.copyOf(ids)))
              .retrieve()
              .body(ProductLookupResponse.class);
    } catch (RestClientResponseException e) {
      if (e.getStatusCode().is4xxClientError()) {
        // A 4xx for a valid lookup means a contract mismatch, not a transient failure.
        log.error(
            "Product Service rejected the lookup request (contract error): status={}",
            e.getStatusCode().value());
      } else {
        log.warn("Product Service returned an error: status={}", e.getStatusCode().value());
      }
      throw new ProductServiceUnavailableException(
          "Product Service returned status " + e.getStatusCode().value(), e);
    } catch (ResourceAccessException e) {
      log.warn("Product Service unreachable or timed out: {}", e.getMessage());
      throw new ProductServiceUnavailableException("Product Service unreachable or timed out", e);
    } catch (RestClientException e) {
      log.error("Product Service returned an unreadable response: {}", e.getMessage());
      throw new ProductServiceUnavailableException("Unreadable Product Service response", e);
    }
    return validated(response);
  }

  private static ProductLookupResponse validated(ProductLookupResponse response) {
    if (response == null
        || response.products() == null
        || response.missingIds() == null
        || response.products().stream().anyMatch(p -> p == null || !p.isComplete())) {
      log.error("Product Service returned an incomplete lookup response (contract error)");
      throw new ProductServiceUnavailableException("Incomplete Product Service response");
    }
    return response;
  }
}
