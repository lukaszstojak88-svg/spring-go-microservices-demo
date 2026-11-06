package com.example.product.model.dto.validation;

import jakarta.validation.Constraint;
import jakarta.validation.Payload;
import java.lang.annotation.Documented;
import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

/**
 * An ISO-4217 currency code in upper case with a defined minor unit (e.g. {@code PLN}). {@code
 * null} is valid; combine with {@code @NotNull}.
 */
@Documented
@Constraint(validatedBy = ValidCurrencyValidator.class)
@Target({ElementType.FIELD, ElementType.PARAMETER, ElementType.RECORD_COMPONENT})
@Retention(RetentionPolicy.RUNTIME)
public @interface ValidCurrency {

  String message() default "must be a valid ISO-4217 currency code, e.g. PLN";

  Class<?>[] groups() default {};

  Class<? extends Payload>[] payload() default {};
}
