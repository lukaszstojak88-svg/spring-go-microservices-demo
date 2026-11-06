package com.example.order.client;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.example.order.config.ProductClientConfig;
import java.io.IOException;
import java.math.BigDecimal;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.TimeUnit;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import okhttp3.mockwebserver.SocketPolicy;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;
import org.slf4j.MDC;
import org.springframework.web.client.RestClient;

/**
 * {@link RestClientProductClient} against a stub HTTP server. The client is built by the production
 * {@link ProductClientConfig}, so timeouts and header propagation are the real ones. No Docker, no
 * running Product Service.
 */
class RestClientProductClientTest {

  private static final String FOUND_BODY =
      """
      {"products":[
        {"id":"p1","name":"Mechanical Keyboard","price":"349.99","currency":"PLN","available":true,
         "description":"ignored","createdAt":"2026-01-15T10:30:00Z"},
        {"id":"p2","name":"Mouse","price":"20.00","currency":"PLN","available":false}
      ],"missingIds":["p3"]}
      """;

  private MockWebServer server;
  private ProductClient client;

  @BeforeEach
  void setUp() throws IOException {
    server = new MockWebServer();
    server.start();
    client = clientFor(server.url("/").toString(), Duration.ofMillis(300));
  }

  @AfterEach
  void tearDown() throws IOException {
    MDC.clear();
    server.shutdown();
  }

  @Test
  void returnsProductsAndMissingIdsFromOneBatchCall() throws Exception {
    server.enqueue(json(200, FOUND_BODY));

    ProductLookupResponse response = client.lookup(List.of("p1", "p2", "p3"));

    assertThat(response.missingIds()).containsExactly("p3");
    assertThat(response.products())
        .extracting(ProductData::id, ProductData::name, ProductData::currency)
        .containsExactly(
            org.assertj.core.groups.Tuple.tuple("p1", "Mechanical Keyboard", "PLN"),
            org.assertj.core.groups.Tuple.tuple("p2", "Mouse", "PLN"));
    // Money is parsed exactly from the decimal string; unknown fields are ignored.
    assertThat(response.products().get(0).price()).isEqualTo(new BigDecimal("349.99"));
    assertThat(response.products().get(1).price()).isEqualTo(new BigDecimal("20.00"));
    assertThat(response.products().get(0).available()).isTrue();
    assertThat(response.products().get(1).available()).isFalse();

    assertThat(server.getRequestCount()).isEqualTo(1);
    RecordedRequest request = server.takeRequest(1, TimeUnit.SECONDS);
    assertThat(request.getMethod()).isEqualTo("POST");
    assertThat(request.getPath()).isEqualTo("/api/products/lookup");
    assertThat(request.getHeader("Content-Type")).startsWith("application/json");
    assertThat(request.getBody().readUtf8()).isEqualTo("{\"ids\":[\"p1\",\"p2\",\"p3\"]}");
  }

  @Test
  void returnsEmptyResultsWhenNothingFound() {
    server.enqueue(json(200, "{\"products\":[],\"missingIds\":[\"p1\"]}"));

    ProductLookupResponse response = client.lookup(List.of("p1"));

    assertThat(response.products()).isEmpty();
    assertThat(response.missingIds()).containsExactly("p1");
  }

  @Test
  void forwardsRequestIdFromMdc() throws Exception {
    server.enqueue(json(200, FOUND_BODY));
    MDC.put("requestId", "req-123.abc");

    client.lookup(List.of("p1"));

    assertThat(server.takeRequest(1, TimeUnit.SECONDS).getHeader("X-Request-Id"))
        .isEqualTo("req-123.abc");
  }

  @Test
  void sendsNoRequestIdHeaderWhenMdcIsEmpty() throws Exception {
    server.enqueue(json(200, FOUND_BODY));

    client.lookup(List.of("p1"));

    assertThat(server.takeRequest(1, TimeUnit.SECONDS).getHeader("X-Request-Id")).isNull();
  }

  @ParameterizedTest
  @ValueSource(ints = {400, 404, 422})
  void translatesClientErrorStatusToUnavailable(int status) {
    server.enqueue(json(status, "{\"code\":\"VALIDATION_ERROR\"}"));

    assertThatThrownBy(() -> client.lookup(List.of("p1")))
        .isInstanceOf(ProductServiceUnavailableException.class)
        .hasMessageContaining(String.valueOf(status));
    assertThat(server.getRequestCount()).isEqualTo(1);
  }

