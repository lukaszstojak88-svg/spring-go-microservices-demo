package com.example.order.model.dto.validation;

import jakarta.validation.Constraint;
import jakarta.validation.Payload;
import java.lang.annotation.Documented;
import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

/** Each product may appear at most once in an order. Reported on the {@code items} field. */
@Documented
@Constraint(validatedBy = UniqueProductIdsValidator.class)
@Target(ElementType.TYPE)
@Retention(RetentionPolicy.RUNTIME)
public @interface UniqueProductIds {

  String message() default "must not contain duplicate productId values";

  Class<?>[] groups() default {};

  Class<? extends Payload>[] payload() default {};
}
