package com.example.order.repository;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.example.order.model.entity.Order;
import com.example.order.model.entity.OrderItem;
import com.example.order.model.entity.OrderStatus;
import com.example.order.support.MongoContainerSupport;
import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;
import org.bson.Document;
import org.bson.types.Decimal128;
import org.bson.types.ObjectId;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.data.mongo.DataMongoTest;
import org.springframework.dao.OptimisticLockingFailureException;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageRequest;
import org.springframework.data.domain.Sort;
import org.springframework.data.mongodb.core.MongoTemplate;

/**
 * Persistence against a real MongoDB (Testcontainers), including Decimal128 money and snapshots.
 */
@DataMongoTest
class OrderRepositoryIT extends MongoContainerSupport {

  private static final Instant CREATED = Instant.parse("2026-01-15T10:30:00.123Z");

  @Autowired private OrderRepository repository;
  @Autowired private MongoTemplate mongoTemplate;

  @BeforeEach
  void cleanDatabase() {
    repository.deleteAll();
  }

  @Test
  void savesAndReadsOrderWithSnapshot() {
    Order saved = repository.save(order(OrderStatus.CREATED, CREATED));

    assertThat(ObjectId.isValid(saved.getId())).isTrue();
    Order found = repository.findById(saved.getId()).orElseThrow();
    assertThat(found.getCustomerId()).isEqualTo("customer-42");
    assertThat(found.getStatus()).isEqualTo(OrderStatus.CREATED);
    assertThat(found.getCurrency()).isEqualTo("PLN");
    assertThat(found.getTotalAmount()).isEqualTo(new BigDecimal("759.98"));
    assertThat(found.getCreatedAt()).isEqualTo(CREATED);
    assertThat(found.getUpdatedAt()).isEqualTo(CREATED);
    assertThat(found.getVersion()).isEqualTo(0L);

    assertThat(found.getItems()).hasSize(2);
    OrderItem first = found.getItems().get(0);
    assertThat(first.getProductId()).isEqualTo("p1");
    assertThat(first.getProductName()).isEqualTo("Mechanical Keyboard");
    assertThat(first.getUnitPrice()).isEqualTo(new BigDecimal("349.99"));
    assertThat(first.getCurrency()).isEqualTo("PLN");
    assertThat(first.getQuantity()).isEqualTo(2);
    assertThat(first.getLineTotal()).isEqualTo(new BigDecimal("699.98"));
    OrderItem second = found.getItems().get(1);
    assertThat(second.getProductId()).isEqualTo("p2");
    assertThat(second.getUnitPrice()).isEqualTo(new BigDecimal("20.00"));
    assertThat(second.getLineTotal()).isEqualTo(new BigDecimal("60.00"));
  }

  @Test
  void returnsEmptyForUnknownId() {
    assertThat(repository.findById(new ObjectId().toHexString())).isEmpty();
  }

  @Test
  void storesMoneyAsDecimal128NotAsString() {
    Order saved = repository.save(order(OrderStatus.CREATED, CREATED));

    Document raw =
        mongoTemplate
            .getCollection("orders")
            .find(new Document("_id", new ObjectId(saved.getId())))
            .first();

    assertThat(raw).isNotNull();
    assertThat(raw.get("totalAmount")).isInstanceOf(Decimal128.class);
    assertThat(((Decimal128) raw.get("totalAmount")).bigDecimalValue())
        .isEqualTo(new BigDecimal("759.98"));
    List<Document> items = raw.getList("items", Document.class);
    assertThat(items).hasSize(2);
    for (Document item : items) {
      assertThat(item.get("unitPrice")).isInstanceOf(Decimal128.class);
      assertThat(item.get("lineTotal")).isInstanceOf(Decimal128.class);
    }
  }

