---
name: java-development
description: Use when writing, reviewing, or modifying Java/JVM source — the house standard for everyday Java craft: design and naming (composition over inheritance, interfaces over implementations, enums, Optional-as-return, no raw types), structure and immutability, dependency injection, exception handling, resource management, input validation, strings, date/time, comments, and gating features to the project's JDK version. Covers almost any Java change that isn't specifically tests or observability.
---

# Java Development

## Overview

The house standard for writing Java well. Apply these as a set on any Java change. For tests use the
`java-testing` skill; for logging/metrics/tracing use the `java-observability` skill.

**First, know your JDK.** Only use language/library features that are *final* in the project's
configured Java version; never use *preview*/incubator features unless the project already enables
`--enable-preview`. Detect the version and read the matching catalog in `references/version/` — see
[Version gating](#version-gating).

## Structure & immutability

- Declare variables, parameters, **and** fields `final` unless mutation is intentional and justified.
- Inject mandatory dependencies through the **constructor** and null-check them; no field/setter
  injection for required collaborators.
- **Always use braces** on `if`/`for`/`while`/`else`, even single-line bodies.
- **Class-level imports only** — no member/static imports unless an API is designed for it.
- Records for immutable data carriers (Java 16+); keep classes small and single-purpose.

```java
public final class InvoiceService {
    private final InvoiceRepository repository;

    public InvoiceService(final InvoiceRepository repository) {
        this.repository = Objects.requireNonNull(repository, "repository");
    }

    public BigDecimal total(final Invoice invoice) {
        if (invoice.lineItems().isEmpty()) {
            return BigDecimal.ZERO;
        }
        final BigDecimal subtotal = invoice.lineItems().stream()
            .map(LineItem::amount).reduce(BigDecimal.ZERO, BigDecimal::add);
        return subtotal;
    }
}
```

## Design & clarity

- **Clarity over cleverness.** Small, single-purpose methods (~10–20 lines); descriptive names
  (`calculateTotalPrice`, `timeoutInMilliseconds`), no abbreviations.
- **Guard clauses over deep nesting** — check and return early instead of nesting `if`s.
- **Composition over inheritance**; make classes `final` unless explicitly designed for extension.
- **Program to interfaces:** declare `List`/`Map`/`Set`, not `ArrayList`/`HashMap`. Prefer
  collections over arrays.
- **No raw types**; always `@Override`; **enums over int/String constants**.
- **`Optional` is a return type only** — never a field, parameter, or collection element.
- **Minimize scope:** declare variables at first use, in the narrowest scope that works.
- **Override `equals` and `hashCode` together**, never one alone — or use a `record`.
- **Switch expressions** (14+) over if-else chains on one value; **lambdas/method references** over
  anonymous classes; **enhanced-for or streams** over index loops (all version-gated, see below).

```java
// ❌ nested conditionals, int constants, silent fall-through
if (order != null) {
    if (!order.items().isEmpty()) {
        if (type == STANDARD) { fee = base; } else if (type == EXPRESS) { fee = base * 2; }
    }
}

// ✅ guard clauses + enum + exhaustive switch expression (14+)
Objects.requireNonNull(order, "order");
if (order.items().isEmpty()) {
    return BigDecimal.ZERO;
}
return switch (order.type()) {          // compiler enforces every ShippingType is handled
    case STANDARD -> baseFee;
    case EXPRESS -> baseFee.multiply(EXPRESS_MULTIPLIER);
};
```

## Input validation

Validate untrusted input **at the boundary, immediately** — fail fast by **throwing** a clear,
specific exception (no secrets/PII in messages). **Never return a default/sentinel (`0`, `null`,
empty) for invalid input** — that hides the bug. `Objects.requireNonNull(x, "x")` for nulls; blank
checks (`x == null || x.isBlank()`, or `StringUtils.isBlank`) for strings. Never let invalid data
flow deeper.

## Exceptions

- Catch the **most specific** type; reserve `catch (Exception)` for true top-level boundaries.
- **Never swallow** — handle, wrap+rethrow with context, or translate. Preserve the cause:
  `throw new XException("context", e)`.
- On `InterruptedException`, restore the flag: `Thread.currentThread().interrupt()` then propagate.
- Don't let `finally`/cleanup mask the original failure. Exceptions are not control flow.

## Resource management

Always **try-with-resources**, never `try`/`finally`. Watch the easy-to-miss ones: `Files.lines(...)`
and `Files.newDirectoryStream(...)` return a resource that must be closed.

```java
try (final Stream<String> lines = Files.lines(path)) {
    return lines.filter(l -> !l.isBlank()).toList();
}
```

## Strings

- `.formatted(...)` / `String.format(...)` for interpolation — not `+`.
- `String.join(delim, ...)` for joining known elements.
- `StringBuilder`/`StringJoiner` in loops; never `+` in a loop.

## Date & time

- Work in **UTC** internally (`Instant` preferred). **Inject a `java.time.Clock`** for "now"
  (`clock.instant()`) — never `Instant.now()`/`LocalDateTime.now()` directly (untestable).
- Never persist/exchange `LocalDateTime` (no offset). Be explicit about `ZoneId`; default to ISO-8601.

## Comments

Comment **intent, not mechanics** — don't restate the signature. Javadoc public APIs: behavior,
`@throws` conditions, edge cases/invariants. Avoid redundant/stale comments. Markdown Javadoc
(`///`, JEP 467) only on **Java 23+** — don't convert existing standard Javadoc.

## Version gating

Detect the project's Java version, then read the matching cumulative catalog (each includes earlier
LTS releases): `references/version/java-8.md`, `java-11.md`, `java-17.md`, `java-21.md`, `java-25.md`.

| Build tool | Where to look                                                                                |
|------------|----------------------------------------------------------------------------------------------|
| Maven      | `maven-compiler-plugin` `<release>`/`<source>`, or `maven.compiler.release` / `java.version` |
| Gradle     | `java.toolchain.languageVersion`, or `sourceCompatibility`/`targetCompatibility`             |

If undeterminable: the lowest LTS the project mentions, else **Java 8** (Maven's default baseline).
Don't assume the JDK you happen to be running. Preview features have been removed before (string
templates, gone in 23) — never depend on them.

## Red flags — stop

- A local/parameter without `final`; a control statement without braces; field/setter injection
- A raw type; a missing `@Override`; an `Optional` field/parameter; `equals` without `hashCode`
- Deeply nested conditionals where guard clauses would flatten; a variable typed `ArrayList` not `List`
- `+` interpolation or `+` in a loop; `Instant.now()`/`LocalDateTime`; `Files.lines(...)` not in `try (...)`
- `catch (Exception)` outside a boundary; swallowed exception or lost cause; swallowed `InterruptedException`
- Unvalidated external input passed deeper; a feature newer than the project's JDK, or a preview feature
