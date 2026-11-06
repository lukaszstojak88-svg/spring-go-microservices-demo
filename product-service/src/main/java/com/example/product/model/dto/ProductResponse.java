package com.example.product.model.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import java.time.Instant;

public record ProductResponse(
    @Schema(example = "673abc1234567890abcdef12") String id,
    @Schema(example = "Mechanical Keyboard") String name,
    @Schema(example = "Compact mechanical keyboard") String description,
    @Schema(example = "349.99", description = "Decimal string scaled to the currency") String price,
    @Schema(example = "PLN") String currency,
    boolean available,
    @Schema(example = "2026-01-15T10:30:00Z") Instant createdAt,
    @Schema(example = "2026-01-15T10:30:00Z") Instant updatedAt) {}
