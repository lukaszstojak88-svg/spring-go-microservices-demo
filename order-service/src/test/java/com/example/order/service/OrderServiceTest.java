package com.example.order.service;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;

import com.example.order.client.ProductClient;
import com.example.order.client.ProductData;
import com.example.order.client.ProductLookupResponse;
import com.example.order.client.ProductServiceUnavailableException;
import com.example.order.exception.ApiError.FieldDetail;
import com.example.order.exception.InvalidSortException;
import com.example.order.exception.OrderConflictException;
import com.example.order.exception.OrderNotFoundException;
import com.example.order.exception.OrderRuleViolationException;
import com.example.order.exception.OrderRuleViolationException.Code;
import com.example.order.mapper.OrderMapperImpl;
import com.example.order.model.dto.CreateOrderRequest;
import com.example.order.model.dto.OrderItemRequest;
import com.example.order.model.dto.OrderItemResponse;
import com.example.order.model.dto.OrderResponse;
import com.example.order.model.dto.PaginatedResponse;
import com.example.order.model.entity.Order;
import com.example.order.model.entity.OrderItem;
import com.example.order.model.entity.OrderStatus;
import com.example.order.repository.OrderRepository;
import java.math.BigDecimal;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.List;
import java.util.Optional;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageImpl;
import org.springframework.data.domain.PageRequest;
import org.springframework.data.domain.Pageable;
import org.springframework.data.domain.Sort;

/**
 * Service-layer unit tests with a mocked {@link ProductClient} and repository; the real
 * MapStruct-generated mapper is used. No Docker or Spring context required.
 */
@ExtendWith(MockitoExtension.class)
class OrderServiceTest {

  private static final Instant NOW = Instant.parse("2026-01-15T11:00:00.987654321Z");
  private static final Instant NOW_MILLIS = Instant.parse("2026-01-15T11:00:00.987Z");
  private static final Instant CREATED = Instant.parse("2026-01-10T09:00:00Z");

  @Mock private OrderRepository repository;
  @Mock private ProductClient productClient;

  private OrderService service;

  @BeforeEach
  void setUp() {
    service =
        new OrderService(
            repository, productClient, new OrderMapperImpl(), Clock.fixed(NOW, ZoneOffset.UTC));
  }

  @Nested
  class Create {

    @Test
    void buildsSnapshotComputesTotalsAndSaves() {
      lookupReturns(
          List.of(
              product("p1", "Mechanical Keyboard", "349.99", "PLN", true),
              product("p2", "Mouse", "20.00", "PLN", true)),
          List.of());
      when(repository.save(any(Order.class))).thenAnswer(inv -> withId(inv.getArgument(0)));

      OrderResponse response = service.create(request("customer-42", item("p1", 2), item("p2", 3)));

      Order saved = captureSaved();
      assertThat(saved.getStatus()).isEqualTo(OrderStatus.CREATED);
      assertThat(saved.getCustomerId()).isEqualTo("customer-42");
      assertThat(saved.getCurrency()).isEqualTo("PLN");
      assertThat(saved.getTotalAmount()).isEqualTo(new BigDecimal("759.98"));
      assertThat(saved.getCreatedAt()).isEqualTo(NOW_MILLIS);
      assertThat(saved.getUpdatedAt()).isEqualTo(NOW_MILLIS);

      OrderItem first = saved.getItems().get(0);
      assertThat(first.getProductId()).isEqualTo("p1");
      assertThat(first.getProductName()).isEqualTo("Mechanical Keyboard");
      assertThat(first.getUnitPrice()).isEqualTo(new BigDecimal("349.99"));
      assertThat(first.getCurrency()).isEqualTo("PLN");
      assertThat(first.getQuantity()).isEqualTo(2);
      assertThat(first.getLineTotal()).isEqualTo(new BigDecimal("699.98"));
      assertThat(saved.getItems().get(1).getLineTotal()).isEqualTo(new BigDecimal("60.00"));

      assertThat(response.id()).isEqualTo("o1");
      assertThat(response.totalAmount()).isEqualTo("759.98");
      assertThat(response.items())
          .extracting(OrderItemResponse::lineTotal)
          .containsExactly("699.98", "60.00");
    }

