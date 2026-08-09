# Java 8 (LTS) — feature reference

Baseline LTS. Everything here is available in every later version too.

## Lambdas & streams

Use for simple, readable transformations; avoid deeply nested or stateful pipelines.

```java
names.stream()
    .filter(User::isActive)
    .map(User::id)
    .collect(Collectors.toList());   // on Java 8; use Stream.toList() once on 16+
```

## Optional (return types only)

`Optional` models a possibly-absent **return** value. Never use it for fields, instance variables,
method parameters, or inside collections (e.g. `List<Optional<T>>`).

```java
Optional<User> findUserById(Id id);
```

## java.time (JSR-310)

Always prefer `Instant`, `OffsetDateTime`, or `ZonedDateTime` over the legacy `Date`/`Calendar`.

```java
Instant now = Instant.now(clock);
```

## Also new in 8

- Default and static methods on interfaces.
- Functional interfaces (`Function`, `Supplier`, `Predicate`, …) and method references.
- `CompletableFuture` for async composition.
