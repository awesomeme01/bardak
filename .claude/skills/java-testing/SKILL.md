---
name: java-testing
description: Use when writing or reviewing Java tests, unit or integration — JUnit Jupiter with AssertJ assertions, the @DisplayName / method-name mirroring convention, behavior-focused naming, Mockito (annotation mocks, no deep stubs, @MockitoBean for Spring), and Testcontainers with @ServiceConnection for real backing services (database, Kafka, Redis). Catches JUnit-assertion use, class-level @DisplayName, imperative or deep-stub mocking, H2-instead-of-real-DB, container-per-method, and slow/flaky suites.
---

# Java Testing

## Overview

Tests document behavior. The house stack: **JUnit Jupiter** for structure, **AssertJ** for
assertions, **Mockito** (annotation-driven) for isolation, and **Testcontainers** +
`@ServiceConnection` when a test needs a real backing service. For container lifecycle and
suite-wide sharing details see `references/testcontainers-patterns.md`.

## Structure & assertions

- **JUnit Jupiter** (5+) by default, unless the repo standardizes on something else.
- **AssertJ for assertions** — `assertThat(x).isEqualTo(y)` / `assertThatThrownBy(...)`, never JUnit's
  `assertEquals`/`assertTrue`.
- Every test has a **method-level `@DisplayName`** in natural language. **Never** at the class level
  (including `@Nested`).
- The **method name mirrors the `@DisplayName` in camelCase** and describes behavior, not
  implementation: `shouldDoXWhenY`, `shouldNotDoXWhenYIsCondition`. Never a `test` prefix.
- One behavior per test; **order-independent and parallel-safe** (no shared mutable state).
- Prefer **`@ParameterizedTest`** for input matrices and edge-case grids over duplicated bodies.
- Never skip/disable/comment out a test to make the build pass.

```java
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import static org.assertj.core.api.Assertions.assertThat;

class NotificationServiceTest {
    @DisplayName("Should save published event When the producer publishes a new event")
    @Test
    void shouldSavePublishedEventWhenTheProducerPublishesANewEvent() {
        assertThat(service.publish(event).status()).isEqualTo(Status.SAVED);
    }
}
```

## Mockito

- Use the JUnit Jupiter integration: **`@ExtendWith(MockitoExtension.class)`**. Don't call
  `MockitoAnnotations.openMocks(...)` or create mocks imperatively with `mock(...)`.
- Prefer **annotation mocks**: `@Mock`, `@Spy`, `@Captor`, `@InjectMocks`.
- **Avoid brittle setups** — no deep stubbing or mock-heavy tests tied to implementation details;
  prefer fakes or real collaborators. Stub only inside the test that needs it (no global stubbing).
- For Spring-managed beans use **`@MockitoBean`** (prefer over the older `@MockBean`).

```java
@ExtendWith(MockitoExtension.class)
class OrderServiceTest {
    @Mock OrderRepository repository;
    @InjectMocks OrderService service;
}

@SpringBootTest
class PaymentControllerTest {
    @MockitoBean PaymentService paymentService;   // prefer over @MockBean
}
```

## Integration tests (Testcontainers)

Integration tests exercise the **real** backing service against production parity, fast and isolated:

- **Use a real engine via Testcontainers, not H2/in-memory.** H2's compatibility mode hides
  dialect/type/identity/`RETURNING` differences — doubly true for **Spring Data JDBC**, which emits
  SQL almost literally. Match the prod engine and major version (`postgres:16-alpine`).
- **Wire it with `@ServiceConnection`** (Spring Boot 3.1+, needs the `spring-boot-testcontainers`
  test dependency). It auto-creates the `*ConnectionDetails` bean — **prefer it over manual
  `@DynamicPropertySource`**, which is the older, more verbose fallback.
- **Container fields are `static`.** A non-static `@Container` starts a fresh container *per test
  method* — slow. Static = once per class.
