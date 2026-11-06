package com.example.product.controller;

import com.example.product.model.dto.CreateProductRequest;
import com.example.product.model.dto.PaginatedResponse;
import com.example.product.model.dto.ProductBatchLookupRequest;
import com.example.product.model.dto.ProductBatchLookupResponse;
import com.example.product.model.dto.ProductResponse;
import com.example.product.model.dto.UpdateProductRequest;
import com.example.product.service.ProductService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import io.swagger.v3.oas.annotations.tags.Tag;
import jakarta.validation.Valid;
import jakarta.validation.constraints.Max;
import jakarta.validation.constraints.Min;
import java.net.URI;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.servlet.support.ServletUriComponentsBuilder;

/**
 * Thin REST layer; business logic lives in {@link ProductService}. Parameter constraints are
 * enforced by Spring MVC built-in method validation.
 */
@RestController
@RequestMapping("/api/products")
@Tag(name = "Products", description = "Product catalogue (source of truth for product data)")
public class ProductController {

  private final ProductService productService;

  public ProductController(ProductService productService) {
    this.productService = productService;
  }

  @GetMapping
  @Operation(summary = "List products (paginated)")
  public PaginatedResponse<ProductResponse> list(
      @Parameter(description = "Zero-based page index") @RequestParam(defaultValue = "0") @Min(0)
          int page,
      @Parameter(description = "Page size, 1-100")
          @RequestParam(defaultValue = "20")
          @Min(1)
          @Max(100)
          int size,
      @Parameter(
              description =
                  "field,asc|desc; fields: name, price, createdAt, updatedAt; default createdAt,desc")
          @RequestParam(required = false)
          String sort) {
    return productService.list(page, size, sort);
  }

  @GetMapping("/{id}")
  @Operation(summary = "Get a product")
  public ProductResponse get(@PathVariable String id) {
    return productService.get(id);
  }

  @PostMapping
  @Operation(summary = "Create a product")
  public ResponseEntity<ProductResponse> create(@Valid @RequestBody CreateProductRequest request) {
    ProductResponse created = productService.create(request);
    URI location =
        ServletUriComponentsBuilder.fromCurrentRequestUri()
            .path("/{id}")
            .buildAndExpand(created.id())
            .toUri();
    return ResponseEntity.created(location).body(created);
  }

  @PutMapping("/{id}")
  @Operation(summary = "Replace the editable fields of a product")
  public ProductResponse update(
      @PathVariable String id, @Valid @RequestBody UpdateProductRequest request) {
    return productService.update(id, request);
  }

  @DeleteMapping("/{id}")
  @Operation(summary = "Delete a product")
  public ResponseEntity<Void> delete(@PathVariable String id) {
    productService.delete(id);
    return ResponseEntity.noContent().build();
  }

  @PostMapping("/lookup")
  @Operation(
      summary = "Batch lookup by ids (max 100)",
      description = "Missing products are returned in missingIds and are not an error.")
  public ProductBatchLookupResponse lookup(@Valid @RequestBody ProductBatchLookupRequest request) {
    return productService.lookup(request.ids());
  }
}