  @Test
  void roundTripsDecimalValuesExactlyIncludingScale() {
    Order order = order(OrderStatus.CREATED, CREATED);
    order.setTotalAmount(new BigDecimal("12345678901234.56"));
    order.getItems().get(0).setUnitPrice(new BigDecimal("0.10"));
    order.getItems().get(0).setLineTotal(new BigDecimal("0.30"));
    order.getItems().get(1).setUnitPrice(new BigDecimal("1500"));
    order.getItems().get(1).setLineTotal(new BigDecimal("4500"));

    Order found = repository.findById(repository.save(order).getId()).orElseThrow();

    // BigDecimal.equals compares scale too: 0.10 must not come back as 0.1.
    assertThat(found.getTotalAmount()).isEqualTo(new BigDecimal("12345678901234.56"));
    assertThat(found.getItems().get(0).getUnitPrice()).isEqualTo(new BigDecimal("0.10"));
    assertThat(found.getItems().get(0).getLineTotal()).isEqualTo(new BigDecimal("0.30"));
    assertThat(found.getItems().get(1).getUnitPrice()).isEqualTo(new BigDecimal("1500"));
    assertThat(found.getItems().get(1).getLineTotal()).isEqualTo(new BigDecimal("4500"));
  }

  @Test
  void snapshotIsStoredInsideTheOrderDocument() {
    Order saved = repository.save(order(OrderStatus.CREATED, CREATED));

    assertThat(mongoTemplate.getCollectionNames()).contains("orders").doesNotContain("products");
    Document raw =
        mongoTemplate
            .getCollection("orders")
            .find(new Document("_id", new ObjectId(saved.getId())))
            .first();
    Document firstItem = raw.getList("items", Document.class).getFirst();
    assertThat(firstItem.getString("productId")).isEqualTo("p1");
    assertThat(firstItem.getString("productName")).isEqualTo("Mechanical Keyboard");
    assertThat(firstItem.getString("currency")).isEqualTo("PLN");
    assertThat(firstItem.getInteger("quantity")).isEqualTo(2);
  }

  @Test
  void updatesStatusAndIncrementsVersion() {
    Order saved = repository.save(order(OrderStatus.CREATED, CREATED));
    Instant updatedAt = CREATED.plusSeconds(60);

    saved.setStatus(OrderStatus.CANCELLED);
    saved.setUpdatedAt(updatedAt);
    repository.save(saved);

    Order found = repository.findById(saved.getId()).orElseThrow();
    assertThat(found.getStatus()).isEqualTo(OrderStatus.CANCELLED);
    assertThat(found.getCreatedAt()).isEqualTo(CREATED);
    assertThat(found.getUpdatedAt()).isEqualTo(updatedAt);
    assertThat(found.getVersion()).isEqualTo(1L);
    assertThat(found.getItems()).hasSize(2);
    assertThat(repository.count()).isEqualTo(1);
  }

  @Test
  void rejectsConcurrentUpdateWithOptimisticLocking() {
    Order saved = repository.save(order(OrderStatus.CREATED, CREATED));
    Order first = repository.findById(saved.getId()).orElseThrow();
    Order second = repository.findById(saved.getId()).orElseThrow();

    first.setStatus(OrderStatus.CANCELLED);
    repository.save(first);
    second.setStatus(OrderStatus.CANCELLED);

    assertThatThrownBy(() -> repository.save(second))
        .isInstanceOf(OptimisticLockingFailureException.class);
  }

  @Test
  void paginatesWithSortAndReportsTotals() {
    for (int i = 0; i < 5; i++) {
      repository.save(order(OrderStatus.CREATED, CREATED.plusSeconds(i)));
    }
    Sort newestFirst = Sort.by(Sort.Direction.DESC, "createdAt");

    Page<Order> first = repository.findAll(PageRequest.of(0, 2, newestFirst));
    Page<Order> last = repository.findAll(PageRequest.of(2, 2, newestFirst));
    Page<Order> beyond = repository.findAll(PageRequest.of(5, 2, newestFirst));

    assertThat(first.getTotalElements()).isEqualTo(5);
    assertThat(first.getTotalPages()).isEqualTo(3);
    assertThat(first.getContent())
        .extracting(Order::getCreatedAt)
        .containsExactly(CREATED.plusSeconds(4), CREATED.plusSeconds(3));
    assertThat(last.getContent())
        .extracting(Order::getCreatedAt)
        .containsExactly(CREATED.plusSeconds(0));
    assertThat(beyond.getContent()).isEmpty();
    assertThat(beyond.getTotalElements()).isEqualTo(5);
  }

