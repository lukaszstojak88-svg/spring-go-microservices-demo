package com.example.order.config;

import com.example.order.client.ProductClient;
import com.example.order.client.ProductServiceProperties;
import com.example.order.client.RequestIdPropagationInterceptor;
import com.example.order.client.RestClientProductClient;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.boot.http.client.ClientHttpRequestFactoryBuilder;
import org.springframework.boot.http.client.ClientHttpRequestFactorySettings;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.client.RestClient;

/**
 * Builds the Product Service {@link RestClient} with explicit connect and read timeouts and {@code
 * X-Request-Id} propagation. The Product Service is NOT part of this service's health/readiness.
 */
@Configuration
@EnableConfigurationProperties(ProductServiceProperties.class)
public class ProductClientConfig {

  @Bean
  public ProductClient productClient(
      RestClient.Builder restClientBuilder, ProductServiceProperties properties) {
    ClientHttpRequestFactorySettings settings =
        ClientHttpRequestFactorySettings.defaults()
            .withConnectTimeout(properties.connectTimeout())
            .withReadTimeout(properties.readTimeout());

    RestClient restClient =
        restClientBuilder
            .baseUrl(properties.baseUrl().toString())
            .requestFactory(ClientHttpRequestFactoryBuilder.jdk().build(settings))
            .requestInterceptor(new RequestIdPropagationInterceptor())
            .build();
    return new RestClientProductClient(restClient);
  }
}
