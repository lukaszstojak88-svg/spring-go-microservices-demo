package com.example.order.service;

import com.example.order.client.ProductClient;
import com.example.order.client.ProductData;
import com.example.order.client.ProductLookupResponse;
import com.example.order.client.ProductServiceUnavailableException;
import com.example.order.exception.ApiError.FieldDetail;
import com.example.order.exception.InvalidSortException;
import com.example.order.exception.OrderConflictException;
import com.example.order.exception.OrderNotFoundException;
import com.example.order.exception.OrderRuleViolationException;
import com.example.order.exception.OrderRuleViolationException.Code;
import com.example.order.mapper.OrderMapper;
import com.example.order.model.dto.CreateOrderRequest;
import com.example.order.model.dto.OrderItemRequest;
import com.example.order.model.dto.OrderResponse;
import com.example.order.model.dto.PaginatedResponse;
import com.example.order.model.entity.Order;
import com.example.order.model.entity.OrderItem;
import com.example.order.model.entity.OrderStatus;
import com.example.order.repository.OrderRepository;
import java.math.BigDecimal;
import java.math.RoundingMode;
import java.time.Clock;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.ArrayList;
import java.util.Currency;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.function.Function;
import java.util.stream.Collectors;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.PageRequest;
import org.springframework.data.domain.Sort;
import org.springframework.stereotype.Service;

@Service
public class OrderService {

  private static final Logger log = LoggerFactory.getLogger(OrderService.class);

  static final Set<String> SORTABLE_FIELDS = Set.of("createdAt", "updatedAt", "totalAmount");
  static final Sort DEFAULT_SORT = Sort.by(Sort.Direction.DESC, "createdAt");

  private final OrderRepository repository;
  private final ProductClient productClient;
  private final OrderMapper mapper;
  private final Clock clock;

  public OrderService(
      OrderRepository repository, ProductClient productClient, OrderMapper mapper, Clock clock) {
    this.repository = repository;
    this.productClient = productClient;
    this.mapper = mapper;
    this.clock = clock;
  }

  /**
   * Verifies all products with ONE batch lookup, builds the product snapshot, computes totals and
   * persists the order. Nothing is persisted unless every product passes verification.
   */
  public OrderResponse create(CreateOrderRequest request) {
    List<OrderItemRequest> requestedItems = request.items();
    List<String> productIds = requestedItems.stream().map(OrderItemRequest::productId).toList();

    ProductLookupResponse lookup = productClient.lookup(productIds);
    Map<String, ProductData> products =
        lookup.products().stream()
            .collect(Collectors.toMap(ProductData::id, Function.identity(), (a, b) -> a));

    verifyAllExist(requestedItems, products);
    verifyAllAvailable(requestedItems, products);
    String currency = verifySingleCurrency(requestedItems, products);
    int scale = currencyScale(currency);

    List<OrderItem> snapshot = new ArrayList<>();
    BigDecimal total = BigDecimal.ZERO.setScale(scale);
    for (OrderItemRequest item : requestedItems) {
      OrderItem line = mapper.toSnapshot(products.get(item.productId()), item.quantity());
      line.setUnitPrice(toScale(line.getUnitPrice(), scale, line.getProductId()));
      line.setLineTotal(
          line.getUnitPrice().multiply(BigDecimal.valueOf(line.getQuantity())).setScale(scale));
      total = total.add(line.getLineTotal());
      snapshot.add(line);
    }

    Instant now = now();
    Order order = new Order();
    order.setCustomerId(request.customerId());
    order.setStatus(OrderStatus.CREATED);
    order.setItems(snapshot);
    order.setTotalAmount(total);
    order.setCurrency(currency);
    order.setCreatedAt(now);
    order.setUpdatedAt(now);

    Order saved = repository.save(order);
    log.info(
        "Order created: id={}, items={}, total={} {}",
        saved.getId(),
        snapshot.size(),
        total.toPlainString(),
        currency);
    return mapper.toResponse(saved);
  }

  /** Reads the stored snapshot only; the Product Service is never called. */
  public OrderResponse get(String id) {
    return mapper.toResponse(findOrThrow(id));
  }

  public PaginatedResponse<OrderResponse> list(
      int page, int size, OrderStatus status, String sort) {
    PageRequest pageRequest = PageRequest.of(page, size, parseSort(sort));
    Page<Order> orders =
        status == null
            ? repository.findAll(pageRequest)
            : repository.findByStatus(status, pageRequest);
    return PaginatedResponse.from(orders, mapper::toResponse);
  }

