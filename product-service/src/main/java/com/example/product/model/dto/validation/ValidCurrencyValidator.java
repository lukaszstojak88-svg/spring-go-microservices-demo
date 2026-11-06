package com.example.product.model.dto.validation;

import jakarta.validation.ConstraintValidator;
import jakarta.validation.ConstraintValidatorContext;
import java.util.Currency;
import java.util.Optional;
import java.util.regex.Pattern;

public class ValidCurrencyValidator implements ConstraintValidator<ValidCurrency, String> {

  private static final Pattern CODE = Pattern.compile("^[A-Z]{3}$");

  @Override
  public boolean isValid(String value, ConstraintValidatorContext context) {
    return value == null || fractionDigits(value).isPresent();
  }

  /**
   * Returns the number of fraction digits of a supported currency, or empty when the code is not a
   * supported ISO-4217 currency (unknown, lower case, or without a minor unit such as {@code XXX}).
   */
  public static Optional<Integer> fractionDigits(String code) {
    if (code == null || !CODE.matcher(code).matches()) {
      return Optional.empty();
    }
    try {
      int digits = Currency.getInstance(code).getDefaultFractionDigits();
      return digits >= 0 ? Optional.of(digits) : Optional.empty();
    } catch (IllegalArgumentException e) {
      return Optional.empty();
    }
  }
}
