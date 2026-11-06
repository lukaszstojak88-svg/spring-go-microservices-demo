package com.example.product.model.dto;

import com.example.product.model.dto.validation.MoneyHolder;
import com.example.product.model.dto.validation.ValidCurrency;
import com.example.product.model.dto.validation.ValidMoney;
import io.swagger.v3.oas.annotations.media.Schema;
import jakarta.validation.constraints.DecimalMin;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Size;
import java.math.BigDecimal;

@ValidMoney
public record CreateProductRequest(
    @Schema(example = "Mechanical Keyboard") @NotBlank @Size(max = 200) String name,
    @Schema(example = "Compact mechanical keyboard") @Size(max = 2000) String description,
    @Schema(
            type = "string",
            example = "349.99",
            description = "Decimal string; fraction digits limited by the currency")
        @NotNull
        @DecimalMin("0")
        BigDecimal price,
    @Schema(example = "PLN") @NotNull @ValidCurrency String currency,
    @Schema(example = "true") @NotNull Boolean available)
    implements MoneyHolder {}
