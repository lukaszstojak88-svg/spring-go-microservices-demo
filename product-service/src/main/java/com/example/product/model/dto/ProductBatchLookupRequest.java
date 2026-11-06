package com.example.product.model.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Size;
import java.util.List;

public record ProductBatchLookupRequest(
    @Schema(description = "Product ids, 1 to 100 (duplicates are ignored)")
        @NotNull
        @Size(min = 1, max = 100)
        List<@NotBlank String> ids) {}
