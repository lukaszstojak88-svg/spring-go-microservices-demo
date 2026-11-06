package com.example.order.config;

import io.swagger.v3.oas.models.OpenAPI;
import io.swagger.v3.oas.models.info.Info;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class OpenApiConfig {

  @Bean
  public OpenAPI orderServiceOpenApi() {
    return new OpenAPI()
        .info(
            new Info()
                .title("Order Service API")
                .version("v1")
                .description(
                    "Source of truth for orders. Each order stores an immutable snapshot of its"
                        + " products. Money is a decimal string; timestamps are ISO-8601 UTC."
                        + " Errors use a common structure (timestamp, status, code, message,"
                        + " path, requestId, details)."));
  }
}
