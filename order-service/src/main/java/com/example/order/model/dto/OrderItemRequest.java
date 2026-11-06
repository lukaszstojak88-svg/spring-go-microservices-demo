package com.example.order.model.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Positive;

/** An order line as sent by the client: product reference and quantity only, never a price. */
public record OrderItemRequest(
    @Schema(example = "673abc1234567890abcdef12") @NotBlank String productId,
    @Schema(example = "2") @NotNull @Positive Integer quantity) {}
