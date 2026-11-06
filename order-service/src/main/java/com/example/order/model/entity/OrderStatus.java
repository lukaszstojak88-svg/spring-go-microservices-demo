package com.example.order.model.entity;

/** Allowed transition: {@code CREATED -> CANCELLED}. Orders are otherwise immutable. */
public enum OrderStatus {
  CREATED,
  CANCELLED
}
