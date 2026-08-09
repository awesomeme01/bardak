# Java 17 (LTS) — feature reference

Includes everything from Java 11.

## Records (final since 16, JEP 395)

Default choice for immutable data carriers — DTOs and value objects.

```java
public record OrderId(String value) {}
```

## Sealed classes & interfaces (final, JEP 409)

Use for closed hierarchies (states, commands, ASTs).

```java
public sealed interface State permits Open, Closed {}
```

## Pattern matching for `instanceof` (final since 16, JEP 394)

Always prefer over manual cast.

```java
if (obj instanceof String s) { ... }
```

## Text blocks (final since 15, JEP 378)

Use for multi-line literals; never concatenate multi-line strings when this is available.

```java
String json = """
    { "id": 1, "active": true }
    """;
```

## Also available (Java 12–17)

- Switch **expressions** (final 14, JEP 361): `int n = switch (day) { case MON -> 1; ... };`
- `Stream.toList()` (Java 16) — prefer over `collect(Collectors.toList())`.
- Helpful NullPointerExceptions (Java 14).

## Preview in 17 — do NOT use yet

- **Pattern matching for `switch`** is only *preview* in 17 (JEP 406). It is finalized in Java 21 —
  see `java-21.md`. Don't use it on a 17 project.