    @Test
    void callsProductServiceOnceWithAllIdsInRequestOrder() {
      lookupReturns(
          List.of(product("b", "B", "1.00", "PLN", true), product("a", "A", "2.00", "PLN", true)),
          List.of());
      when(repository.save(any(Order.class))).thenAnswer(inv -> withId(inv.getArgument(0)));

      service.create(request(null, item("a", 1), item("b", 1)));

      verify(productClient).lookup(List.of("a", "b"));
    }

    @Test
    void computesExactlyWithoutFloatingPointErrors() {
      // 0.10 * 3 = 0.30 exactly (as double it would be 0.30000000000000004).
      lookupReturns(List.of(product("p1", "Item", "0.10", "PLN", true)), List.of());
      when(repository.save(any(Order.class))).thenAnswer(inv -> withId(inv.getArgument(0)));

      OrderResponse response = service.create(request(null, item("p1", 3)));

      assertThat(response.totalAmount()).isEqualTo("0.30");
      assertThat(response.items().getFirst().unitPrice()).isEqualTo("0.10");
    }

    @Test
    void normalizesUnitPriceToCurrencyScale() {
      lookupReturns(List.of(product("p1", "Item", "20", "PLN", true)), List.of());
      when(repository.save(any(Order.class))).thenAnswer(inv -> withId(inv.getArgument(0)));

      OrderResponse response = service.create(request(null, item("p1", 2)));

      assertThat(response.items().getFirst().unitPrice()).isEqualTo("20.00");
      assertThat(response.totalAmount()).isEqualTo("40.00");
    }

    @Test
    void usesCurrencyWithoutMinorUnit() {
      lookupReturns(List.of(product("p1", "Item", "1500", "JPY", true)), List.of());
      when(repository.save(any(Order.class))).thenAnswer(inv -> withId(inv.getArgument(0)));

      OrderResponse response = service.create(request(null, item("p1", 3)));

      assertThat(response.totalAmount()).isEqualTo("4500");
      assertThat(response.currency()).isEqualTo("JPY");
    }

    @Test
    void ignoresUnrequestedProductsReturnedByLookup() {
      lookupReturns(
          List.of(
              product("p1", "Item", "1.00", "PLN", true),
              product("other", "Other", "5.00", "EUR", true)),
          List.of());
      when(repository.save(any(Order.class))).thenAnswer(inv -> withId(inv.getArgument(0)));

      OrderResponse response = service.create(request(null, item("p1", 1)));

      assertThat(response.items()).hasSize(1);
      assertThat(response.totalAmount()).isEqualTo("1.00");
    }

    @Test
    void rejectsMissingProductsAndSavesNothing() {
      lookupReturns(List.of(product("p1", "A", "1.00", "PLN", true)), List.of("p2", "p3"));

      assertThatThrownBy(
              () -> service.create(request(null, item("p1", 1), item("p2", 1), item("p3", 1))))
          .isInstanceOfSatisfying(
              OrderRuleViolationException.class,
              ex -> {
                assertThat(ex.getCode()).isEqualTo(Code.PRODUCT_NOT_FOUND);
                assertThat(ex.getMessage()).isEqualTo("Product(s) not found: p2, p3");
                assertThat(ex.getDetails())
                    .extracting(FieldDetail::field)
                    .containsExactly("items[1].productId", "items[2].productId");
              });
      verify(repository, never()).save(any());
    }

    @Test
    void rejectsUnavailableProductsAndSavesNothing() {
      lookupReturns(
          List.of(
              product("p1", "A", "1.00", "PLN", true), product("p2", "B", "2.00", "PLN", false)),
          List.of());

      assertThatThrownBy(() -> service.create(request(null, item("p1", 1), item("p2", 1))))
          .isInstanceOfSatisfying(
              OrderRuleViolationException.class,
              ex -> {
                assertThat(ex.getCode()).isEqualTo(Code.PRODUCT_UNAVAILABLE);
                assertThat(ex.getMessage()).isEqualTo("Product(s) not available: p2");
                assertThat(ex.getDetails())
                    .extracting(FieldDetail::field)
                    .containsExactly("items[1].productId");
              });
      verify(repository, never()).save(any());
    }