- **Share containers across the whole suite**, not per class — a shared singleton (or a Spring-managed
  container in an imported `@TestConfiguration` base) so the suite starts one Postgres, not one per
  test class. See the reference.
- **Isolate state between tests.** Don't let one test's data leak into another. Use `@Transactional`
  rollback (repository slice tests) or reset (truncate / Flyway clean) in setup; never rely on order.
- **Use the narrowest slice** that exercises the behavior: `@DataJdbcTest` (JDBC beans only, rolls
  back per test) for repository tests; `@SpringBootTest` only when you need the full service path.
- **Separate integration tests from unit tests** so a slow container suite doesn't run on every
  `mvn test`: name them `*IT` (Maven **Failsafe**) vs `*Test` (**Surefire**), and/or `@Tag("integration")`.
- Don't hand-manage `start()/stop()` for `@Container` fields (the extension owns the lifecycle) —
  only for the deliberate singleton/reuse patterns in the reference.

```java
import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;
// ... plus the usual JUnit/AssertJ/Spring imports

@Testcontainers
@SpringBootTest
class OrderPersistenceIT {

    @Container
    @ServiceConnection                                 // auto-wires spring.datasource.* — no @DynamicPropertySource
    static final PostgreSQLContainer<?> POSTGRES = new PostgreSQLContainer<>("postgres:16-alpine");

    @Autowired OrderService orderService;
    @Autowired OrderRepository orderRepository;

    @DisplayName("Should persist order and read it back When place is called")
    @Test
    void shouldPersistOrderAndReadItBackWhenPlaceIsCalled() {
        var placed = orderService.place(new Order(null, "customer-42", BigDecimal.valueOf(99.95)));
        assertThat(orderRepository.findById(placed.id())).get()
            .satisfies(found -> assertThat(found.customerId()).isEqualTo("customer-42"));
    }
}
```

## Common mistakes

| Rationalization                                       | Reality                                                                                                              |
|-------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------|
| "`assertEquals` is fine / imported"                   | House standard is AssertJ — reads as behavior, better failures.                                                      |
| "`@DisplayName` on the `@Nested` class groups nicely" | Class-level `@DisplayName` is forbidden; put it on each method.                                                      |
| "`testGet` is clear enough"                           | Behavior names: `shouldReturnAccountWhenIdExists`. No `test` prefix.                                                 |
| "`openMocks` in `@BeforeEach`"                        | Use `@ExtendWith(MockitoExtension.class)` + `@Mock`.                                                                 |
| "Deep stubs keep the test short"                      | Brittle — restructure or use a fake/real collaborator.                                                               |
| "I'll `@Disabled` this flaky one"                     | Never disable tests to go green; fix the test or the code.                                                           |
| "H2 in-memory is faster for tests"                    | It hides real SQL/dialect bugs — Spring Data JDBC especially. Use the prod engine.                                   |
| "`@DynamicPropertySource` works fine"                 | `@ServiceConnection` is the modern, terse replacement (Boot 3.1+).                                                   |
| "Each test/class gets its own container"              | Non-static `@Container` = container per method; per class is still slow at scale. `static`, shared across the suite. |
| "Integration tests run with the unit tests"           | Separate `*IT`/Failsafe from `*Test`/Surefire so `mvn test` stays fast.                                              |

## Red flags — stop

- `org.junit.jupiter.api.Assertions` / `assertEquals` imported
- `@DisplayName` on a class or `@Nested` class; a `test`-prefixed name; name ≠ its `@DisplayName`
- `MockitoAnnotations.openMocks`, imperative `mock(...)`, deep stubs, `@MockBean` when `@MockitoBean` exists
- Tests sharing static/mutable state, or depending on execution order / leftover data
- `H2`/`hsqldb`/`:mem:` standing in for the production database
- A non-`static` `@Container`; manual `@DynamicPropertySource` where `@ServiceConnection` applies
- A `@SpringBootTest` where a `@DataJdbcTest`/`@WebMvcTest` slice would do
