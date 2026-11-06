package com.example.product.config;

import static org.assertj.core.api.Assertions.assertThat;

import java.util.UUID;
import java.util.concurrent.atomic.AtomicReference;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.NullAndEmptySource;
import org.junit.jupiter.params.provider.ValueSource;
import org.slf4j.MDC;
import org.springframework.mock.web.MockFilterChain;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;

class RequestIdFilterTest {

  private final RequestIdFilter filter = new RequestIdFilter();

  @Test
  void propagatesValidIncomingIdToMdcAndResponse() throws Exception {
    MockHttpServletRequest request = new MockHttpServletRequest("GET", "/api/products");
    request.addHeader(RequestIdFilter.HEADER, "abc-123.X_y");
    MockHttpServletResponse response = new MockHttpServletResponse();
    AtomicReference<String> mdcDuringRequest = new AtomicReference<>();

    filter.doFilter(
        request,
        response,
        new MockFilterChain() {
          @Override
          public void doFilter(
              jakarta.servlet.ServletRequest req, jakarta.servlet.ServletResponse res) {
            mdcDuringRequest.set(MDC.get(RequestIdFilter.MDC_KEY));
          }
        });

    assertThat(mdcDuringRequest.get()).isEqualTo("abc-123.X_y");
    assertThat(response.getHeader(RequestIdFilter.HEADER)).isEqualTo("abc-123.X_y");
    assertThat(MDC.get(RequestIdFilter.MDC_KEY)).as("MDC cleared after request").isNull();
  }

  @Test
  void generatesIdWhenHeaderIsMissing() throws Exception {
    MockHttpServletResponse response = new MockHttpServletResponse();

    filter.doFilter(
        new MockHttpServletRequest("GET", "/api/products"), response, new MockFilterChain());

    String generated = response.getHeader(RequestIdFilter.HEADER);
    assertThat(UUID.fromString(generated)).isNotNull();
  }

  @ParameterizedTest
  @NullAndEmptySource
  @ValueSource(strings = {"has space", "<script>", "line\nbreak", "ąę"})
  void replacesMissingOrUnsafeIdsWithUuid(String header) {
    String resolved = RequestIdFilter.resolveRequestId(header);

    assertThat(UUID.fromString(resolved)).isNotNull();
  }

  @Test
  void rejectsTooLongIds() {
    assertThat(RequestIdFilter.resolveRequestId("a".repeat(128))).isEqualTo("a".repeat(128));
    assertThat(RequestIdFilter.resolveRequestId("a".repeat(129))).isNotEqualTo("a".repeat(129));
  }
}