    @Test
    void reportsMissingBeforeUnavailable() {
      lookupReturns(List.of(product("p1", "A", "1.00", "PLN", false)), List.of("p2"));

      assertThatThrownBy(() -> service.create(request(null, item("p1", 1), item("p2", 1))))
          .isInstanceOfSatisfying(
              OrderRuleViolationException.class,
              ex -> assertThat(ex.getCode()).isEqualTo(Code.PRODUCT_NOT_FOUND));
    }

    @Test
    void rejectsMixedCurrenciesAndSavesNothing() {
      lookupReturns(
          List.of(
              product("p1", "A", "1.00", "PLN", true),
              product("p2", "B", "2.00", "EUR", true),
              product("p3", "C", "3.00", "PLN", true)),
          List.of());

      assertThatThrownBy(
              () -> service.create(request(null, item("p1", 1), item("p2", 1), item("p3", 1))))
          .isInstanceOfSatisfying(
              OrderRuleViolationException.class,
              ex -> {
                assertThat(ex.getCode()).isEqualTo(Code.CURRENCY_MISMATCH);
                assertThat(ex.getDetails())
                    .extracting(FieldDetail::field)
                    .containsExactly("items[1].productId");
                assertThat(ex.getDetails().getFirst().message())
                    .isEqualTo("Product p2 is priced in EUR, expected PLN");
              });
      verify(repository, never()).save(any());
    }

    @Test
    void propagatesProductServiceFailureAndSavesNothing() {
      when(productClient.lookup(any()))
          .thenThrow(new ProductServiceUnavailableException("timeout"));

      assertThatThrownBy(() -> service.create(request(null, item("p1", 1))))
          .isInstanceOf(ProductServiceUnavailableException.class);
      verify(repository, never()).save(any());
    }

    @Test
    void treatsInvalidProductPriceScaleAsIntegrationError() {
      lookupReturns(List.of(product("p1", "A", "1.999", "PLN", true)), List.of());

      assertThatThrownBy(() -> service.create(request(null, item("p1", 1))))
          .isInstanceOf(ProductServiceUnavailableException.class);
      verify(repository, never()).save(any());
    }

    @Test
    void treatsUnknownProductCurrencyAsIntegrationError() {
      lookupReturns(List.of(product("p1", "A", "1.00", "ZZZ", true)), List.of());

      assertThatThrownBy(() -> service.create(request(null, item("p1", 1))))
          .isInstanceOf(ProductServiceUnavailableException.class);
      verify(repository, never()).save(any());
    }
  }

  @Nested
  class Get {

    @Test
    void returnsStoredSnapshotWithoutCallingProductService() {
      when(repository.findById("o1")).thenReturn(Optional.of(storedOrder(OrderStatus.CREATED)));

      OrderResponse response = service.get("o1");

      assertThat(response.id()).isEqualTo("o1");
      assertThat(response.items().getFirst().productName()).isEqualTo("Keyboard (old name)");
      assertThat(response.items().getFirst().unitPrice()).isEqualTo("300.00");
      assertThat(response.totalAmount()).isEqualTo("600.00");
      verifyNoInteractions(productClient);
    }

    @Test
    void throwsWhenOrderIsMissing() {
      when(repository.findById("missing")).thenReturn(Optional.empty());

      assertThatThrownBy(() -> service.get("missing"))
          .isInstanceOf(OrderNotFoundException.class)
          .hasMessage("Order missing not found");
    }
  }

  @Nested
  class Cancel {

    @Test
    void cancelsCreatedOrder() {
      when(repository.findById("o1")).thenReturn(Optional.of(storedOrder(OrderStatus.CREATED)));
      when(repository.save(any(Order.class))).thenAnswer(inv -> inv.getArgument(0));

      OrderResponse response = service.cancel("o1");

      Order saved = captureSaved();
      assertThat(saved.getStatus()).isEqualTo(OrderStatus.CANCELLED);
      assertThat(saved.getUpdatedAt()).isEqualTo(NOW_MILLIS);
      assertThat(saved.getCreatedAt()).isEqualTo(CREATED);
      assertThat(saved.getTotalAmount()).isEqualTo(new BigDecimal("600.00"));
      assertThat(response.status()).isEqualTo(OrderStatus.CANCELLED);
      verifyNoInteractions(productClient);
    }

