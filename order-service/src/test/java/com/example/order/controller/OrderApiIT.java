package com.example.order.controller;

import static org.assertj.core.api.Assertions.assertThat;
import static org.hamcrest.Matchers.containsString;
import static org.hamcrest.Matchers.endsWith;
import static org.hamcrest.Matchers.not;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.header;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.example.order.model.entity.Order;
import com.example.order.model.entity.OrderStatus;
import com.example.order.repository.OrderRepository;
import com.example.order.support.MongoContainerSupport;
import java.io.IOException;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.TimeUnit;
import okhttp3.mockwebserver.Dispatcher;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import okhttp3.mockwebserver.SocketPolicy;
import org.bson.types.ObjectId;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.http.MediaType;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.springframework.test.web.servlet.MockMvc;

/**
 * Full application (real MongoDB via Testcontainers) with the Product Service replaced by a stub
 * HTTP server. Verifies endpoints, validation, serialization and error mapping without a running
 * Product Service.
 */
@SpringBootTest
@AutoConfigureMockMvc
class OrderApiIT extends MongoContainerSupport {

  private static final MockWebServer PRODUCT_SERVICE = new MockWebServer();
  private static final List<RecordedRequest> RECEIVED = new CopyOnWriteArrayList<>();
  private static volatile MockResponse productResponse;

  static {
    PRODUCT_SERVICE.setDispatcher(
        new Dispatcher() {
          @Override
          public MockResponse dispatch(RecordedRequest request) {
            RECEIVED.add(request);
            return productResponse != null
                ? productResponse
                : new MockResponse().setResponseCode(500);
          }
        });
    try {
      PRODUCT_SERVICE.start();
    } catch (IOException e) {
      throw new IllegalStateException(e);
    }
  }

  @DynamicPropertySource
  static void productServiceProperties(DynamicPropertyRegistry registry) {
    registry.add("product-service.base-url", () -> PRODUCT_SERVICE.url("/").toString());
    registry.add("product-service.connect-timeout", () -> "1s");
    registry.add("product-service.read-timeout", () -> "500ms");
  }

  @AfterAll
  static void stopStub() throws IOException {
    PRODUCT_SERVICE.shutdown();
  }

  @Autowired private MockMvc mockMvc;
  @Autowired private OrderRepository repository;

  @BeforeEach
  void reset() {
    repository.deleteAll();
    RECEIVED.clear();
    productResponse = null;
  }

  // ---- create ----

  @Test
  void createsOrderWithSnapshotTotalsAndLocation() throws Exception {
    productStub(
        """
        {"products":[
          {"id":"p1","name":"Mechanical Keyboard","price":"349.99","currency":"PLN","available":true},
          {"id":"p2","name":"Mouse","price":"20","currency":"PLN","available":true}],
         "missingIds":[]}
        """);

    String body =
        mockMvc
            .perform(
                post("/api/orders")
                    .contentType(MediaType.APPLICATION_JSON)
                    .content(createRequest("customer-42", item("p1", 2), item("p2", 3))))
            .andExpect(status().isCreated())
            .andExpect(header().string("Location", containsString("/api/orders/")))
            .andExpect(jsonPath("$.id").isNotEmpty())
            .andExpect(jsonPath("$.customerId").value("customer-42"))
            .andExpect(jsonPath("$.status").value("CREATED"))
            .andExpect(jsonPath("$.currency").value("PLN"))
            .andExpect(jsonPath("$.totalAmount").value("759.98"))
            .andExpect(jsonPath("$.items.length()").value(2))
            .andExpect(jsonPath("$.items[0].productId").value("p1"))
            .andExpect(jsonPath("$.items[0].productName").value("Mechanical Keyboard"))
            .andExpect(jsonPath("$.items[0].unitPrice").value("349.99"))
            .andExpect(jsonPath("$.items[0].quantity").value(2))
            .andExpect(jsonPath("$.items[0].lineTotal").value("699.98"))
            .andExpect(jsonPath("$.items[1].unitPrice").value("20.00"))
            .andExpect(jsonPath("$.items[1].lineTotal").value("60.00"))
            .andExpect(jsonPath("$.createdAt").value(endsWith("Z")))
            .andReturn()
            .getResponse()
            .getContentAsString();

    assertThat(repository.count()).isEqualTo(1);
    Order stored = repository.findAll().getFirst();
    assertThat(stored.getStatus()).isEqualTo(OrderStatus.CREATED);
    assertThat(stored.getItems().getFirst().getProductName()).isEqualTo("Mechanical Keyboard");
    assertThat(body).contains(stored.getId());

    assertThat(RECEIVED).hasSize(1);
    RecordedRequest lookup = RECEIVED.getFirst();
    assertThat(lookup.getPath()).isEqualTo("/api/products/lookup");
    assertThat(lookup.getBody().readUtf8()).isEqualTo("{\"ids\":[\"p1\",\"p2\"]}");
  }

