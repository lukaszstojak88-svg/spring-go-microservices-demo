package com.example.product.controller;

import static org.assertj.core.api.Assertions.assertThat;
import static org.hamcrest.Matchers.endsWith;
import static org.hamcrest.Matchers.hasSize;
import static org.hamcrest.Matchers.isA;
import static org.hamcrest.Matchers.matchesPattern;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.delete;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.put;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.header;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.example.product.model.entity.Product;
import com.example.product.repository.ProductRepository;
import com.example.product.support.MongoContainerSupport;
import com.jayway.jsonpath.JsonPath;
import java.math.BigDecimal;
import java.util.Collections;
import java.util.stream.Collectors;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.ResultActions;

/**
 * REST API integration tests: full Spring context, MockMvc, real MongoDB (Testcontainers). Each
 * test starts from an empty {@code products} collection.
 */
@SpringBootTest
@AutoConfigureMockMvc
class ProductApiIT extends MongoContainerSupport {

  private static final String ISO_UTC_MILLIS =
      "\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(\\.\\d{1,3})?Z";
  private static final String MISSING_ID = "673abc1234567890abcdef12";

  @Autowired private MockMvc mockMvc;
  @Autowired private ProductRepository repository;

  @BeforeEach
  void cleanDatabase() {
    repository.deleteAll();
  }

  @Nested
  class CreateProduct {

    @Test
    void returns201WithLocationAndBody() throws Exception {
      String body =
          createProduct(
                  """
                  {"name":"Mechanical Keyboard","description":"Compact mechanical keyboard",
                   "price":"349.99","currency":"PLN","available":true}
                  """)
              .andExpect(status().isCreated())
              .andExpect(
                  header().string("Location", matchesPattern(".*/api/products/[0-9a-f]{24}")))
              .andExpect(jsonPath("$.id").isString())
              .andExpect(jsonPath("$.name").value("Mechanical Keyboard"))
              .andExpect(jsonPath("$.description").value("Compact mechanical keyboard"))
              .andExpect(jsonPath("$.price").value("349.99"))
              .andExpect(jsonPath("$.price", isA(String.class)))
              .andExpect(jsonPath("$.currency").value("PLN"))
              .andExpect(jsonPath("$.available").value(true))
              .andExpect(jsonPath("$.createdAt", matchesPattern(ISO_UTC_MILLIS)))
              .andExpect(jsonPath("$.updatedAt", matchesPattern(ISO_UTC_MILLIS)))
              .andReturn()
              .getResponse()
              .getContentAsString();

      String id = JsonPath.read(body, "$.id");
      Product stored = repository.findById(id).orElseThrow();
      assertThat(stored.getPrice()).isEqualTo(new BigDecimal("349.99"));
    }

    @Test
    void normalizesPriceToCurrencyScale() throws Exception {
      createProduct(product("Mouse", "\"20\"", "PLN"))
          .andExpect(status().isCreated())
          .andExpect(jsonPath("$.price").value("20.00"));
    }

    @Test
    void acceptsNumericPriceWithoutFloatingPointError() throws Exception {
      createProduct(product("Mouse", "0.1", "PLN"))
          .andExpect(status().isCreated())
          .andExpect(jsonPath("$.price").value("0.10"));
    }

    @Test
    void rejectsMissingRequiredFields() throws Exception {
      createProduct("{}")
          .andExpect(status().isBadRequest())
          .andExpect(jsonPath("$.code").value("VALIDATION_ERROR"))
          .andExpect(jsonPath("$.status").value(400))
          .andExpect(jsonPath("$.path").value("/api/products"))
          .andExpect(
              jsonPath("$.details[*].field")
                  .value(
                      org.hamcrest.Matchers.containsInAnyOrder(
                          "name", "price", "currency", "available")));
      assertThat(repository.count()).isZero();
    }

    @Test
    void rejectsNegativePrice() throws Exception {
      createProduct(product("Mouse", "\"-1.00\"", "PLN"))
          .andExpect(status().isBadRequest())
          .andExpect(jsonPath("$.details[0].field").value("price"));
    }

    @Test
    void rejectsTooManyFractionDigits() throws Exception {
      createProduct(product("Mouse", "\"1.999\"", "PLN"))
          .andExpect(status().isBadRequest())
          .andExpect(jsonPath("$.details[0].field").value("price"))
          .andExpect(
              jsonPath("$.details[0].message")
                  .value("must have at most 2 fraction digits for PLN"));
    }

