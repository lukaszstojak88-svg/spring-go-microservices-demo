package com.example.order.config;

import java.time.Clock;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class ClockConfig {

  /** All timestamps are UTC; a bean makes time controllable in tests. */
  @Bean
  public Clock clock() {
    return Clock.systemUTC();
  }
}
