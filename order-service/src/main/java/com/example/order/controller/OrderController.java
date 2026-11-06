package com.example.order.controller;

import com.example.order.model.dto.CreateOrderRequest;
import com.example.order.model.dto.OrderResponse;
import com.example.order.model.dto.PaginatedResponse;
import com.example.order.model.entity.OrderStatus;
import com.example.order.service.OrderService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import io.swagger.v3.oas.annotations.tags.Tag;
import jakarta.validation.Valid;
import jakarta.validation.constraints.Max;
import jakarta.validation.constraints.Min;
import java.net.URI;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.servlet.support.ServletUriComponentsBuilder;

/**
 * Thin REST layer; business logic lives in {@link OrderService}. Orders cannot be updated or
 * deleted; the only state change is cancellation.
 */
@RestController
@RequestMapping("/api/orders")
@Tag(name = "Orders", description = "Orders with immutable product snapshots")
public class OrderController {

  private final OrderService orderService;

  public OrderController(OrderService orderService) {
    this.orderService = orderService;
  }

  @PostMapping
  @Operation(
      summary = "Create an order",
      description =
          "Prices are taken from the Product Service, never from the client. Errors: 400"
              + " VALIDATION_ERROR, 422 PRODUCT_NOT_FOUND / PRODUCT_UNAVAILABLE /"
              + " CURRENCY_MISMATCH, 503 PRODUCT_SERVICE_UNAVAILABLE.")
  public ResponseEntity<OrderResponse> create(@Valid @RequestBody CreateOrderRequest request) {
    OrderResponse created = orderService.create(request);
    URI location =
        ServletUriComponentsBuilder.fromCurrentRequestUri()
            .path("/{id}")
            .buildAndExpand(created.id())
            .toUri();
    return ResponseEntity.created(location).body(created);
  }

  @GetMapping("/{id}")
  @Operation(summary = "Get an order (stored snapshot; does not call the Product Service)")
  public OrderResponse get(@PathVariable String id) {
    return orderService.get(id);
  }

  @GetMapping
  @Operation(summary = "List orders (paginated, optional status filter)")
  public PaginatedResponse<OrderResponse> list(
      @Parameter(description = "Zero-based page index") @RequestParam(defaultValue = "0") @Min(0)
          int page,
      @Parameter(description = "Page size, 1-100")
          @RequestParam(defaultValue = "20")
          @Min(1)
          @Max(100)
          int size,
      @Parameter(description = "Filter by status") @RequestParam(required = false)
          OrderStatus status,
      @Parameter(
              description =
                  "field,asc|desc; fields: createdAt, updatedAt, totalAmount; default"
                      + " createdAt,desc")
          @RequestParam(required = false)
          String sort) {
    return orderService.list(page, size, status, sort);
  }

  @PostMapping("/{id}/cancel")
  @Operation(
      summary = "Cancel an order",
      description = "Allowed only from CREATED; cancelling again returns 409 CONFLICT.")
  public OrderResponse cancel(@PathVariable String id) {
    return orderService.cancel(id);
  }
}
