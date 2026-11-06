package com.example.product;

import static org.assertj.core.api.Assertions.assertThat;

import com.example.product.support.MongoContainerSupport;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;

/** The application starts against a real MongoDB and reports liveness and readiness. */
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class ApplicationIT extends MongoContainerSupport {

  @Autowired private TestRestTemplate restTemplate;

  @Test
  void readinessProbeIsUp() {
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
        .contains("/api/products", "/api/products/{id}", "/api/products/lookup");
  }
}
