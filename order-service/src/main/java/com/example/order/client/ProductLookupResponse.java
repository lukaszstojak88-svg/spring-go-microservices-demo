package com.example.order.client;

import java.util.List;

/** Order Service's own copy of the Product Service batch lookup response. */
public record ProductLookupResponse(List<ProductData> products, List<String> missingIds) {}
