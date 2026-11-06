package com.example.product.model.dto;

import static org.assertj.core.api.Assertions.assertThat;

import jakarta.validation.ConstraintViolation;
import jakarta.validation.Validation;
import jakarta.validation.Validator;
import jakarta.validation.ValidatorFactory;
import java.math.BigDecimal;
import java.util.Collections;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.stream.Collectors;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;
import org.junit.jupiter.params.provider.ValueSource;

/** Business validation of request DTOs (Bean Validation + custom money/currency constraints). */
class ProductRequestValidationTest {

  private static ValidatorFactory factory;
  private static Validator validator;

  @BeforeAll
  static void setUpValidator() {
    factory = Validation.buildDefaultValidatorFactory();
    validator = factory.getValidator();
  }

  @AfterAll
  static void closeValidator() {
    factory.close();
  }

  @Test
  void acceptsValidRequest() {
    assertThat(validate(create("349.99", "PLN"))).isEmpty();
  }

  @ParameterizedTest
  @CsvSource({
    "0, PLN",
    "20, PLN",
    "20.5, PLN",
    "20.50, PLN",
    "20.500, PLN",
    "1500, JPY",
    "1.125, BHD"
  })
  void acceptsPricesWithinCurrencyScale(String price, String currency) {
    assertThat(validate(create(price, currency))).isEmpty();
  }

  @ParameterizedTest
  @CsvSource({"1.999, PLN, 2", "0.001, EUR, 2", "1.5, JPY, 0", "1.1255, BHD, 3"})
  void rejectsPricesWithTooManyFractionDigits(String price, String currency, int digits) {
    Map<String, String> violations = validate(create(price, currency));

    assertThat(violations)
        .containsEntry("price", "must have at most " + digits + " fraction digits for " + currency);
  }

  @Test
  void rejectsNegativePrice() {
    assertThat(validate(create("-0.01", "PLN"))).containsKey("price");
  }

  @ParameterizedTest
  @ValueSource(strings = {"pln", "PL", "PLNN", "ABC", "XXX", "12A", ""})
  void rejectsInvalidCurrency(String currency) {
    Map<String, String> violations = validate(create("10.00", currency));

    assertThat(violations)
        .containsEntry("currency", "must be a valid ISO-4217 currency code, e.g. PLN")
        .doesNotContainKey("price");
  }

  @Test
  void rejectsMissingRequiredFields() {
    Map<String, String> violations =
        validate(new CreateProductRequest(null, null, null, null, null));

    assertThat(violations.keySet())
        .containsExactlyInAnyOrder("name", "price", "currency", "available");
  }

  @ParameterizedTest
  @ValueSource(strings = {"", "   "})
  void rejectsBlankName(String name) {
    assertThat(validate(new CreateProductRequest(name, null, BigDecimal.ONE, "PLN", true)))
        .containsKey("name");
  }

  @Test
  void rejectsTooLongNameAndDescription() {
    Map<String, String> violations =
        validate(
            new CreateProductRequest(
                "n".repeat(201), "d".repeat(2001), BigDecimal.ONE, "PLN", true));

    assertThat(violations.keySet()).containsExactlyInAnyOrder("name", "description");
  }

  @Test
  void updateRequestHasTheSameRules() {
    Map<String, String> violations =
        validate(new UpdateProductRequest(" ", null, new BigDecimal("1.999"), "PLN", null));

    assertThat(violations.keySet()).containsExactlyInAnyOrder("name", "price", "available");
  }

  @Test
  void lookupRequestRequiresOneToHundredNonBlankIds() {
    assertThat(validate(new ProductBatchLookupRequest(List.of("a", "b")))).isEmpty();
    assertThat(validate(new ProductBatchLookupRequest(null))).containsKey("ids");
    assertThat(validate(new ProductBatchLookupRequest(List.of()))).containsKey("ids");
    assertThat(validate(new ProductBatchLookupRequest(Collections.nCopies(100, "a")))).isEmpty();
    assertThat(validate(new ProductBatchLookupRequest(Collections.nCopies(101, "a"))))
        .containsKey("ids");
    assertThat(validate(new ProductBatchLookupRequest(List.of("a", " "))))
        .containsKey("ids[1].<list element>");
  }

  private static CreateProductRequest create(String price, String currency) {
    return new CreateProductRequest("Item", "desc", new BigDecimal(price), currency, true);
  }

  private static <T> Map<String, String> validate(T request) {
    Set<ConstraintViolation<T>> violations = validator.validate(request);
    return violations.stream()
        .collect(
            Collectors.toMap(
                v -> v.getPropertyPath().toString(),
                ConstraintViolation::getMessage,
                (first, second) -> first));
  }
}
