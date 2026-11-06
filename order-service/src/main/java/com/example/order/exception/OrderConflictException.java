package com.example.order.exception;

/** The requested status transition is not allowed in the current state (409 CONFLICT). */
public class OrderConflictException extends RuntimeException {

  public OrderConflictException(String message) {
    super(message);
  }
}