    @Test
    void rejectsInvalidCurrency() throws Exception {
      createProduct(product("Mouse", "\"1.00\"", "pln"))
          .andExpect(status().isBadRequest())
          .andExpect(jsonPath("$.details[0].field").value("currency"));
    }

    @Test
    void rejectsMalformedJsonWithoutLeakingParserDetails() throws Exception {
      createProduct("{\"name\":")
          .andExpect(status().isBadRequest())
          .andExpect(jsonPath("$.code").value("VALIDATION_ERROR"))
          .andExpect(jsonPath("$.message").value("Malformed request body"))
          .andExpect(jsonPath("$.details").isEmpty());
    }

    @Test
    void rejectsNonNumericPrice() throws Exception {
      createProduct(product("Mouse", "\"abc\"", "PLN"))
          .andExpect(status().isBadRequest())
          .andExpect(jsonPath("$.message").value("Malformed request body"));
    }

    @Test
    void rejectsUnsupportedMediaType() throws Exception {
      mockMvc
          .perform(post("/api/products").contentType(MediaType.TEXT_PLAIN).content("x"))
          .andExpect(status().isUnsupportedMediaType())
          .andExpect(jsonPath("$.code").value("UNSUPPORTED_MEDIA_TYPE"));
    }
  }

  @Nested
  class GetProduct {

    @Test
    void returnsExistingProduct() throws Exception {
      String id = createAndGetId("Keyboard", "349.99");

      mockMvc
          .perform(get("/api/products/{id}", id))
          .andExpect(status().isOk())
          .andExpect(jsonPath("$.id").value(id))
          .andExpect(jsonPath("$.name").value("Keyboard"))
          .andExpect(jsonPath("$.price").value("349.99"));
    }

    @Test
    void returns404WithErrorStructureForMissingProduct() throws Exception {
      mockMvc
          .perform(get("/api/products/{id}", MISSING_ID).header("X-Request-Id", "req-404"))
          .andExpect(status().isNotFound())
          .andExpect(header().string("X-Request-Id", "req-404"))
          .andExpect(jsonPath("$.timestamp", matchesPattern(ISO_UTC_MILLIS)))
          .andExpect(jsonPath("$.status").value(404))
          .andExpect(jsonPath("$.code").value("NOT_FOUND"))
          .andExpect(jsonPath("$.message").value("Product " + MISSING_ID + " not found"))
          .andExpect(jsonPath("$.path").value("/api/products/" + MISSING_ID))
          .andExpect(jsonPath("$.requestId").value("req-404"))
          .andExpect(jsonPath("$.details").isArray());
    }

    @Test
    void returns404ForNonObjectIdIdentifier() throws Exception {
      mockMvc.perform(get("/api/products/not-an-id")).andExpect(status().isNotFound());
    }

    @Test
    void generatesRequestIdWhenMissing() throws Exception {
      mockMvc
          .perform(get("/api/products/{id}", MISSING_ID))
          .andExpect(header().string("X-Request-Id", matchesPattern("[0-9a-f-]{36}")))
          .andExpect(jsonPath("$.requestId", matchesPattern("[0-9a-f-]{36}")));
    }
  }

  @Nested
  class ListProducts {

    @Test
    void returnsEmptyPage() throws Exception {
      mockMvc
          .perform(get("/api/products"))
          .andExpect(status().isOk())
          .andExpect(jsonPath("$.content").isEmpty())
          .andExpect(jsonPath("$.page").value(0))
          .andExpect(jsonPath("$.size").value(20))
          .andExpect(jsonPath("$.totalElements").value(0))
          .andExpect(jsonPath("$.totalPages").value(0));
    }

    @Test
    void paginatesAndSortsByPrice() throws Exception {
      createAndGetId("C", "30.00");
      createAndGetId("A", "10.00");
      createAndGetId("E", "100.00");
      createAndGetId("B", "9.99");
      createAndGetId("D", "50.00");

      mockMvc
          .perform(
              get("/api/products").param("page", "1").param("size", "2").param("sort", "price,asc"))
          .andExpect(status().isOk())
          .andExpect(jsonPath("$.content", hasSize(2)))
          .andExpect(jsonPath("$.content[0].name").value("C"))
          .andExpect(jsonPath("$.content[1].name").value("D"))
          .andExpect(jsonPath("$.page").value(1))
          .andExpect(jsonPath("$.size").value(2))
          .andExpect(jsonPath("$.totalElements").value(5))
          .andExpect(jsonPath("$.totalPages").value(3));

      mockMvc
          .perform(
              get("/api/products").param("page", "2").param("size", "2").param("sort", "price,asc"))
          .andExpect(jsonPath("$.content", hasSize(1)))
          .andExpect(jsonPath("$.content[0].name").value("E"));
    }