  @Test
  void ignoresPricesSentByTheClient() throws Exception {
    productStub(oneProduct("p1", "10.00", "PLN", true));

    mockMvc
        .perform(
            post("/api/orders")
                .contentType(MediaType.APPLICATION_JSON)
                .content(
                    """
                    {"items":[{"productId":"p1","quantity":2,"unitPrice":"0.01","price":"0.01"}],
                     "totalAmount":"0.01"}
                    """))
        .andExpect(status().isCreated())
        .andExpect(jsonPath("$.items[0].unitPrice").value("10.00"))
        .andExpect(jsonPath("$.totalAmount").value("20.00"));
  }

  @Test
  void forwardsRequestIdToProductServiceAndEchoesIt() throws Exception {
    productStub(oneProduct("p1", "10.00", "PLN", true));

    mockMvc
        .perform(
            post("/api/orders")
                .header("X-Request-Id", "trace-abc-123")
                .contentType(MediaType.APPLICATION_JSON)
                .content(createRequest(null, item("p1", 1))))
        .andExpect(status().isCreated())
        .andExpect(header().string("X-Request-Id", "trace-abc-123"));

    assertThat(RECEIVED.getFirst().getHeader("X-Request-Id")).isEqualTo("trace-abc-123");
  }

  // ---- validation ----

  @Test
  void rejectsEmptyItems() throws Exception {
    assertValidationError("{\"items\":[]}");
  }

  @Test
  void rejectsMissingItems() throws Exception {
    assertValidationError("{\"customerId\":\"c1\"}");
  }

  @ParameterizedTest
  @ValueSource(ints = {0, -1})
  void rejectsNonPositiveQuantity(int quantity) throws Exception {
    assertValidationError(createRequest(null, item("p1", quantity)));
  }

  @Test
  void rejectsDuplicateProductIds() throws Exception {
    assertValidationError(createRequest(null, item("p1", 1), item("p1", 2)));
  }

  @Test
  void rejectsBlankProductId() throws Exception {
    assertValidationError(createRequest(null, item(" ", 1)));
  }

  @Test
  void rejectsMalformedJson() throws Exception {
    assertValidationError("{not json");
  }

  @Test
  void rejectsFractionalQuantity() throws Exception {
    assertValidationError("{\"items\":[{\"productId\":\"p1\",\"quantity\":1.5}]}");
  }

  // ---- business rule errors (422) ----

  @Test
  void rejectsMissingProductsAndPersistsNothing() throws Exception {
    productStub(
        """
        {"products":[{"id":"p1","name":"A","price":"1.00","currency":"PLN","available":true}],
         "missingIds":["p2"]}
        """);

    mockMvc
        .perform(
            post("/api/orders")
                .contentType(MediaType.APPLICATION_JSON)
                .content(createRequest(null, item("p1", 1), item("p2", 1))))
        .andExpect(status().isUnprocessableEntity())
        .andExpect(jsonPath("$.code").value("PRODUCT_NOT_FOUND"))
        .andExpect(jsonPath("$.status").value(422))
        .andExpect(jsonPath("$.details[0].field").value("items[1].productId"));

    assertThat(repository.count()).isZero();
  }

