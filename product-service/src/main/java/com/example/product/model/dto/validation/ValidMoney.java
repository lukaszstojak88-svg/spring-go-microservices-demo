package com.example.product.model.dto.validation;

import jakarta.validation.Constraint;
import jakarta.validation.Payload;
import java.lang.annotation.Documented;
import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

/**
 * Class-level constraint: the price must not have more fraction digits than its currency allows
 * (e.g. at most 2 for PLN). Prices are never rounded silently. The violation is reported on the
 * {@code price} field.
 */
@Documented
@Constraint(validatedBy = ValidMoneyValidator.class)
@Target(ElementType.TYPE)
@Retention(RetentionPolicy.RUNTIME)
public @interface ValidMoney {

  String message() default "has more fraction digits than the currency allows";

  Class<?>[] groups() default {};

  Class<? extends Payload>[] payload() default {};
}