    @Test
    void cancellingTwiceIsAConflict() {
      when(repository.findById("o1")).thenReturn(Optional.of(storedOrder(OrderStatus.CANCELLED)));

      assertThatThrownBy(() -> service.cancel("o1"))
          .isInstanceOf(OrderConflictException.class)
          .hasMessage("Order o1 is already cancelled");
      verify(repository, never()).save(any());
    }

    @Test
    void throwsWhenOrderIsMissing() {
      when(repository.findById("missing")).thenReturn(Optional.empty());

      assertThatThrownBy(() -> service.cancel("missing"))
          .isInstanceOf(OrderNotFoundException.class);
    }
  }

  @Nested
  class ListOrders {

    @Test
    void listsAllWithoutStatusFilter() {
      ArgumentCaptor<Pageable> pageable = ArgumentCaptor.forClass(Pageable.class);
      when(repository.findAll(pageable.capture())).thenReturn(page(1, 2, 5));

      PaginatedResponse<OrderResponse> response = service.list(1, 2, null, null);

      assertThat(pageable.getValue().getSort())
          .isEqualTo(Sort.by(Sort.Order.desc("createdAt"), Sort.Order.asc("id")));
      assertThat(response.page()).isEqualTo(1);
      assertThat(response.size()).isEqualTo(2);
      assertThat(response.totalElements()).isEqualTo(5);
      assertThat(response.totalPages()).isEqualTo(3);
      verify(repository, never()).findByStatus(any(), any());
    }

    @Test
    void filtersByStatus() {
      when(repository.findByStatus(any(), any())).thenReturn(page(0, 20, 1));

      PaginatedResponse<OrderResponse> response =
          service.list(0, 20, OrderStatus.CANCELLED, "totalAmount,asc");

      ArgumentCaptor<Pageable> pageable = ArgumentCaptor.forClass(Pageable.class);
      verify(repository)
          .findByStatus(org.mockito.ArgumentMatchers.eq(OrderStatus.CANCELLED), pageable.capture());
      assertThat(pageable.getValue().getSort())
          .isEqualTo(Sort.by(Sort.Order.asc("totalAmount"), Sort.Order.asc("id")));
      assertThat(response.content()).hasSize(1);
    }

    @ParameterizedTest
    @ValueSource(strings = {"status,asc", "customerId", "createdAt,up", "createdAt,asc,x"})
    void rejectsInvalidSort(String sort) {
      assertThatThrownBy(() -> OrderService.parseSort(sort))
          .isInstanceOf(InvalidSortException.class);
    }

    private Page<Order> page(int page, int size, long total) {
      return new PageImpl<>(
          List.of(storedOrder(OrderStatus.CREATED)), PageRequest.of(page, size), total);
    }
  }

  private void lookupReturns(List<ProductData> products, List<String> missingIds) {
    when(productClient.lookup(any())).thenReturn(new ProductLookupResponse(products, missingIds));
  }

  private Order captureSaved() {
    ArgumentCaptor<Order> captor = ArgumentCaptor.forClass(Order.class);
    verify(repository).save(captor.capture());
    return captor.getValue();
  }

  private static Order withId(Order order) {
    order.setId("o1");
    return order;
  }

  private static ProductData product(
      String id, String name, String price, String currency, boolean available) {
    return new ProductData(id, name, new BigDecimal(price), currency, available);
  }

  private static OrderItemRequest item(String productId, int quantity) {
    return new OrderItemRequest(productId, quantity);
  }

  private static CreateOrderRequest request(String customerId, OrderItemRequest... items) {
    return new CreateOrderRequest(customerId, List.of(items));
  }

  private static Order storedOrder(OrderStatus status) {
    OrderItem item = new OrderItem();
    item.setProductId("p1");
    item.setProductName("Keyboard (old name)");
    item.setUnitPrice(new BigDecimal("300.00"));
    item.setCurrency("PLN");
    item.setQuantity(2);
    item.setLineTotal(new BigDecimal("600.00"));

    Order order = new Order();
    order.setId("o1");
    order.setCustomerId("customer-42");
    order.setStatus(status);
    order.setItems(List.of(item));
    order.setTotalAmount(new BigDecimal("600.00"));
    order.setCurrency("PLN");
    order.setCreatedAt(CREATED);
    order.setUpdatedAt(CREATED);
    order.setVersion(0L);
    return order;
  }
}
