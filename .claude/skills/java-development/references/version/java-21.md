# Java 21 (LTS) — feature reference

Includes everything from Java 17.

## Virtual threads (final, JEP 444)

Prefer for high-concurrency, blocking (I/O-bound) workloads.

```java
try (var exec = Executors.newVirtualThreadPerTaskExecutor()) { ... }
```

(`ExecutorService` is `AutoCloseable` since Java 19, so try-with-resources works here.)

## Pattern matching for `switch` (final, JEP 441)

Replace complex if/else chains.

```java
return switch (obj) {
    case String s -> s.length();
    case Integer i -> i;
    default -> 0;
};
```

## Record patterns (final, JEP 440)

Destructure records directly in patterns.

```java
if (point instanceof Point(int x, int y)) { ... }
```

## Sequenced collections (final, JEP 431)

`SequencedCollection`/`SequencedMap` with `getFirst()`, `getLast()`, `reversed()`.

## Preview in 21 — do NOT use by default

These require `--enable-preview` (and a release pinned to 21). Use only if the project already opts
in:

- **Structured concurrency** (JEP 453) — `StructuredTaskScope`. Still preview; finalized in no LTS
  through 25. Don't reach for it just because the use case fits.
- **Scoped values** (JEP 446) — finalized later, in Java 25.

> Note: **string templates** (JEP 430) were also preview in 21 and were **removed in Java 23**. A
> reminder that preview features are not safe to depend on.
