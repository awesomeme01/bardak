# Testcontainers patterns: lifecycle, sharing, speed

**Docs:** [Spring Boot — Testcontainers](https://docs.spring.io/spring-boot/reference/testing/testcontainers.html)
· [@ServiceConnection](https://docs.spring.io/spring-boot/reference/testing/testcontainers.html#testing.testcontainers.service-connections)
· [Testcontainers JUnit 5](https://java.testcontainers.org/test_framework_integration/junit_5/)
· [Container reuse](https://java.testcontainers.org/features/reuse/)

## Contents

- Dependencies (Maven, test scope)
- Lifecycle options, slowest → fastest: per-class static `@Container`; suite-wide singleton; Spring-managed container bean
- Reuse (local dev speed only — NOT CI)
- State isolation between tests
- Manual wiring fallback (`@DynamicPropertySource`)

## Dependencies (Maven, test scope)

```xml
<dependency>
  <groupId>org.springframework.boot</groupId><artifactId>spring-boot-testcontainers</artifactId>
  <scope>test</scope>
</dependency>
<dependency>
  <groupId>org.testcontainers</groupId><artifactId>postgresql</artifactId><scope>test</scope>
</dependency>
<dependency>
  <groupId>org.testcontainers</groupId><artifactId>junit-jupiter</artifactId><scope>test</scope>
</dependency>
```

`spring-boot-dependencies` already manages the Testcontainers BOM version — don't pin it yourself.

## Lifecycle options (slowest → fastest across a suite)

### 1. Per-class static `@Container` (baseline)

```java
@Testcontainers
@SpringBootTest
class FooIT {
    @Container @ServiceConnection
    static final PostgreSQLContainer<?> DB = new PostgreSQLContainer<>("postgres:16-alpine");
}
```

Starts once per **test class**. Fine for a few IT classes; at scale you pay one Postgres startup per
class.

### 2. Singleton container shared across the whole suite (recommended at scale)

Start one container, manually, and **never stop it** — the JVM tears it down (Ryuk reaps it). Share
via a base class:

```java
@Testcontainers
public abstract class AbstractPostgresIT {
    static final PostgreSQLContainer<?> DB = new PostgreSQLContainer<>("postgres:16-alpine");
    static { DB.start(); }                      // started once for the whole JVM/suite

    @DynamicPropertySource
    static void props(DynamicPropertyRegistry r) {
        r.add("spring.datasource.url", DB::getJdbcUrl);
        r.add("spring.datasource.username", DB::getUsername);
        r.add("spring.datasource.password", DB::getPassword);
    }
}
```

> Note: with the manual singleton you **don't** use `@Container` (no per-class lifecycle) and wire
> via `@DynamicPropertySource`, since `@ServiceConnection` is tied to a `@Container`/bean. If you
> prefer `@ServiceConnection`, use the Spring-managed bean form below.

### 3. Spring-managed container bean (clean + `@ServiceConnection`)

```java
@TestConfiguration(proxyBeanMethods = false)
public class ContainersConfig {
    @Bean
    @ServiceConnection
    PostgreSQLContainer<?> postgres() {
        return new PostgreSQLContainer<>("postgres:16-alpine");
    }
}

@SpringBootTest
@Import(ContainersConfig.class)
class FooIT { ... }
```

Spring starts the container before other beans and stops it after, in the right order. Reusing the
same `@Import` config across IT classes lets the context cache keep the container warm.

## Reuse (local dev speed only — NOT CI)

Keep a container running between test-suite executions on a developer machine:

```java
new PostgreSQLContainer<>("postgres:16-alpine").withReuse(true);   // and call start() yourself
```

Enable per-machine (not via classpath): `~/.testcontainers.properties` → `testcontainers.reuse.enable=true`
(or `TESTCONTAINERS_REUSE_ENABLE=true`). With JDBC URLs add `TC_REUSABLE=true`. Reuse is
**experimental and explicitly not suited for CI** — gate it to local dev.

## State isolation between tests

- **Repository slice tests:** `@DataJdbcTest` (and `@DataJpaTest`) wrap each test in a transaction
  that **rolls back** — clean by default.
- **`@SpringBootTest` service tests:** commits are real. Reset between tests — truncate the touched
  tables in `@BeforeEach`/`@AfterEach`, or re-run Flyway clean+migrate. Never depend on another
  test's leftover rows or on execution order.

## Manual wiring fallback (`@DynamicPropertySource`)

Pre-3.1, or for properties `@ServiceConnection` doesn't cover:

```java
@DynamicPropertySource
static void props(DynamicPropertyRegistry registry) {
    registry.add("spring.datasource.url", DB::getJdbcUrl);
}
```
