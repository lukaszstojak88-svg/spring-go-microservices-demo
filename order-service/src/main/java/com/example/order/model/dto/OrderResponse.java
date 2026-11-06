package com.example.order.model.dto;

import com.example.order.model.entity.OrderStatus;
import io.swagger.v3.oas.annotations.media.Schema;
import java.time.Instant;
import java.util.List;

public record OrderResponse(
    @Schema(example = "674def4567890abcdef12345") String id,
    @Schema(example = "customer-42") String customerId,
    @Schema(example = "CREATED") OrderStatus status,
    List<OrderItemResponse> items,
    @Schema(example = "699.98", description = "Decimal string") String totalAmount,
    @Schema(example = "PLN") String currency,
    @Schema(example = "2026-01-15T11:00:00Z") Instant createdAt,
    @Schema(example = "2026-01-15T11:00:00Z") Instant updatedAt) {}