    @Test
    void sortsByNameDescending() throws Exception {
      createAndGetId("Alpha", "1.00");
      createAndGetId("Gamma", "1.00");
      createAndGetId("Beta", "1.00");

      mockMvc
          .perform(get("/api/products").param("sort", "name,desc"))
          .andExpect(
              jsonPath("$.content[*].name")
                  .value(org.hamcrest.Matchers.contains("Gamma", "Beta", "Alpha")));
    }

    @Test
    void rejectsUnknownSortField() throws Exception {
      mockMvc
          .perform(get("/api/products").param("sort", "description,asc"))
          .andExpect(status().isBadRequest())
          .andExpect(jsonPath("$.code").value("VALIDATION_ERROR"))
          .andExpect(jsonPath("$.details[0].field").value("sort"));
    }

    @Test
    void rejectsOutOfRangeAndNonNumericPaging() throws Exception {
      mockMvc
          .perform(get("/api/products").param("size", "101"))
          .andExpect(status().isBadRequest())
          .andExpect(jsonPath("$.details[0].field").value("size"));
      mockMvc.perform(get("/api/products").param("size", "0")).andExpect(status().isBadRequest());
      mockMvc
          .perform(get("/api/products").param("page", "-1"))
          .andExpect(status().isBadRequest())
          .andExpect(jsonPath("$.details[0].field").value("page"));
      mockMvc
          .perform(get("/api/products").param("page", "abc"))
          .andExpect(status().isBadRequest())
          .andExpect(jsonPath("$.details[0].field").value("page"));
    }
  }

  @Nested
  class UpdateProduct {

    @Test
    void replacesFieldsAndKeepsCreatedAt() throws Exception {
      String id = createAndGetId("Mouse", "20.00");
      String createdAt = JsonPath.read(getJson(id), "$.createdAt");

      mockMvc
          .perform(
              put("/api/products/{id}", id)
                  .contentType(MediaType.APPLICATION_JSON)
                  .content(
                      """
                      {"name":"Mouse Pro","description":"Wireless","price":"25.5",
                       "currency":"PLN","available":false}
                      """))
          .andExpect(status().isOk())
          .andExpect(jsonPath("$.id").value(id))
          .andExpect(jsonPath("$.name").value("Mouse Pro"))
          .andExpect(jsonPath("$.description").value("Wireless"))
          .andExpect(jsonPath("$.price").value("25.50"))
          .andExpect(jsonPath("$.available").value(false))
          .andExpect(jsonPath("$.createdAt").value(createdAt));

      Product stored = repository.findById(id).orElseThrow();
      assertThat(stored.getName()).isEqualTo("Mouse Pro");
      assertThat(stored.getUpdatedAt()).isAfterOrEqualTo(stored.getCreatedAt());
    }

    @Test
    void returns404ForMissingProduct() throws Exception {
      mockMvc
          .perform(
              put("/api/products/{id}", MISSING_ID)
                  .contentType(MediaType.APPLICATION_JSON)
                  .content(product("X", "\"1.00\"", "PLN")))
          .andExpect(status().isNotFound())
          .andExpect(jsonPath("$.code").value("NOT_FOUND"));
    }

    @Test
    void rejectsInvalidBodyAndKeepsStoredProduct() throws Exception {
      String id = createAndGetId("Mouse", "20.00");

      mockMvc
          .perform(
              put("/api/products/{id}", id)
                  .contentType(MediaType.APPLICATION_JSON)
                  .content(product("", "\"1.999\"", "PLN")))
          .andExpect(status().isBadRequest())
          .andExpect(
              jsonPath("$.details[*].field")
                  .value(org.hamcrest.Matchers.containsInAnyOrder("name", "price")));

      assertThat(repository.findById(id).orElseThrow().getName()).isEqualTo("Mouse");
    }
  }

  @Nested
  class DeleteProduct {

    @Test
    void returns204AndRemovesProduct() throws Exception {
      String id = createAndGetId("Mouse", "20.00");

      mockMvc.perform(delete("/api/products/{id}", id)).andExpect(status().isNoContent());

      mockMvc.perform(get("/api/products/{id}", id)).andExpect(status().isNotFound());
      assertThat(repository.existsById(id)).isFalse();
    }

