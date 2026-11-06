package com.example.order.exception;

import java.time.Instant;
import java.util.List;

/** Stable error body shared (by convention, not by code) across all services. */
public record ApiError(
    Instant timestamp,
    int status,
    String code,
    String message,
    String path,
    String requestId,
    List<FieldDetail> details) {

  public record FieldDetail(String field, String message) {}
}
