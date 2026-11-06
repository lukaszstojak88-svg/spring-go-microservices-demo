package com.example.product.model.dto;

import java.util.List;

/**
 * Found products and the ids that do not exist. Missing ids are not an error at this level; the
 * caller decides what they mean. Both lists follow the order of the request.
 */
public record ProductBatchLookupResponse(List<ProductResponse> products, List<String> missingIds) {}