    @Test
    void returns404ForMissingProduct() throws Exception {
      mockMvc
          .perform(delete("/api/products/{id}", MISSING_ID))
          .andExpect(status().isNotFound())
          .andExpect(jsonPath("$.code").value("NOT_FOUND"));
    }
  }

  @Nested
  class BatchLookup {

    @Test
    void returnsFoundProductsAndMissingIdsInRequestOrder() throws Exception {
      String keyboard = createAndGetId("Keyboard", "349.99");
      String mouse = createAndGetId("Mouse", "20.00");

      lookup(
              "{\"ids\":[\"%s\",\"%s\",\"missing-1\",\"%s\",\"%s\"]}"
                  .formatted(mouse, MISSING_ID, keyboard, mouse))
          .andExpect(status().isOk())
          .andExpect(jsonPath("$.products", hasSize(2)))
          .andExpect(jsonPath("$.products[0].id").value(mouse))
          .andExpect(jsonPath("$.products[0].price").value("20.00"))
          .andExpect(jsonPath("$.products[1].id").value(keyboard))
          .andExpect(
              jsonPath("$.missingIds")
                  .value(org.hamcrest.Matchers.contains(MISSING_ID, "missing-1")));
    }

    @Test
    void allMissingIdsIsNotAnError() throws Exception {
      lookup("{\"ids\":[\"x\",\"y\"]}")
          .andExpect(status().isOk())
          .andExpect(jsonPath("$.products").isEmpty())
          .andExpect(jsonPath("$.missingIds").value(org.hamcrest.Matchers.contains("x", "y")));
    }

    @Test
    void acceptsExactlyHundredIds() throws Exception {
      lookup(idsJson(100))
          .andExpect(status().isOk())
          .andExpect(jsonPath("$.missingIds", hasSize(100)));
    }

    @Test
    void rejectsMoreThanHundredIds() throws Exception {
      lookup(idsJson(101))
          .andExpect(status().isBadRequest())
          .andExpect(jsonPath("$.code").value("VALIDATION_ERROR"))
          .andExpect(jsonPath("$.details[0].field").value("ids"));
    }

    @Test
    void rejectsEmptyMissingOrBlankIds() throws Exception {
      lookup("{\"ids\":[]}").andExpect(status().isBadRequest());
      lookup("{}").andExpect(status().isBadRequest());
      lookup("{\"ids\":[\"a\",\" \"]}")
          .andExpect(status().isBadRequest())
          .andExpect(jsonPath("$.details[0].field", endsWith("]")));
    }

    private ResultActions lookup(String json) throws Exception {
      return mockMvc.perform(
          post("/api/products/lookup").contentType(MediaType.APPLICATION_JSON).content(json));
    }

    private String idsJson(int count) {
      return Collections.nCopies(count, "").stream()
          .map(ignored -> "\"" + java.util.UUID.randomUUID() + "\"")
          .collect(Collectors.joining(",", "{\"ids\":[", "]}"));
    }
  }

  @Nested
  class Routing {

    @Test
    void unsupportedMethodReturns405() throws Exception {
      mockMvc
          .perform(
              org.springframework.test.web.servlet.request.MockMvcRequestBuilders.patch(
                  "/api/products/{id}", MISSING_ID))
          .andExpect(status().isMethodNotAllowed())
          .andExpect(jsonPath("$.code").value("METHOD_NOT_ALLOWED"));
    }

    @Test
    void unknownPathReturns404() throws Exception {
      mockMvc
          .perform(get("/api/unknown"))
          .andExpect(status().isNotFound())
          .andExpect(jsonPath("$.code").value("NOT_FOUND"));
    }
  }

  private ResultActions createProduct(String json) throws Exception {
    return mockMvc.perform(
        post("/api/products").contentType(MediaType.APPLICATION_JSON).content(json));
  }

  private String createAndGetId(String name, String price) throws Exception {
    String body =
        createProduct(product(name, "\"" + price + "\"", "PLN"))
            .andExpect(status().isCreated())
            .andReturn()
            .getResponse()
            .getContentAsString();
    return JsonPath.read(body, "$.id");
  }

  private String getJson(String id) throws Exception {
    return mockMvc
        .perform(get("/api/products/{id}", id))
        .andReturn()
        .getResponse()
        .getContentAsString();
  }

  /** {@code priceJson} is raw JSON, e.g. {@code "\"20.00\""} for a string or {@code 0.1}. */
  private static String product(String name, String priceJson, String currency) {
    return """
        {"name":"%s","price":%s,"currency":"%s","available":true}
        """
        .formatted(name, priceJson, currency);
  }
}
