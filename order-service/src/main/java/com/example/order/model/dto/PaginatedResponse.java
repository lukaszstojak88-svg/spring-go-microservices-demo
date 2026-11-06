package com.example.order.model.dto;

import java.util.List;
import java.util.function.Function;
import org.springframework.data.domain.Page;

public record PaginatedResponse<T>(
    List<T> content, int page, int size, long totalElements, int totalPages) {

  public static <S, T> PaginatedResponse<T> from(Page<S> page, Function<S, T> mapper) {
    return new PaginatedResponse<>(
        page.getContent().stream().map(mapper).toList(),
        page.getNumber(),
        page.getSize(),
        page.getTotalElements(),
        page.getTotalPages());
  }
}
