package com.example.order.mapper;

import com.example.order.client.ProductData;
import com.example.order.model.dto.OrderItemResponse;
import com.example.order.model.dto.OrderResponse;
import com.example.order.model.entity.Order;
import com.example.order.model.entity.OrderItem;
import java.math.BigDecimal;
import org.mapstruct.Mapper;
import org.mapstruct.Mapping;

@Mapper
public interface OrderMapper {

  OrderResponse toResponse(Order order);

  OrderItemResponse toItemResponse(OrderItem item);

  /**
   * Builds the immutable snapshot of a product at order time. The line total is computed by the
   * service, which owns the money rules.
   */
  @Mapping(target = "productId", source = "product.id")
  @Mapping(target = "productName", source = "product.name")
  @Mapping(target = "unitPrice", source = "product.price")
  @Mapping(target = "currency", source = "product.currency")
  @Mapping(target = "quantity", source = "quantity")
  @Mapping(target = "lineTotal", ignore = true)
  OrderItem toSnapshot(ProductData product, int quantity);

  /** Money is exposed as a plain decimal string (never scientific notation, never a number). */
  default String moneyToString(BigDecimal value) {
    return value == null ? null : value.toPlainString();
  }
}