  @Test
  void rejectsUnavailableProductsAndPersistsNothing() throws Exception {
    productStub(oneProduct("p1", "1.00", "PLN", false));

    mockMvc
        .perform(
            post("/api/orders")
                .contentType(MediaType.APPLICATION_JSON)
                .content(createRequest(null, item("p1", 1))))
        .andExpect(status().isUnprocessableEntity())
        .andExpect(jsonPath("$.code").value("PRODUCT_UNAVAILABLE"))
        .andExpect(jsonPath("$.details[0].field").value("items[0].productId"));

    assertThat(repository.count()).isZero();
  }

  @Test
  void rejectsMixedCurrenciesAndPersistsNothing() throws Exception {
    productStub(
        """
        {"products":[
          {"id":"p1","name":"A","price":"1.00","currency":"PLN","available":true},
          {"id":"p2","name":"B","price":"2.00","currency":"EUR","available":true}],
         "missingIds":[]}
        """);

    mockMvc
        .perform(
            post("/api/orders")
                .contentType(MediaType.APPLICATION_JSON)
                .content(createRequest(null, item("p1", 1), item("p2", 1))))
        .andExpect(status().isUnprocessableEntity())
        .andExpect(jsonPath("$.code").value("CURRENCY_MISMATCH"))
        .andExpect(jsonPath("$.details[0].field").value("items[1].productId"));

    assertThat(repository.count()).isZero();
  }

  // ---- Product Service failures (503, nothing persisted) ----

  @Test
  void mapsProductServiceServerErrorTo503() throws Exception {
    productStub(new MockResponse().setResponseCode(500).setBody("{\"trace\":\"boom\"}"));

    assertProductServiceUnavailable();
  }

  @Test
  void mapsProductServiceClientErrorTo503AsContractError() throws Exception {
    productStub(new MockResponse().setResponseCode(400).setBody("{}"));

    assertProductServiceUnavailable();
  }

