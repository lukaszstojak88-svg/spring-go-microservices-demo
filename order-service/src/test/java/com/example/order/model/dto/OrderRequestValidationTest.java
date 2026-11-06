package com.example.order.model.dto;

import static org.assertj.core.api.Assertions.assertThat;

import jakarta.validation.ConstraintViolation;
import jakarta.validation.Validation;
import jakarta.validation.Validator;
import jakarta.validation.ValidatorFactory;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.stream.Collectors;
import java.util.stream.IntStream;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

class OrderRequestValidationTest {

  private static ValidatorFactory factory;
  private static Validator validator;

  @BeforeAll
  static void setUpValidator() {
    factory = Validation.buildDefaultValidatorFactory();
    validator = factory.getValidator();
  }

  @AfterAll
  static void closeValidator() {
    factory.close();
  }

  @Test
  void acceptsValidRequestWithAndWithoutCustomer() {
    assertThat(validate(request("customer-42", item("p1", 2), item("p2", 1)))).isEmpty();
    assertThat(validate(request(null, item("p1", 1)))).isEmpty();
  }

  @Test
  void rejectsMissingOrEmptyItems() {
    assertThat(validate(new CreateOrderRequest("c", null))).containsKey("items");
    assertThat(validate(new CreateOrderRequest("c", List.of()))).containsKey("items");
  }

  @Test
  void rejectsMoreThanHundredItems() {
    OrderItemRequest[] items =
        IntStream.rangeClosed(1, 101)
            .mapToObj(i -> item("p" + i, 1))
            .toArray(OrderItemRequest[]::new);

    assertThat(validate(request("c", items))).containsKey("items");
    assertThat(validate(request("c", Arrays.copyOf(items, 100)))).isEmpty();
  }

  @ParameterizedTest
  @ValueSource(ints = {0, -1, -100})
  void rejectsNonPositiveQuantity(int quantity) {
    assertThat(validate(request("c", item("p1", quantity)))).containsKey("items[0].quantity");
  }

  @Test
  void rejectsMissingQuantityAndProductId() {
    Map<String, String> violations = validate(request("c", new OrderItemRequest(" ", null)));

    assertThat(violations).containsKeys("items[0].productId", "items[0].quantity");
  }

  @Test
  void rejectsNullItem() {
    List<OrderItemRequest> items = new ArrayList<>();
    items.add(null);

    assertThat(validate(new CreateOrderRequest("c", items))).containsKey("items[0].<list element>");
  }

  @Test
  void rejectsDuplicateProductIds() {
    Map<String, String> violations =
        validate(request("c", item("p1", 1), item("p2", 1), item("p1", 3), item("p2", 1)));

    assertThat(violations)
        .containsEntry("items", "must not contain duplicate productId values: p1, p2");
  }

  @Test
  void rejectsTooLongCustomerId() {
    assertThat(validate(request("c".repeat(101), item("p1", 1)))).containsKey("customerId");
  }

  private static OrderItemRequest item(String productId, Integer quantity) {
    return new OrderItemRequest(productId, quantity);
  }

  private static CreateOrderRequest request(String customerId, OrderItemRequest... items) {
    return new CreateOrderRequest(customerId, List.of(items));
  }

  private static Map<String, String> validate(CreateOrderRequest request) {
    Set<ConstraintViolation<CreateOrderRequest>> violations = validator.validate(request);
    return violations.stream()
        .collect(
            Collectors.toMap(
                v -> v.getPropertyPath().toString(),
                ConstraintViolation::getMessage,
                (first, second) -> first));
  }
}
