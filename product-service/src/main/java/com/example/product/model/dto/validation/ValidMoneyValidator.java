package com.example.product.model.dto.validation;

import jakarta.validation.ConstraintValidator;
import jakarta.validation.ConstraintValidatorContext;
import java.math.BigDecimal;
import java.util.Optional;

public class ValidMoneyValidator implements ConstraintValidator<ValidMoney, MoneyHolder> {

  @Override
  public boolean isValid(MoneyHolder value, ConstraintValidatorContext context) {
    if (value == null || value.price() == null) {
      return true; // @NotNull on the field reports a missing price.
    }
    Optional<Integer> digits = ValidCurrencyValidator.fractionDigits(value.currency());
    if (digits.isEmpty()) {
      return true; // @ValidCurrency on the field reports an invalid currency.
    }
    if (fractionDigitsOf(value.price()) <= digits.get()) {
      return true;
    }
    context.disableDefaultConstraintViolation();
    context
        .buildConstraintViolationWithTemplate(
            "must have at most " + digits.get() + " fraction digits for " + value.currency())
        .addPropertyNode("price")
        .addConstraintViolation();
    return false;
  }

  private static int fractionDigitsOf(BigDecimal price) {
    return Math.max(0, price.stripTrailingZeros().scale());
  }
}
