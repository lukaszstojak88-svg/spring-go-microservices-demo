package com.example.order.model.entity;

import java.math.BigDecimal;
import org.springframework.data.mongodb.core.mapping.Field;
import org.springframework.data.mongodb.core.mapping.FieldType;

/**
 * Immutable product snapshot taken when the order was created (decision D2). Reading an order never
 * requires the Product Service; {@code productId} is a logical reference only.
 */
public class OrderItem {

  private String productId;

  private String productName;

  @Field(targetType = FieldType.DECIMAL128)
  private BigDecimal unitPrice;

  private String currency;

  private int quantity;

  @Field(targetType = FieldType.DECIMAL128)
  private BigDecimal lineTotal;

  public String getProductId() {
    return productId;
  }

  public void setProductId(String productId) {
    this.productId = productId;
  }

  public String getProductName() {
    return productName;
  }

  public void setProductName(String productName) {
    this.productName = productName;
  }

  public BigDecimal getUnitPrice() {
    return unitPrice;
  }

  public void setUnitPrice(BigDecimal unitPrice) {
    this.unitPrice = unitPrice;
  }

  public String getCurrency() {
    return currency;
  }

  public void setCurrency(String currency) {
    this.currency = currency;
  }

  public int getQuantity() {
    return quantity;
  }

  public void setQuantity(int quantity) {
    this.quantity = quantity;
  }

  public BigDecimal getLineTotal() {
    return lineTotal;
  }

  public void setLineTotal(BigDecimal lineTotal) {
    this.lineTotal = lineTotal;
  }
}