  @ParameterizedTest
  @ValueSource(ints = {500, 502, 503})
  void translatesServerErrorStatusToUnavailableWithoutRetry(int status) {
    server.enqueue(json(status, "{\"code\":\"INTERNAL_ERROR\"}"));

    assertThatThrownBy(() -> client.lookup(List.of("p1")))
        .isInstanceOf(ProductServiceUnavailableException.class)
        .hasMessageContaining(String.valueOf(status));
    assertThat(server.getRequestCount()).as("no automatic retries").isEqualTo(1);
  }

  @Test
  void translatesReadTimeoutToUnavailable() {
    server.enqueue(json(200, FOUND_BODY).setHeadersDelay(3, TimeUnit.SECONDS));

    long start = System.nanoTime();
    assertThatThrownBy(() -> client.lookup(List.of("p1")))
        .isInstanceOf(ProductServiceUnavailableException.class)
        .hasMessageContaining("unreachable or timed out");
    Duration elapsed = Duration.ofNanos(System.nanoTime() - start);

    assertThat(elapsed)
        .as("fails at the read timeout, not after the response")
        .isLessThan(Duration.ofSeconds(2));
  }

  @Test
  void translatesConnectionResetToUnavailable() {
    server.enqueue(new MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AT_START));

    assertThatThrownBy(() -> client.lookup(List.of("p1")))
        .isInstanceOf(ProductServiceUnavailableException.class);
  }

  @Test
  void translatesConnectionRefusedToUnavailable() throws IOException {
    String closedUrl = server.url("/").toString();
    server.shutdown();
    ProductClient refused = clientFor(closedUrl, Duration.ofSeconds(1));

    assertThatThrownBy(() -> refused.lookup(List.of("p1")))
        .isInstanceOf(ProductServiceUnavailableException.class)
        .hasMessageContaining("unreachable or timed out");
  }

  @Test
  void translatesMalformedJsonToUnavailable() {
    server.enqueue(json(200, "{not json"));

    assertThatThrownBy(() -> client.lookup(List.of("p1")))
        .isInstanceOf(ProductServiceUnavailableException.class);
  }

  @Test
  void translatesEmptyBodyToUnavailable() {
    server.enqueue(new MockResponse().setResponseCode(200));

    assertThatThrownBy(() -> client.lookup(List.of("p1")))
        .isInstanceOf(ProductServiceUnavailableException.class);
  }

  @ParameterizedTest
  @ValueSource(
      strings = {
        "{\"missingIds\":[]}",
        "{\"products\":[]}",
        "{\"products\":[{\"id\":\"p1\",\"name\":\"A\",\"currency\":\"PLN\",\"available\":true}],\"missingIds\":[]}",
        "{\"products\":[{\"id\":\"p1\",\"name\":\"A\",\"price\":\"1.00\",\"currency\":\"PLN\"}],\"missingIds\":[]}",
        "{\"products\":[null],\"missingIds\":[]}"
      })
  void rejectsIncompleteResponseAsContractError(String body) {
    server.enqueue(json(200, body));

    assertThatThrownBy(() -> client.lookup(List.of("p1")))
        .isInstanceOf(ProductServiceUnavailableException.class);
  }

  @Test
  void doesNotLeakInternalDetailsInExceptionMessage() {
    server.enqueue(json(500, "{\"trace\":\"com.example.secret.InternalClass\"}"));

    assertThatThrownBy(() -> client.lookup(List.of("p1")))
        .isInstanceOf(ProductServiceUnavailableException.class)
        .hasMessageNotContaining("InternalClass")
        .hasMessageNotContaining(String.valueOf(server.getPort()));
  }

  private static ProductClient clientFor(String baseUrl, Duration readTimeout) {
    ProductServiceProperties properties =
        new ProductServiceProperties(
            java.net.URI.create(baseUrl), Duration.ofSeconds(1), readTimeout);
    return new ProductClientConfig().productClient(RestClient.builder(), properties);
  }

  private static MockResponse json(int status, String body) {
    return new MockResponse()
        .setResponseCode(status)
        .setHeader("Content-Type", "application/json")
        .setBody(body);
  }
}
