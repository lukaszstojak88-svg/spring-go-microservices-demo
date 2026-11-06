package com.example.order.client;

import com.example.order.config.RequestIdFilter;
import java.io.IOException;
import org.slf4j.MDC;
import org.springframework.http.HttpRequest;
import org.springframework.http.client.ClientHttpRequestExecution;
import org.springframework.http.client.ClientHttpRequestInterceptor;
import org.springframework.http.client.ClientHttpResponse;

/** Forwards the current request's {@code X-Request-Id} (from the MDC) to the Product Service. */
public class RequestIdPropagationInterceptor implements ClientHttpRequestInterceptor {

  @Override
  public ClientHttpResponse intercept(
      HttpRequest request, byte[] body, ClientHttpRequestExecution execution) throws IOException {
    String requestId = MDC.get(RequestIdFilter.MDC_KEY);
    if (requestId != null) {
      request.getHeaders().set(RequestIdFilter.HEADER, requestId);
    }
    return execution.execute(request, body);
  }
}
