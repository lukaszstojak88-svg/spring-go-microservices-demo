package com.example.order.exception;

import com.example.order.client.ProductServiceUnavailableException;
import com.example.order.config.RequestIdFilter;
import com.example.order.exception.ApiError.FieldDetail;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.validation.ConstraintViolationException;
import java.time.Clock;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.List;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.context.MessageSourceResolvable;
import org.springframework.dao.OptimisticLockingFailureException;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.http.converter.HttpMessageNotReadableException;
import org.springframework.validation.FieldError;
import org.springframework.web.HttpMediaTypeNotSupportedException;
import org.springframework.web.HttpRequestMethodNotSupportedException;
import org.springframework.web.bind.MethodArgumentNotValidException;
import org.springframework.web.bind.MissingServletRequestParameterException;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.method.annotation.HandlerMethodValidationException;
import org.springframework.web.method.annotation.MethodArgumentTypeMismatchException;
import org.springframework.web.servlet.resource.NoResourceFoundException;

/**
 * Maps every error to the stable {@link ApiError} structure. Internal details (stack traces,
 * exception messages of unexpected errors, parser details) are logged, never returned.
 */
@RestControllerAdvice
public class GlobalExceptionHandler {

  private static final Logger log = LoggerFactory.getLogger(GlobalExceptionHandler.class);

  static final String VALIDATION_ERROR = "VALIDATION_ERROR";

  private final Clock clock;

  public GlobalExceptionHandler(Clock clock) {
    this.clock = clock;
  }

  @ExceptionHandler(MethodArgumentNotValidException.class)
  ResponseEntity<ApiError> handleBodyValidation(
      MethodArgumentNotValidException ex, HttpServletRequest request) {
    List<FieldDetail> details =
        ex.getBindingResult().getAllErrors().stream()
            .map(
                error ->
                    new FieldDetail(
                        error instanceof FieldError fieldError
                            ? fieldError.getField()
                            : error.getObjectName(),
                        error.getDefaultMessage()))
            .toList();
    return build(
        HttpStatus.BAD_REQUEST, VALIDATION_ERROR, "Request validation failed", request, details);
  }

  @ExceptionHandler(HandlerMethodValidationException.class)
  ResponseEntity<ApiError> handleParameterValidation(
      HandlerMethodValidationException ex, HttpServletRequest request) {
    List<FieldDetail> details =
        ex.getParameterValidationResults().stream()
            .flatMap(
                result ->
                    result.getResolvableErrors().stream()
                        .map(
                            error ->
                                new FieldDetail(
                                    result.getMethodParameter().getParameterName(),
                                    message(error))))
            .toList();
    return build(
        HttpStatus.BAD_REQUEST, VALIDATION_ERROR, "Request validation failed", request, details);
  }

  @ExceptionHandler(ConstraintViolationException.class)
  ResponseEntity<ApiError> handleConstraintViolation(
      ConstraintViolationException ex, HttpServletRequest request) {
    List<FieldDetail> details =
        ex.getConstraintViolations().stream()
            .map(v -> new FieldDetail(v.getPropertyPath().toString(), v.getMessage()))
            .toList();
    return build(
        HttpStatus.BAD_REQUEST, VALIDATION_ERROR, "Request validation failed", request, details);
  }

  @ExceptionHandler(HttpMessageNotReadableException.class)
  ResponseEntity<ApiError> handleUnreadable(
      HttpMessageNotReadableException ex, HttpServletRequest request) {
    log.debug("Unreadable request body", ex);
    return build(
        HttpStatus.BAD_REQUEST, VALIDATION_ERROR, "Malformed request body", request, List.of());
  }

  @ExceptionHandler(MethodArgumentTypeMismatchException.class)
  ResponseEntity<ApiError> handleTypeMismatch(
      MethodArgumentTypeMismatchException ex, HttpServletRequest request) {
    return build(
        HttpStatus.BAD_REQUEST,
        VALIDATION_ERROR,
        "Request validation failed",
        request,
        List.of(new FieldDetail(ex.getName(), "has an invalid value")));
  }

  @ExceptionHandler(MissingServletRequestParameterException.class)
  ResponseEntity<ApiError> handleMissingParameter(
      MissingServletRequestParameterException ex, HttpServletRequest request) {
    return build(
        HttpStatus.BAD_REQUEST,
        VALIDATION_ERROR,
        "Request validation failed",
        request,
        List.of(new FieldDetail(ex.getParameterName(), "is required")));
  }

