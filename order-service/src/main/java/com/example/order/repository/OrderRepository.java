package com.example.order.repository;

import com.example.order.model.entity.Order;
import com.example.order.model.entity.OrderStatus;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;
import org.springframework.data.mongodb.repository.MongoRepository;

public interface OrderRepository extends MongoRepository<Order, String> {

  Page<Order> findByStatus(OrderStatus status, Pageable pageable);
}
