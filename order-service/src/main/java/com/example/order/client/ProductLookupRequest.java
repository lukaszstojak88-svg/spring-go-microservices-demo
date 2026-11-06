package com.example.order.client;

import java.util.List;

/** Order Service's own copy of the Product Service batch lookup request (no shared library). */
public record ProductLookupRequest(List<String> ids) {}
