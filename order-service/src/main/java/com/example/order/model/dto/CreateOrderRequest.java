package com.example.order.model.dto;

import com.example.order.model.dto.validation.UniqueProductIds;
import io.swagger.v3.oas.annotations.media.Schema;
import jakarta.validation.Valid;
import jakarta.validation.constraints.NotEmpty;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Size;
import java.util.List;

/**
 * Order creation request. It contains no prices: prices always come from the Product Service.
 * Duplicate product ids are rejected (not merged).
 */
@UniqueProductIds
public record CreateOrderRequest(
    @Schema(example = "customer-42", description = "Optional opaque customer reference")
        @Size(max = 100)
        String customerId,
    @Schema(description = "1 to 100 items, each product at most once")
        @NotNull
        @NotEmpty
        @Size(max = 100)
        List<@NotNull @Valid OrderItemRequest> items) {}
