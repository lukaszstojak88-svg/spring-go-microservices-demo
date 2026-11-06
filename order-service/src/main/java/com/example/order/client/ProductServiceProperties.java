package com.example.order.client;

import jakarta.validation.constraints.NotNull;
import java.net.URI;
import java.time.Duration;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.validation.annotation.Validated;

/**
 * Product Service client settings ({@code product-service.*}), bound from {@code
 * PRODUCT_SERVICE_URL}, {@code PRODUCT_CLIENT_CONNECT_TIMEOUT} and {@code
 * PRODUCT_CLIENT_READ_TIMEOUT}. Timeouts are mandatory and explicit.
 */
@Validated
@ConfigurationProperties("product-service")
public record ProductServiceProperties(
    @NotNull URI baseUrl, @NotNull Duration connectTimeout, @NotNull Duration readTimeout) {}
