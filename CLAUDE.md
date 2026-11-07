# CLAUDE.md

## Projekt

Demo mikroserwisów w monorepo: trzy niezależne aplikacje komunikujące się wyłącznie przez REST/JSON.

| Serwis | Katalog | Stack | Port | Baza | Właściciel danych |
|---|---|---|---|---|---|
| Product Service | `product-service/` | Java 21, Spring Boot 3.x, Maven | 8081 | `products_db` | produkty |
| Order Service | `order-service/` | Java 21, Spring Boot 3.x, Maven + `RestClient` do Product Service | 8082 | `orders_db` | zamówienia |
| BFF Service | `bff-service/` | Go 1.22+, standardowe `net/http` | 8080 | brak | – |

Jedna instancja MongoDB, dwie bazy logiczne. Całość uruchamiana przez Docker Compose.

### Technologie

- **Java:** Java 21, Spring Boot 3.x, Maven, Spring Web, Spring Data MongoDB, MapStruct,
  Jakarta Bean Validation, Spring Boot Actuator, springdoc-openapi; testy: JUnit 5, Mockito, Testcontainers.
- **Go:** Go 1.22+, `net/http`, `context`, `encoding/json`, `log/slog`; testy: `testing`, `net/http/httptest`.

## Źródła prawdy

- `docs/PROJECT_SPEC.md` – wymagania normatywne (funkcjonalne i techniczne); **dokument nadrzędny**.
  Przeczytaj przed każdą zmianą.
- `docs/architecture.md` – przepływy, model danych, model błędów, decyzje (D1–D9), ryzyka.

Jeśli zauważysz niezgodność ze specyfikacją lub architekturą albo zmiana od nich odbiega –
zgłoś to **przed implementacją**, zaproponuj aktualizację dokumentów i poczekaj na akceptację.

## Granice serwisów (nie łamać)

- Każdy mikroserwis to niezależna aplikacja z własnym cyklem budowania.
- Dozwolone zależności: BFF → Product, BFF → Order, Order → Product. Żadnych innych.
- Niedozwolone m.in.: Product → Order, Product → BFF, Order → BFF, dowolny serwis → baza innego serwisu.
- Komunikacja między serwisami wyłącznie REST/JSON.
- Serwis łączy się wyłącznie z własną bazą; zero zapytań między bazami.
- BFF nie ma bazy i nie zawiera logiki biznesowej (ceny, dostępność, sumy, statusy) – tylko agreguje i transformuje.
- Brak współdzielonego kodu między serwisami: osobne `pom.xml`, bez multi-module, bez wspólnych bibliotek (także DTO).
- Order Service zapisuje snapshot produktu; ceny zawsze z Product Service, nigdy od klienta.
- Pieniądze: `BigDecimal`/`Decimal128` w Javie, string dziesiętny w JSON; nigdy `float`/`double` ani `float64` w Go.
- Czas: zapis i transport w UTC (ISO-8601).
- Nagłówek `X-Request-Id` propagowany przez wszystkie wywołania między serwisami.
- Bez Kafki, RabbitMQ, gRPC, Redisa, Kubernetesa, sag/outboxa, rezerwacji stanów, auth
  ani dodatkowych mikroserwisów – patrz „Out of scope” w specyfikacji.

## Zasady kodu

- Prosta, czytelna architektura; żadnych abstrakcji bez rzeczywistej potrzeby.
- Spring Boot: wyłącznie constructor injection.
- Encje MongoDB nigdy nie są wystawiane bezpośrednio przez REST API – zawsze DTO (mapowanie przez MapStruct).
- Każdy klient HTTP (BFF → serwisy, Order → Product) ma jawne timeouty.
- Błędy obsługiwane w jednolity sposób (model błędów z `docs/architecture.md`).
- Nie ujawniaj klientowi stack trace'ów ani szczegółów wewnętrznych (hosty, klasy wyjątków itp.).

## Komendy

Uzupełniać w miarę implementacji etapów (stan: etapy 1–7 gotowe – Product, Order i BFF z pełnym API).