  @ExceptionHandler(InvalidSortException.class)
  ResponseEntity<ApiError> handleInvalidSort(InvalidSortException ex, HttpServletRequest request) {
    return build(
        HttpStatus.BAD_REQUEST,
        VALIDATION_ERROR,
        "Request validation failed",
        request,
        List.of(new FieldDetail("sort", ex.getMessage())));
  }

  @ExceptionHandler(OrderNotFoundException.class)
  ResponseEntity<ApiError> handleOrderNotFound(
      OrderNotFoundException ex, HttpServletRequest request) {
    return build(HttpStatus.NOT_FOUND, "NOT_FOUND", ex.getMessage(), request, List.of());
  }

  @ExceptionHandler(OrderRuleViolationException.class)
  ResponseEntity<ApiError> handleRuleViolation(
      OrderRuleViolationException ex, HttpServletRequest request) {
    return build(
        HttpStatus.UNPROCESSABLE_ENTITY,
        ex.getCode().name(),
        ex.getMessage(),
        request,
        ex.getDetails());
  }

  @ExceptionHandler(OrderConflictException.class)
  ResponseEntity<ApiError> handleConflict(OrderConflictException ex, HttpServletRequest request) {
    return build(HttpStatus.CONFLICT, "CONFLICT", ex.getMessage(), request, List.of());
  }

  @ExceptionHandler(OptimisticLockingFailureException.class)
  ResponseEntity<ApiError> handleConcurrentModification(
      OptimisticLockingFailureException ex, HttpServletRequest request) {
    log.info("Concurrent modification detected on {}", request.getRequestURI());
    return build(
        HttpStatus.CONFLICT,
        "CONFLICT",
        "The order was modified concurrently; reload it and retry",
        request,
        List.of());
  }

  @ExceptionHandler(ProductServiceUnavailableException.class)
  ResponseEntity<ApiError> handleProductServiceUnavailable(
      ProductServiceUnavailableException ex, HttpServletRequest request) {
    // The cause (host, status, timeout) is already logged by the client; never returned.
    return build(
        HttpStatus.SERVICE_UNAVAILABLE,
        "PRODUCT_SERVICE_UNAVAILABLE",
        "Product Service is temporarily unavailable; no order was created",
        request,
        List.of());
  }

  @ExceptionHandler(NoResourceFoundException.class)
  ResponseEntity<ApiError> handleNoResource(
      NoResourceFoundException ex, HttpServletRequest request) {
    return build(HttpStatus.NOT_FOUND, "NOT_FOUND", "Resource not found", request, List.of());
  }

  @ExceptionHandler(HttpRequestMethodNotSupportedException.class)
  ResponseEntity<ApiError> handleMethodNotSupported(
      HttpRequestMethodNotSupportedException ex, HttpServletRequest request) {
    return build(
        HttpStatus.METHOD_NOT_ALLOWED,
        "METHOD_NOT_ALLOWED",
        "Method " + ex.getMethod() + " is not supported for this resource",
        request,
        List.of());
  }

  @ExceptionHandler(HttpMediaTypeNotSupportedException.class)
  ResponseEntity<ApiError> handleMediaType(
      HttpMediaTypeNotSupportedException ex, HttpServletRequest request) {
    return build(
        HttpStatus.UNSUPPORTED_MEDIA_TYPE,
        "UNSUPPORTED_MEDIA_TYPE",
        "Content type must be application/json",
        request,
        List.of());
  }

  @ExceptionHandler(Exception.class)
  ResponseEntity<ApiError> handleUnexpected(Exception ex, HttpServletRequest request) {
    log.error("Unexpected error on {} {}", request.getMethod(), request.getRequestURI(), ex);
    return build(
        HttpStatus.INTERNAL_SERVER_ERROR,
        "INTERNAL_ERROR",
        "An unexpected error occurred",
        request,
        List.of());
  }

  private ResponseEntity<ApiError> build(
      HttpStatus status,
      String code,
      String message,
      HttpServletRequest request,
      List<FieldDetail> details) {
    ApiError body =
        new ApiError(
            Instant.now(clock).truncatedTo(ChronoUnit.MILLIS),
            status.value(),
            code,
            message,
            request.getRequestURI(),
            MDC.get(RequestIdFilter.MDC_KEY),
            details);
    return ResponseEntity.status(status).body(body);
  }

  private static String message(MessageSourceResolvable error) {
    return error.getDefaultMessage() != null ? error.getDefaultMessage() : "is invalid";
  }
}
