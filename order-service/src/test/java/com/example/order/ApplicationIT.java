package com.example.order;

import static org.assertj.core.api.Assertions.assertThat;

import com.example.order.support.MongoContainerSupport;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.test.context.TestPropertySource;

/**
 * The application starts against a real MongoDB and reports liveness and readiness. The Product
 * Service URL points to a closed port: readiness must stay UP without the Product Service (D7).
 */
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
@TestPropertySource(properties = "product-service.base-url=http://127.0.0.1:1")
class ApplicationIT extends MongoContainerSupport {

  @Autowired private TestRestTemplate restTemplate;

  @Test
  void readinessIsUpEvenWhenProductServiceIsUnreachable() {
    ResponseEntity<String> response =
        restTemplate.getForEntity("/actuator/health/readiness", String.class);

    assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
    assertThat(response.getBody()).contains("\"status\":\"UP\"");
  }

  @Test
  void livenessProbeIsUp() {
    ResponseEntity<String> response =
        restTemplate.getForEntity("/actuator/health/liveness", String.class);

    assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
  }

  @Test
  void openApiDocumentIsPublished() {
    ResponseEntity<String> response = restTemplate.getForEntity("/v3/api-docs", String.class);

    assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
    assertThat(response.getBody())
        .contains("/api/orders", "/api/orders/{id}", "/api/orders/{id}/cancel");
  }
}