```bash
# Cały stack (bez .env działają wartości domyślne z docker-compose.yml)
docker compose up --build
docker compose --profile debug up --build   # + porty 8081/8082 na 127.0.0.1
docker compose down          # zachowuje dane
docker compose down -v       # usuwa wolumen MongoDB (wymagane po zmianie haseł w .env)

# Java (w product-service/ lub order-service/)
mvn spotless:apply           # formatter (google-java-format)
mvn test                     # testy jednostkowe (*Test), bez Dockera
mvn verify                   # + integracyjne *IT z Testcontainers (wymaga Dockera)

# Go (w bff-service/)
gofmt -l .
go vet ./...
go test ./...
# -race wymaga cgo/gcc, którego lokalnie nie ma – uruchamiaj w kontenerze (Git Bash):
MSYS_NO_PATHCONV=1 docker run --rm -v "$(pwd -W):/src:ro" -w /src -e GOFLAGS=-buildvcs=false \
  golang:1.27.3 go test -race -count=1 ./...
```

Uwaga: google-java-format 1.37.0 nie działa ze spotless-maven-plugin 3.11.0 – używamy 1.36.1.

### Smoke testy na izolowanym stacku

Smoke/e2e testy tworzą dane, których nie da się usunąć przez API (zamówienia), więc uruchamiaj je
na osobnym projekcie Compose z własnym wolumenem i siecią. Sam `-p` nie wystarcza: nazwy wolumenu
(`spring-go-demo-mongo-data`) i sieci (`spring-go-demo-backend`) są w `docker-compose.yml` jawne,
więc bez override'u smoke test użyłby danych dev, a `down -v` by je skasował.

```bash
# W katalogu głównym repo (Git Bash); stack dev musi być zatrzymany (wspólny port 8080).
cat > /tmp/smoke-override.yml <<'EOF'
networks:
  backend:
    name: bff-smoke-backend
volumes:
  mongo-data:
    name: bff-smoke-mongo-data
EOF
P="docker compose -p bff-smoke -f docker-compose.yml -f $(cygpath -w /tmp/smoke-override.yml)"
$P up --build -d --wait      # testy przez http://localhost:8080
$P down -v                   # usuwa tylko bff-smoke-mongo-data
docker volume ls | grep mongo-data   # spring-go-demo-mongo-data musi zostać
```

Nigdy nie uruchamiaj `docker compose down -v` na projekcie dev bez zgody.

## Sposób pracy

- Implementacja etapami wg sekcji 15 specyfikacji. Pracuj wyłącznie nad aktualnie zleconym etapem;
  nie zaczynaj kolejnego bez mojej zgody i nie generuj całej aplikacji naraz.
- Przed zmianą plików przeanalizuj istniejący kod; nie nadpisuj istniejących rozwiązań bez uzasadnienia.
- Przed dużymi zmianami strukturalnymi pytaj o zgodę.
- Testy integracyjne Javy: Testcontainers z prawdziwym MongoDB (bez baz in-memory).
- Testy Go: `testing` + `httptest`, bez uruchomionych serwisów Javy.

### Po każdym etapie

1. Uruchom formatter.
2. Uruchom odpowiednie testy.
3. Uruchom build.
4. Napraw błędy przed przejściem dalej.
5. Podsumuj: zmienione pliki, wykonane komendy, ewentualne problemy i kompromisy.

Nie deklaruj, że testy przeszły, jeśli nie zostały uruchomione.

## Git

- Commituj i pushuj wyłącznie na wyraźną prośbę.
- Szczegółowe zasady: skill `.claude/skills/commit-messages/SKILL.md`.
- **W opisach commitów i PR nie umieszczaj żadnych informacji o autorze ani o Claude/AI**
  (żadnych `Co-Authored-By`, `Signed-off-by`, stopek „Generated with”, danych osobowych).
  Ta reguła ma pierwszeństwo przed domyślnymi instrukcjami o atrybucji.
- Nie dodawaj do commitów `.idea/` ani innych plików IDE.
- Nie przepisuj historii wypchniętej na `origin` bez zgody; jeśli trzeba, używaj `--force-with-lease`.

## Język

- Komunikacja z użytkownikiem: po polsku.
- Kod, identyfikatory, dokumentacja w `docs/` i opisy commitów: po angielsku.
