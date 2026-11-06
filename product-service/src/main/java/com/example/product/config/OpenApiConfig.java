package com.example.product.config;

import io.swagger.v3.oas.models.OpenAPI;
import io.swagger.v3.oas.models.info.Info;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class OpenApiConfig {

  @Bean
  public OpenAPI productServiceOpenApi() {
    return new OpenAPI()
        .info(
            new Info()
                .title("Product Service API")
                .version("v1")
                .description(
                    "Source of truth for product data. Money is a decimal string scaled to the"
                        + " currency; timestamps are ISO-8601 UTC. Errors use a common structure"
                        + " (timestamp, status, code, message, path, requestId, details)."));
  }
}