  /** Allowed transition: CREATED -> CANCELLED. Cancelling again is a conflict. */
  public OrderResponse cancel(String id) {
    Order order = findOrThrow(id);
    if (order.getStatus() == OrderStatus.CANCELLED) {
      throw new OrderConflictException("Order " + id + " is already cancelled");
    }
    order.setStatus(OrderStatus.CANCELLED);
    order.setUpdatedAt(now());

    Order saved = repository.save(order); // @Version detects concurrent cancellations.
    log.info("Order cancelled: id={}", id);
    return mapper.toResponse(saved);
  }

  private static void verifyAllExist(
      List<OrderItemRequest> items, Map<String, ProductData> products) {
    List<FieldDetail> details = new ArrayList<>();
    List<String> missing = new ArrayList<>();
    for (int i = 0; i < items.size(); i++) {
      String productId = items.get(i).productId();
      if (!products.containsKey(productId)) {
        missing.add(productId);
        details.add(new FieldDetail(itemField(i), "Product " + productId + " does not exist"));
      }
    }
    if (!details.isEmpty()) {
      throw new OrderRuleViolationException(
          Code.PRODUCT_NOT_FOUND, "Product(s) not found: " + String.join(", ", missing), details);
    }
  }

  private static void verifyAllAvailable(
      List<OrderItemRequest> items, Map<String, ProductData> products) {
    List<FieldDetail> details = new ArrayList<>();
    List<String> unavailable = new ArrayList<>();
    for (int i = 0; i < items.size(); i++) {
      String productId = items.get(i).productId();
      if (!products.get(productId).available()) {
        unavailable.add(productId);
        details.add(new FieldDetail(itemField(i), "Product " + productId + " is not available"));
      }
    }
    if (!details.isEmpty()) {
      throw new OrderRuleViolationException(
          Code.PRODUCT_UNAVAILABLE,
          "Product(s) not available: " + String.join(", ", unavailable),
          details);
    }
  }

  /** All items must share the currency of the first item. */
  private static String verifySingleCurrency(
      List<OrderItemRequest> items, Map<String, ProductData> products) {
    String currency = products.get(items.getFirst().productId()).currency();
    List<FieldDetail> details = new ArrayList<>();
    for (int i = 0; i < items.size(); i++) {
      String itemCurrency = products.get(items.get(i).productId()).currency();
      if (!currency.equals(itemCurrency)) {
        details.add(
            new FieldDetail(
                itemField(i),
                "Product "
                    + items.get(i).productId()
                    + " is priced in "
                    + itemCurrency
                    + ", expected "
                    + currency));
      }
    }
    if (!details.isEmpty()) {
      throw new OrderRuleViolationException(
          Code.CURRENCY_MISMATCH, "All products in an order must share one currency", details);
    }
    return currency;
  }

  private static String itemField(int index) {
    return "items[" + index + "].productId";
  }

  /**
   * Fraction digits of the order currency. Product data that violates the money contract (unknown
   * currency) is an integration error, not a client error.
   */
  private static int currencyScale(String currency) {
    try {
      int digits = Currency.getInstance(currency).getDefaultFractionDigits();
      if (digits >= 0) {
        return digits;
      }
    } catch (IllegalArgumentException e) {
      // fall through
    }
    log.error("Product Service returned an unsupported currency: {}", currency);
    throw new ProductServiceUnavailableException("Unsupported currency in product data");
  }

  /** Scales a product price without rounding; a price with too many digits is a contract error. */
  private static BigDecimal toScale(BigDecimal price, int scale, String productId) {
    try {
      return price.setScale(scale, RoundingMode.UNNECESSARY);
    } catch (ArithmeticException e) {
      log.error("Product {} has a price with too many fraction digits: {}", productId, price);
      throw new ProductServiceUnavailableException("Invalid price in product data", e);
    }
  }

  private Order findOrThrow(String id) {
    return repository.findById(id).orElseThrow(() -> new OrderNotFoundException(id));
  }

  /** MongoDB stores millisecond precision; truncating keeps responses equal to stored values. */
  private Instant now() {
    return Instant.now(clock).truncatedTo(ChronoUnit.MILLIS);
  }

  /** Parses {@code field,asc|desc} against a whitelist; the id is added as a tie-breaker. */
  static Sort parseSort(String sort) {
    Sort primary = DEFAULT_SORT;
    if (sort != null && !sort.isBlank()) {
      String[] parts = sort.split(",", -1);
      String field = parts[0].trim();
      if (!SORTABLE_FIELDS.contains(field)) {
        throw new InvalidSortException(
            "sort field must be one of "
                + String.join(", ", SORTABLE_FIELDS.stream().sorted().toList()));
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
}