  @Test
  void mapsProductServiceTimeoutTo503() throws Exception {
    productStub(json(oneProduct("p1", "1.00", "PLN", true)).setHeadersDelay(3, TimeUnit.SECONDS));

    long start = System.nanoTime();
    assertProductServiceUnavailable();

    assertThat(TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start))
        .as("fails at the read timeout (500 ms), not after the delayed response")
        .isLessThan(2500);
  }

  @Test
  void mapsDroppedConnectionTo503() throws Exception {
    productStub(new MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AT_START));

    assertProductServiceUnavailable();
  }

  @Test
  void mapsInvalidProductServiceResponseTo503() throws Exception {
    productStub("{\"unexpected\":true}");

    assertProductServiceUnavailable();
  }

  // ---- read ----

  @Test
  void getsOrderById() throws Exception {
    productStub(oneProduct("p1", "349.99", "PLN", true));
    String id = createOrder(item("p1", 2));

    mockMvc
        .perform(get("/api/orders/{id}", id))
        .andExpect(status().isOk())
        .andExpect(jsonPath("$.id").value(id))
        .andExpect(jsonPath("$.status").value("CREATED"))
        .andExpect(jsonPath("$.totalAmount").value("699.98"))
        .andExpect(jsonPath("$.items[0].productName").value("Product p1"));
  }

  @Test
  void readsOrderSnapshotWithoutCallingProductService() throws Exception {
    productStub(oneProduct("p1", "349.99", "PLN", true));
    String id = createOrder(item("p1", 2));
    RECEIVED.clear();
    // The Product Service now fails and its data would differ: the snapshot must still be served.
    productStub(new MockResponse().setResponseCode(500));

    mockMvc
        .perform(get("/api/orders/{id}", id))
        .andExpect(status().isOk())
        .andExpect(jsonPath("$.items[0].unitPrice").value("349.99"))
        .andExpect(jsonPath("$.totalAmount").value("699.98"));
    mockMvc.perform(get("/api/orders")).andExpect(status().isOk());

    assertThat(RECEIVED).as("reading never calls the Product Service").isEmpty();
  }

  @Test
  void returnsNotFoundForMissingOrder() throws Exception {
    mockMvc
        .perform(get("/api/orders/{id}", new ObjectId().toHexString()))
        .andExpect(status().isNotFound())
        .andExpect(jsonPath("$.code").value("NOT_FOUND"))
        .andExpect(jsonPath("$.status").value(404));
  }

  @Test
  void errorBodyContainsRequestIdAndNoInternals() throws Exception {
    mockMvc
        .perform(get("/api/orders/{id}", "does-not-exist").header("X-Request-Id", "req-777"))
        .andExpect(status().isNotFound())
        .andExpect(jsonPath("$.requestId").value("req-777"))
        .andExpect(jsonPath("$.path").value("/api/orders/does-not-exist"))
        .andExpect(jsonPath("$.timestamp").value(endsWith("Z")))
        .andExpect(jsonPath("$.trace").doesNotExist());
  }

  // ---- list ----

  @Test
  void listsOrdersWithPaginationAndStatusFilter() throws Exception {
    productStub(oneProduct("p1", "1.00", "PLN", true));
    String first = createOrder(item("p1", 1));
    createOrder(item("p1", 2));
    createOrder(item("p1", 3));
    mockMvc.perform(post("/api/orders/{id}/cancel", first)).andExpect(status().isOk());

    mockMvc
        .perform(get("/api/orders").param("page", "0").param("size", "2"))
        .andExpect(status().isOk())
        .andExpect(jsonPath("$.content.length()").value(2))
        .andExpect(jsonPath("$.page").value(0))
        .andExpect(jsonPath("$.size").value(2))
        .andExpect(jsonPath("$.totalElements").value(3))
        .andExpect(jsonPath("$.totalPages").value(2));
    mockMvc
        .perform(get("/api/orders").param("status", "CANCELLED"))
        .andExpect(status().isOk())
        .andExpect(jsonPath("$.totalElements").value(1))
        .andExpect(jsonPath("$.content[0].id").value(first))
        .andExpect(jsonPath("$.content[0].status").value("CANCELLED"));
    mockMvc
        .perform(get("/api/orders").param("status", "CREATED").param("sort", "totalAmount,asc"))
        .andExpect(status().isOk())
        .andExpect(jsonPath("$.totalElements").value(2))
        .andExpect(jsonPath("$.content[0].totalAmount").value("2.00"))
        .andExpect(jsonPath("$.content[1].totalAmount").value("3.00"));
  }

  @ParameterizedTest
  @ValueSource(
      strings = {
        "size=0",
        "size=101",
        "page=-1",
        "status=SHIPPED",
        "sort=customerId,asc",
        "sort=createdAt,up"
      })
  void rejectsInvalidListParameters(String query) throws Exception {
    mockMvc
        .perform(get("/api/orders?" + query))
        .andExpect(status().isBadRequest())
        .andExpect(jsonPath("$.code").value("VALIDATION_ERROR"));
  }

  // ---- cancel ----

  @Test
  void cancelsOrder() throws Exception {
    productStub(oneProduct("p1", "5.00", "PLN", true));
    String id = createOrder(item("p1", 1));

    mockMvc
        .perform(post("/api/orders/{id}/cancel", id))
        .andExpect(status().isOk())
        .andExpect(jsonPath("$.id").value(id))
        .andExpect(jsonPath("$.status").value("CANCELLED"))
        .andExpect(jsonPath("$.totalAmount").value("5.00"));

    assertThat(repository.findById(id).orElseThrow().getStatus()).isEqualTo(OrderStatus.CANCELLED);
  }

  @Test
  void cancellingAgainReturnsConflict() throws Exception {
    productStub(oneProduct("p1", "5.00", "PLN", true));
    String id = createOrder(item("p1", 1));
    mockMvc.perform(post("/api/orders/{id}/cancel", id)).andExpect(status().isOk());

    mockMvc
        .perform(post("/api/orders/{id}/cancel", id))
        .andExpect(status().isConflict())
        .andExpect(jsonPath("$.code").value("CONFLICT"))
        .andExpect(jsonPath("$.status").value(409));
    assertThat(repository.findById(id).orElseThrow().getStatus()).isEqualTo(OrderStatus.CANCELLED);
  }

  @Test
  void cancelDoesNotCallProductService() throws Exception {
    productStub(oneProduct("p1", "5.00", "PLN", true));
    String id = createOrder(item("p1", 1));
    RECEIVED.clear();

    mockMvc.perform(post("/api/orders/{id}/cancel", id)).andExpect(status().isOk());

    assertThat(RECEIVED).isEmpty();
  }

  @Test
  void cancellingMissingOrderReturnsNotFound() throws Exception {
    mockMvc
        .perform(post("/api/orders/{id}/cancel", new ObjectId().toHexString()))
        .andExpect(status().isNotFound())
        .andExpect(jsonPath("$.code").value("NOT_FOUND"));
  }

  // ---- unsupported operations ----

  @Test
  void ordersCannotBeUpdatedOrDeleted() throws Exception {
    mockMvc
        .perform(
            org.springframework.test.web.servlet.request.MockMvcRequestBuilders.delete(
                "/api/orders/abc"))
        .andExpect(status().isMethodNotAllowed())
        .andExpect(jsonPath("$.code").value("METHOD_NOT_ALLOWED"));
  }

  // ---- helpers ----

  private void assertValidationError(String requestBody) throws Exception {
    mockMvc
        .perform(post("/api/orders").contentType(MediaType.APPLICATION_JSON).content(requestBody))
        .andExpect(status().isBadRequest())
        .andExpect(jsonPath("$.code").value("VALIDATION_ERROR"))
        .andExpect(jsonPath("$.status").value(400));
    assertThat(RECEIVED).as("invalid requests never reach the Product Service").isEmpty();
    assertThat(repository.count()).isZero();
  }

  private void assertProductServiceUnavailable() throws Exception {
    mockMvc
        .perform(
            post("/api/orders")
                .contentType(MediaType.APPLICATION_JSON)
                .content(createRequest(null, item("p1", 1))))
        .andExpect(status().isServiceUnavailable())
        .andExpect(jsonPath("$.code").value("PRODUCT_SERVICE_UNAVAILABLE"))
        .andExpect(jsonPath("$.status").value(503))
        .andExpect(jsonPath("$.message").value(not(containsString("127.0.0.1"))))
        .andExpect(jsonPath("$.message").value(not(containsString("Exception"))));
    assertThat(repository.count()).as("no order persisted").isZero();
  }

  private String createOrder(String... items) throws Exception {
    String response =
        mockMvc
            .perform(
                post("/api/orders")
                    .contentType(MediaType.APPLICATION_JSON)
                    .content(createRequest(null, items)))
            .andExpect(status().isCreated())
            .andReturn()
            .getResponse()
            .getContentAsString();
    return com.jayway.jsonpath.JsonPath.read(response, "$.id");
  }

  private static void productStub(String body) {
    productResponse = json(body);
  }

  private static void productStub(MockResponse response) {
    productResponse = response;
  }

  private static MockResponse json(String body) {
    return new MockResponse()
        .setResponseCode(200)
        .setHeader("Content-Type", "application/json")
        .setBody(body);
  }

  private static String oneProduct(String id, String price, String currency, boolean available) {
    return "{\"products\":[{\"id\":\"%s\",\"name\":\"Product %s\",\"price\":\"%s\",\"currency\":\"%s\",\"available\":%s}],\"missingIds\":[]}"
        .formatted(id, id, price, currency, available);
  }

  private static String item(String productId, int quantity) {
    return "{\"productId\":\"%s\",\"quantity\":%d}".formatted(productId, quantity);
  }

  private static String createRequest(String customerId, String... items) {
    String customer = customerId == null ? "" : "\"customerId\":\"%s\",".formatted(customerId);
    return "{%s\"items\":[%s]}".formatted(customer, String.join(",", items));
  }
}
