# Java 11 (LTS) — feature reference

Includes everything from Java 8.

## HTTP Client (final, JEP 321)

Prefer `java.net.http.HttpClient` for HTTP calls unless the project already standardizes on another
client (Spring `RestClient`/`WebClient`/`RestTemplate`, Apache HttpClient). Don't introduce a new
client arbitrarily.

```java
client.send(request, BodyHandlers.ofString());
```

## String & Files helpers

Use these to cut boilerplate:

```java
if (input.isBlank()) { ... }          // also: strip(), lines(), repeat(n)
Path file = Files.writeString(Files.createTempFile(dir, "demo", ".txt"), "Sample text");
String content = Files.readString(file);
```

## var for local variables

- `var` for locals (JEP 286, Java 10).
- `var` in lambda parameters (JEP 323, Java 11): `(var x, var y) -> ...`.

Use `var` when the initializer makes the type obvious; don't use it to hide non-obvious types.

## Also available (Java 9–11)

- `Collectors.toUnmodifiableList/Set/Map` and `List.of` / `Map.of` factories (Java 9–10).
- Single-file source launch (JEP 330): run `java Foo.java` without compiling first.
- Collection/Stream additions: `Stream.takeWhile`/`dropWhile`/`iterate` (Java 9).