  @Test
  void sortsByTotalAmountNumerically() {
    // Decimal128 sorts numerically; a string representation would sort "9.00" after "100.00".
    for (String total : List.of("9.00", "100.00", "25.50")) {
      Order order = order(OrderStatus.CREATED, CREATED);
      order.setTotalAmount(new BigDecimal(total));
      repository.save(order);
    }

    Page<Order> page =
        repository.findAll(PageRequest.of(0, 10, Sort.by(Sort.Direction.ASC, "totalAmount")));

    assertThat(page.getContent())
        .extracting(Order::getTotalAmount)
        .containsExactly(new BigDecimal("9.00"), new BigDecimal("25.50"), new BigDecimal("100.00"));
  }

  @Test
  void filtersByStatusWithPagination() {
    for (int i = 0; i < 3; i++) {
      repository.save(order(OrderStatus.CREATED, CREATED.plusSeconds(i)));
    }
    for (int i = 0; i < 2; i++) {
      repository.save(order(OrderStatus.CANCELLED, CREATED.plusSeconds(10 + i)));
    }
    Sort newestFirst = Sort.by(Sort.Direction.DESC, "createdAt");

    Page<Order> created =
        repository.findByStatus(OrderStatus.CREATED, PageRequest.of(0, 2, newestFirst));
    Page<Order> cancelled =
        repository.findByStatus(OrderStatus.CANCELLED, PageRequest.of(0, 10, newestFirst));

    assertThat(created.getTotalElements()).isEqualTo(3);
    assertThat(created.getTotalPages()).isEqualTo(2);
    assertThat(created.getContent()).hasSize(2);
    assertThat(created.getContent()).allMatch(o -> o.getStatus() == OrderStatus.CREATED);
    assertThat(cancelled.getTotalElements()).isEqualTo(2);
    assertThat(cancelled.getContent()).allMatch(o -> o.getStatus() == OrderStatus.CANCELLED);
  }

  @Test
  void statusFilterReturnsEmptyPageWhenNothingMatches() {
    repository.save(order(OrderStatus.CREATED, CREATED));

    Page<Order> cancelled = repository.findByStatus(OrderStatus.CANCELLED, PageRequest.of(0, 10));

    assertThat(cancelled.getContent()).isEmpty();
    assertThat(cancelled.getTotalElements()).isZero();
  }

  private static Order order(OrderStatus status, Instant createdAt) {
    OrderItem keyboard = new OrderItem();
    keyboard.setProductId("p1");
    keyboard.setProductName("Mechanical Keyboard");
    keyboard.setUnitPrice(new BigDecimal("349.99"));
    keyboard.setCurrency("PLN");
    keyboard.setQuantity(2);
    keyboard.setLineTotal(new BigDecimal("699.98"));

    OrderItem mouse = new OrderItem();
    mouse.setProductId("p2");
    mouse.setProductName("Mouse");
    mouse.setUnitPrice(new BigDecimal("20.00"));
    mouse.setCurrency("PLN");
    mouse.setQuantity(3);
    mouse.setLineTotal(new BigDecimal("60.00"));

    Order order = new Order();
    order.setCustomerId("customer-42");
    order.setStatus(status);
    order.setItems(new java.util.ArrayList<>(List.of(keyboard, mouse)));
    order.setTotalAmount(new BigDecimal("759.98"));
    order.setCurrency("PLN");
    order.setCreatedAt(createdAt);
    order.setUpdatedAt(createdAt);
    return order;
  }
}
