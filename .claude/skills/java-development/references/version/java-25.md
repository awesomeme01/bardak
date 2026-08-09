# Java 25 (LTS) — feature reference

Includes everything from Java 21. LTS, released September 2025.

## Finalized in 25

- **Scoped values** (final, JEP 506) — share immutable data with callees and child threads;
  the modern, safer alternative to `ThreadLocal` for that pattern.
- **Module import declarations** (final, JEP 511) — `import module java.base;` to import a whole
  module's exported API at once.
- **Compact source files & instance main methods** (final, JEP 512) — simpler entry points for
  scripts and learning; a source file may omit the enclosing class and use `void main()`.
- **Flexible constructor bodies** (final, JEP 513) — statements may appear before `super(...)` /
  `this(...)` (to validate/prepare arguments), as long as they don't read the object under
  construction.
- **Key Derivation Function API** (final, JEP 510) — `javax.crypto.KDF` for HKDF and friends.

## Still preview/incubator in 25 — do NOT use by default

Require `--enable-preview` (or are incubator modules). Use only if the project already opts in:

- **Structured concurrency** (JEP 505, *fifth* preview) — `StructuredTaskScope`. Still not final.
- **Primitive types in patterns, `instanceof`, and `switch`** (JEP 507, preview).
- **Stable values** (JEP 502, preview).
- **PEM encodings of cryptographic objects** (JEP 470, preview).
- **Vector API** (JEP 508, incubator — has been incubating for many releases).

## Between 21 and 25 (notable finals)

- **Unnamed variables & patterns** `_` (final 22, JEP 456).
- **Markdown documentation comments** (JEP 467, Java 23) — see the Comments section of the
  `java-development` skill for when to use `///` Markdown Javadoc.
- **`Stream.gatherers`** (final 24, JEP 485) — custom intermediate stream operations.
