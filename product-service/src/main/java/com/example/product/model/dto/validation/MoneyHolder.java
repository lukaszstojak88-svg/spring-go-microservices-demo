package com.example.product.model.dto.validation;

import java.math.BigDecimal;

/** A request carrying a price and its currency, validated together by {@link ValidMoney}. */
public interface MoneyHolder {

  BigDecimal price();

  String currency();
}
