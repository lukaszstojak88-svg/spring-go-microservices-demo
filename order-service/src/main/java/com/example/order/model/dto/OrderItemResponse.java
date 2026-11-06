package com.example.order.model.dto;

import io.swagger.v3.oas.annotations.media.Schema;

/** Product snapshot stored in the order at creation time. Money as decimal strings. */
public record OrderItemResponse(
    @Schema(example = "673abc1234567890abcdef12") String productId,
    @Schema(example = "Mechanical Keyboard") String productName,
    @Schema(example = "349.99") String unitPrice,
    @Schema(example = "PLN") String currency,
    @Schema(example = "2") int quantity,
    @Schema(example = "699.98") String lineTotal) {}
