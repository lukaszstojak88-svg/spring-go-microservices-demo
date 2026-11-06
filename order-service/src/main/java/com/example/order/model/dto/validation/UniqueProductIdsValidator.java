package com.example.order.model.dto.validation;

import com.example.order.model.dto.CreateOrderRequest;
import com.example.order.model.dto.OrderItemRequest;
import jakarta.validation.ConstraintValidator;
import jakarta.validation.ConstraintValidatorContext;
import java.util.HashSet;
import java.util.LinkedHashSet;
import java.util.Objects;
import java.util.Set;

public class UniqueProductIdsValidator
    implements ConstraintValidator<UniqueProductIds, CreateOrderRequest> {

  @Override
  public boolean isValid(CreateOrderRequest request, ConstraintValidatorContext context) {
    if (request == null || request.items() == null) {
      return true; // @NotNull / @NotEmpty report missing items.
    }
    Set<String> seen = new HashSet<>();
    Set<String> duplicates = new LinkedHashSet<>();
    request.items().stream()
        .filter(Objects::nonNull)
        .map(OrderItemRequest::productId)
        .filter(id -> id != null && !id.isBlank())
        .forEach(
            id -> {
              if (!seen.add(id)) {
                duplicates.add(id);
              }
            });
    if (duplicates.isEmpty()) {
      return true;
    }
    context.disableDefaultConstraintViolation();
    context
        .buildConstraintViolationWithTemplate(
            "must not contain duplicate productId values: " + String.join(", ", duplicates))
        .addPropertyNode("items")
        .addConstraintViolation();
    return false;
  }
}
