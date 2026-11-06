package com.example.product.exception;

public class ProductNotFoundException extends RuntimeException {

  public ProductNotFoundException(String id) {
    super("Product " + id + " not found");
  }
}
