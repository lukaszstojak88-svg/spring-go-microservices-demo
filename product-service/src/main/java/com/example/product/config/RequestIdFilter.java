package com.example.product.config;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import java.io.IOException;
import java.util.UUID;
import java.util.regex.Pattern;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

/**
 * Reads or generates {@code X-Request-Id}, puts it into the MDC (so it appears in every log line
 * and error body), echoes it in the response, and writes one access-log line per request.
 */
@Component
@Order(Ordered.HIGHEST_PRECEDENCE)
public class RequestIdFilter extends OncePerRequestFilter {

  public static final String HEADER = "X-Request-Id";
  public static final String MDC_KEY = "requestId";

  /** Only safe characters are accepted, which prevents log injection via the header. */
  private static final Pattern VALID_ID = Pattern.compile("^[A-Za-z0-9._-]{1,128}$");

  private static final Logger log = LoggerFactory.getLogger(RequestIdFilter.class);

  @Override
  protected void doFilterInternal(
      HttpServletRequest request, HttpServletResponse response, FilterChain chain)
      throws ServletException, IOException {
    String requestId = resolveRequestId(request.getHeader(HEADER));
    MDC.put(MDC_KEY, requestId);
    response.setHeader(HEADER, requestId);
    long start = System.nanoTime();
    try {
      chain.doFilter(request, response);
    } finally {
      if (!request.getRequestURI().startsWith("/actuator")) {
        log.info(
            "{} {} -> {} ({} ms)",
            request.getMethod(),
            request.getRequestURI(),
            response.getStatus(),
            (System.nanoTime() - start) / 1_000_000);
      }
      MDC.remove(MDC_KEY);
    }
  }

  static String resolveRequestId(String header) {
    if (header != null && VALID_ID.matcher(header).matches()) {
      return header;
    }
    return UUID.randomUUID().toString();
  }
}
