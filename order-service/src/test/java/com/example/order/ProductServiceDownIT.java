package com.example.order;

import static org.assertj.core.api.Assertions.assertThat;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

import com.example.order.model.entity.Order;
import com.example.order.model.entity.OrderItem;
import com.example.order.model.entity.OrderStatus;
import com.example.order.repository.OrderRepository;
import com.example.order.support.MongoContainerSupport;
import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.http.MediaType;
import org.springframework.test.context.TestPropertySource;
import org.springframework.test.web.servlet.MockMvc;

/**
 * The Product Service address is a closed port (connection refused). The Order Service must stay
 * READY and keep serving everything that does not need the Product Service; only order creation
 * fails, with 503 and nothing persisted.
 */
@SpringBootTest
@AutoConfigureMockMvc
@TestPropertySource(
    properties = {
      "product-service.base-url=http://127.0.0.1:1",
      "product-service.connect-timeout=500ms",
      "product-service.read-timeout=500ms",
      "management.endpoint.health.show-components=always"
    })
class ProductServiceDownIT extends MongoContainerSupport {

  @Autowired private MockMvc mockMvc;
  @Autowired private OrderRepository repository;

  @BeforeEach
  void cleanDatabase() {
    repository.deleteAll();
  }

  @Test
  void staysReadyWhenProductServiceIsUnavailable() throws Exception {
    mockMvc
        .perform(get("/actuator/health/readiness"))
        .andExpect(status().isOk())
        .andExpect(jsonPath("$.status").value("UP"));
    mockMvc
        .perform(get("/actuator/health/liveness"))
        .andExpect(status().isOk())
        .andExpect(jsonPath("$.status").value("UP"));
  }

  @Test
  void readinessDependsOnOwnMongoDbOnly() throws Exception {
    mockMvc
        .perform(get("/actuator/health/readiness"))
        .andExpect(status().isOk())
        .andExpect(jsonPath("$.components.mongo.status").value("UP"))
        .andExpect(jsonPath("$.components.productService").doesNotExist())
        .andExpect(jsonPath("$.components.product").doesNotExist());
  }

  @Test
  void orderCreationFailsWith503AndPersistsNothing() throws Exception {
    mockMvc
        .perform(
            post("/api/orders")
                .contentType(MediaType.APPLICATION_JSON)
                .content("{\"items\":[{\"productId\":\"p1\",\"quantity\":1}]}"))
        .andExpect(status().isServiceUnavailable())
        .andExpect(jsonPath("$.code").value("PRODUCT_SERVICE_UNAVAILABLE"));

    assertThat(repository.count()).isZero();
  }

  @Test
  void readsAndListsOrdersWithoutProductService() throws Exception {
    String id = repository.save(storedOrder(OrderStatus.CREATED)).getId();

    mockMvc
        .perform(get("/api/orders/{id}", id))
        .andExpect(status().isOk())
        .andExpect(jsonPath("$.id").value(id))
        .andExpect(jsonPath("$.items[0].productName").value("Keyboard (snapshot)"))
        .andExpect(jsonPath("$.items[0].unitPrice").value("349.99"))
        .andExpect(jsonPath("$.totalAmount").value("699.98"));
    mockMvc
        .perform(get("/api/orders"))
        .andExpect(status().isOk())
        .andExpect(jsonPath("$.totalElements").value(1));
  }

  @Test
  void cancelsOrderWithoutProductService() throws Exception {
    String id = repository.save(storedOrder(OrderStatus.CREATED)).getId();

    mockMvc
        .perform(post("/api/orders/{id}/cancel", id))
        .andExpect(status().isOk())
        .andExpect(jsonPath("$.status").value("CANCELLED"));
    mockMvc.perform(post("/api/orders/{id}/cancel", id)).andExpect(status().isConflict());
  }

  private static Order storedOrder(OrderStatus status) {
    OrderItem item = new OrderItem();
    item.setProductId("p1");
    item.setProductName("Keyboard (snapshot)");
    item.setUnitPrice(new BigDecimal("349.99"));
    item.setCurrency("PLN");
    item.setQuantity(2);
    item.setLineTotal(new BigDecimal("699.98"));

    Order order = new Order();
    order.setCustomerId("customer-42");
    order.setStatus(status);
    order.setItems(List.of(item));
    order.setTotalAmount(new BigDecimal("699.98"));
    order.setCurrency("PLN");
    order.setCreatedAt(Instant.parse("2026-01-15T10:30:00.123Z"));
    order.setUpdatedAt(Instant.parse("2026-01-15T10:30:00.123Z"));
    return order;
  }
}
