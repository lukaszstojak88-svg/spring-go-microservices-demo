package com.example.order.exception;

import com.example.order.exception.ApiError.FieldDetail;
import java.util.List;

/**
 * The order request is well-formed but violates a business rule that depends on product data (422).
 * The message and details are safe to return to clients.
 */
public class OrderRuleViolationException extends RuntimeException {

  public enum Code {
    PRODUCT_NOT_FOUND,
    PRODUCT_UNAVAILABLE,
    CURRENCY_MISMATCH
  }

  private final Code code;
  private final List<FieldDetail> details;

  public OrderRuleViolationException(Code code, String message, List<FieldDetail> details) {
    super(message);
    this.code = code;
    this.details = List.copyOf(details);
  }

  public Code getCode() {
    return code;
  }

  public List<FieldDetail> getDetails() {
    return details;
  }
}
