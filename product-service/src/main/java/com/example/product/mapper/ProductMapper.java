package com.example.product.mapper;

import com.example.product.model.dto.CreateProductRequest;
import com.example.product.model.dto.ProductResponse;
import com.example.product.model.dto.UpdateProductRequest;
import com.example.product.model.entity.Product;
import java.math.BigDecimal;
import org.mapstruct.Mapper;
import org.mapstruct.Mapping;
import org.mapstruct.MappingTarget;

@Mapper
public interface ProductMapper {

  @Mapping(target = "id", ignore = true)
  @Mapping(target = "createdAt", ignore = true)
  @Mapping(target = "updatedAt", ignore = true)
  Product toEntity(CreateProductRequest request);

  @Mapping(target = "id", ignore = true)
  @Mapping(target = "createdAt", ignore = true)
  @Mapping(target = "updatedAt", ignore = true)
  void updateEntity(UpdateProductRequest request, @MappingTarget Product product);

  ProductResponse toResponse(Product product);

  /** Money is exposed as a plain decimal string (never scientific notation, never a number). */
  default String moneyToString(BigDecimal value) {
    return value == null ? null : value.toPlainString();
  }
}
