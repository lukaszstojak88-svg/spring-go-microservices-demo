package com.example.product.repository;

import static org.assertj.core.api.Assertions.assertThat;

import com.example.product.model.entity.Product;
import com.example.product.support.MongoContainerSupport;
import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;
import org.bson.Document;
import org.bson.types.Decimal128;
import org.bson.types.ObjectId;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.data.mongo.DataMongoTest;
import org.springframework.data.domain.PageRequest;
import org.springframework.data.domain.Sort;
import org.springframework.data.mongodb.core.MongoTemplate;

/** Persistence against a real MongoDB (Testcontainers), including the Decimal128 money format. */
@DataMongoTest
class ProductRepositoryIT extends MongoContainerSupport {

  private static final Instant CREATED = Instant.parse("2026-01-15T10:30:00.123Z");

  @Autowired private ProductRepository repository;
  @Autowired private MongoTemplate mongoTemplate;

  @BeforeEach
  void cleanDatabase() {
    repository.deleteAll();
  }

  @Test
  void savesAndReadsProduct() {
    Product saved = repository.save(product("Mechanical Keyboard", "349.99", "PLN"));

    assertThat(saved.getId()).isNotBlank();
    Product found = repository.findById(saved.getId()).orElseThrow();
    assertThat(found.getName()).isEqualTo("Mechanical Keyboard");
    assertThat(found.getDescription()).isEqualTo("Compact mechanical keyboard");
    assertThat(found.getPrice()).isEqualTo(new BigDecimal("349.99"));
    assertThat(found.getCurrency()).isEqualTo("PLN");
    assertThat(found.isAvailable()).isTrue();
    assertThat(found.getCreatedAt()).isEqualTo(CREATED);
    assertThat(found.getUpdatedAt()).isEqualTo(CREATED);
  }

  @Test
  void generatesObjectIdHexIdentifier() {
    Product saved = repository.save(product("Mouse", "20.00", "PLN"));

    assertThat(ObjectId.isValid(saved.getId())).isTrue();
  }

  @Test
  void updatesProduct() {
    Product saved = repository.save(product("Mouse", "20.00", "PLN"));
    Instant updatedAt = CREATED.plusSeconds(60);

    saved.setName("Mouse Pro");
    saved.setPrice(new BigDecimal("25.50"));
    saved.setAvailable(false);
    saved.setUpdatedAt(updatedAt);
    repository.save(saved);

    Product found = repository.findById(saved.getId()).orElseThrow();
    assertThat(found.getName()).isEqualTo("Mouse Pro");
    assertThat(found.getPrice()).isEqualTo(new BigDecimal("25.50"));
    assertThat(found.isAvailable()).isFalse();
    assertThat(found.getCreatedAt()).isEqualTo(CREATED);
    assertThat(found.getUpdatedAt()).isEqualTo(updatedAt);
    assertThat(repository.count()).isEqualTo(1);
  }

  @Test
  void deletesProduct() {
    Product saved = repository.save(product("Mouse", "20.00", "PLN"));

    repository.deleteById(saved.getId());

    assertThat(repository.findById(saved.getId())).isEmpty();
    assertThat(repository.existsById(saved.getId())).isFalse();
  }

  @Test
  void findByUnknownIdReturnsEmpty() {
    assertThat(repository.findById("673abc1234567890abcdef12")).isEmpty();
    assertThat(repository.findById("not-an-object-id")).isEmpty();
  }

  @Test
  void storesPriceAsDecimal128() {
    Product saved = repository.save(product("Keyboard", "349.99", "PLN"));

    Document raw =
        mongoTemplate
            .getCollection("products")
            .find(new Document("_id", new ObjectId(saved.getId())))
            .first();

    assertThat(raw).isNotNull();
    assertThat(raw.get("price")).isInstanceOf(Decimal128.class);
    assertThat(raw.get("price")).isEqualTo(Decimal128.parse("349.99"));
  }

  @ParameterizedTest
  @ValueSource(strings = {"0", "0.00", "0.10", "20.00", "349.99", "1500", "12345678901234567.89"})
  void moneyRoundTripPreservesValueAndScale(String amount) {
    BigDecimal price = new BigDecimal(amount);
    Product saved = repository.save(product("Item", amount, "PLN"));

    BigDecimal read = repository.findById(saved.getId()).orElseThrow().getPrice();

    // equals() also compares scale: "20.00" must stay "20.00", not become "20" or "20.0".
    assertThat(read).isEqualTo(price);
    assertThat(read.toPlainString()).isEqualTo(amount);
  }

  @Test
  void findAllByIdReturnsOnlyExistingProducts() {
    Product first = repository.save(product("A", "1.00", "PLN"));
    Product second = repository.save(product("B", "2.00", "PLN"));

    List<Product> found =
        repository.findAllById(List.of(first.getId(), "673abc1234567890abcdef12", second.getId()));

    assertThat(found)
        .extracting(Product::getId)
        .containsExactlyInAnyOrder(first.getId(), second.getId());
  }

  @Test
  void sortsNumericallyByDecimal128Price() {
    repository.save(product("Ten", "10.00", "PLN"));
    repository.save(product("Nine", "9.99", "PLN"));
    repository.save(product("Hundred", "100.00", "PLN"));

    List<Product> sorted = repository.findAll(PageRequest.of(0, 10, Sort.by("price"))).getContent();

    // A string-stored price would sort "10.00" < "100.00" < "9.99".
    assertThat(sorted).extracting(Product::getName).containsExactly("Nine", "Ten", "Hundred");
  }

  private static Product product(String name, String price, String currency) {
    Product product = new Product();
    product.setName(name);
    product.setDescription("Compact mechanical keyboard");
    product.setPrice(new BigDecimal(price));
    product.setCurrency(currency);
    product.setAvailable(true);
    product.setCreatedAt(CREATED);
    product.setUpdatedAt(CREATED);
    return product;
  }
}
