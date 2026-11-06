package com.example.product.service;

import com.example.product.exception.InvalidSortException;
import com.example.product.exception.ProductNotFoundException;
import com.example.product.mapper.ProductMapper;
import com.example.product.model.dto.CreateProductRequest;
import com.example.product.model.dto.PaginatedResponse;
import com.example.product.model.dto.ProductBatchLookupResponse;
import com.example.product.model.dto.ProductResponse;
import com.example.product.model.dto.UpdateProductRequest;
import com.example.product.model.entity.Product;
import com.example.product.repository.ProductRepository;
import java.math.RoundingMode;
import java.time.Clock;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.ArrayList;
import java.util.Currency;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.function.Function;
import java.util.stream.Collectors;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.data.domain.PageRequest;
import org.springframework.data.domain.Sort;
import org.springframework.stereotype.Service;

@Service
public class ProductService {

  private static final Logger log = LoggerFactory.getLogger(ProductService.class);

  static final Set<String> SORTABLE_FIELDS = Set.of("name", "price", "createdAt", "updatedAt");
  static final Sort DEFAULT_SORT = Sort.by(Sort.Direction.DESC, "createdAt");

  private final ProductRepository repository;
  private final ProductMapper mapper;
  private final Clock clock;

  public ProductService(ProductRepository repository, ProductMapper mapper, Clock clock) {
    this.repository = repository;
    this.mapper = mapper;
    this.clock = clock;
  }

  public ProductResponse create(CreateProductRequest request) {
    Product product = mapper.toEntity(request);
    normalizePrice(product);
    Instant now = now();
    product.setCreatedAt(now);
    product.setUpdatedAt(now);

    Product saved = repository.save(product);
    log.info("Product created: id={}", saved.getId());
    return mapper.toResponse(saved);
  }

  public ProductResponse get(String id) {
    return mapper.toResponse(findOrThrow(id));
  }

  public PaginatedResponse<ProductResponse> list(int page, int size, String sort) {
    PageRequest pageRequest = PageRequest.of(page, size, parseSort(sort));
    return PaginatedResponse.from(repository.findAll(pageRequest), mapper::toResponse);
  }

  public ProductResponse update(String id, UpdateProductRequest request) {
    Product product = findOrThrow(id);
    mapper.updateEntity(request, product);
    normalizePrice(product);
    product.setUpdatedAt(now());

    Product saved = repository.save(product);
    log.info("Product updated: id={}", saved.getId());
    return mapper.toResponse(saved);
  }

  public void delete(String id) {
    if (!repository.existsById(id)) {
      throw new ProductNotFoundException(id);
    }
    repository.deleteById(id);
    log.info("Product deleted: id={}", id);
  }

  /** Missing products are reported in {@code missingIds}, never as an error. */
  public ProductBatchLookupResponse lookup(List<String> ids) {
    List<String> uniqueIds = List.copyOf(new LinkedHashSet<>(ids));
    Map<String, Product> found =
        repository.findAllById(uniqueIds).stream()
            .collect(Collectors.toMap(Product::getId, Function.identity()));

    List<ProductResponse> products = new ArrayList<>();
    List<String> missingIds = new ArrayList<>();
    for (String id : uniqueIds) {
      Product product = found.get(id);
      if (product != null) {
        products.add(mapper.toResponse(product));
      } else {
        missingIds.add(id);
      }
    }
    return new ProductBatchLookupResponse(products, missingIds);
  }

  private Product findOrThrow(String id) {
    return repository.findById(id).orElseThrow(() -> new ProductNotFoundException(id));
  }

  /**
   * Scales the price to the currency's fraction digits (e.g. 20 -> 20.00 for PLN). Validation has
   * already rejected prices with too many fraction digits, so this never rounds; UNNECESSARY makes
   * an unexpected rounding fail loudly.
   */
  private static void normalizePrice(Product product) {
    int digits = Currency.getInstance(product.getCurrency()).getDefaultFractionDigits();
    product.setPrice(product.getPrice().setScale(digits, RoundingMode.UNNECESSARY));
  }

  /** MongoDB stores millisecond precision; truncating keeps responses equal to stored values. */
  private Instant now() {
    return Instant.now(clock).truncatedTo(ChronoUnit.MILLIS);
  }

  /**
   * Parses {@code field,asc|desc} against a whitelist. The id is always added as a tie-breaker so
   * that pagination is stable.
   */
  static Sort parseSort(String sort) {
    Sort primary = DEFAULT_SORT;
    if (sort != null && !sort.isBlank()) {
      String[] parts = sort.split(",", -1);
      String field = parts[0].trim();
      if (!SORTABLE_FIELDS.contains(field)) {
        throw new InvalidSortException(
            "sort field must be one of " + String.join(", ", sorted(SORTABLE_FIELDS)));
      }
      if (parts.length > 2) {
        throw new InvalidSortException("sort must have the form field,asc|desc");
      }
      Sort.Direction direction = Sort.Direction.ASC;
      if (parts.length == 2) {
        direction =
            Sort.Direction.fromOptionalString(parts[1].trim())
                .orElseThrow(() -> new InvalidSortException("sort direction must be asc or desc"));
      }
      primary = Sort.by(direction, field);
    }
    return primary.and(Sort.by(Sort.Direction.ASC, "id"));
  }

  private static List<String> sorted(Set<String> values) {
    return values.stream().sorted().toList();
  }
}
