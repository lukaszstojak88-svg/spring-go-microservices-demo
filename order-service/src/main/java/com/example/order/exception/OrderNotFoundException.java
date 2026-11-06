package com.example.order.exception;

public class OrderNotFoundException extends RuntimeException {

  public OrderNotFoundException(String id) {
    super("Order " + id + " not found");
  }
}
