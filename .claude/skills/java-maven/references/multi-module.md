# Multi-module (reactor) Maven projects

**Docs:** [Guide to Working with Multiple Modules](https://maven.apache.org/guides/mini/guide-multiple-modules.html)
· [POM reference](https://maven.apache.org/pom.html)
· [Introduction to the dependency mechanism](https://maven.apache.org/guides/introduction/introduction-to-dependency-mechanism.html)
· [CLI options](https://maven.apache.org/ref/current/maven-embedder/cli.html)

> Maven 4 renames the concept: **multi-module → multi-project**, **module → subproject**, and
> `<modules>` → `<subprojects>`. The mechanics below are identical; only the element name differs.
> See `maven-4-upgrade.md`. This file uses Maven 3 terms (`<modules>`) since most repos are still 3.x.

## Contents

- Structure: parent / aggregator POM
- The reactor (build-order rules)
- Inter-module dependencies
- Centralizing versions: dependencyManagement & BOMs
- Building a subset (reactor CLI flags: `-pl`/`-am`/`-amd`/`-rf`/`-N`)
- Conventions

## Structure: parent / aggregator POM

A multi-module build has an **aggregator** POM with `<packaging>pom</packaging>` that lists its
children. It is usually also the **parent** the children inherit from (the two roles are separable
but commonly combined).

```xml
<!-- root pom.xml -->
<groupId>com.acme</groupId>
<artifactId>acme-parent</artifactId>
<version>1.0.0-SNAPSHOT</version>
<packaging>pom</packaging>

<modules>
<module>acme-core</module>
<module>acme-api</module>
<module>acme-app</module>
</modules>
```

Each child references the parent:

```xml

<parent>
    <groupId>com.acme</groupId>
    <artifactId>acme-parent</artifactId>
    <version>1.0.0-SNAPSHOT</version>
    <relativePath>..</relativePath>
</parent>
<artifactId>acme-core</artifactId>   <!-- inherits groupId + version -->
```

## The reactor

The reactor collects all modules in the build, **sorts them into the correct build order**, and
builds them in order. Sort priority:

1. A project **dependency** on another module in the build
2. A **plugin declaration** where the plugin is another module
3. A **plugin dependency** on another module
4. A **build-extension** declaration on another module
5. Otherwise, **declaration order** in `<modules>`

**Important:** `<dependencyManagement>` and `<pluginManagement>` entries do **not** affect build
order — only "instantiated" references (actual `<dependency>`/`<plugin>` use) do.

## Inter-module dependencies

Depend on a sibling like any other artifact; use `${project.version}` so the version tracks the
reactor:

```xml

<dependency>
    <groupId>com.acme</groupId>
    <artifactId>acme-core</artifactId>
    <version>${project.version}</version>
</dependency>
```

## Centralizing versions: dependencyManagement & BOMs

Manage versions once in the parent's `<dependencyManagement>`; children declare the dependency
without a version. Import a third-party **BOM** to align a whole stack (see
[Importing Dependencies / BOMs](https://maven.apache.org/guides/introduction/introduction-to-dependency-mechanism.html#importing-dependencies)):

```xml

<dependencyManagement>
    <dependencies>
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-dependencies</artifactId>
            <version>3.3.0</version>
            <type>pom</type>
            <scope>import</scope>     <!-- BOM import -->
        </dependency>
    </dependencies>
</dependencyManagement>
```

Pin plugin versions the same way via `<build><pluginManagement>` in the parent.

## Building a subset (reactor CLI flags)

| Flag   | Long form                | Effect                                                                     |
|--------|--------------------------|----------------------------------------------------------------------------|
| `-pl`  | `--projects`             | Build only the listed projects (by `:artifactId`, group:artifact, or path) |
| `-am`  | `--also-make`            | Also build the listed projects' **dependencies** (upstream)                |
| `-amd` | `--also-make-dependents` | Also build projects that **depend on** the listed ones (downstream)        |
| `-rf`  | `--resume-from`          | Resume the reactor from a given project (after a failure)                  |
| `-N`   | `--non-recursive`        | Build only the current POM, ignore `<modules>`                             |
| `-fae` | `--fail-at-end`          | Build all modules, report failures at the end                              |
| `-ff`  | `--fail-fast`            | Stop on first failure (default)                                            |

Examples:

```bash
mvn -pl acme-api -am verify        # build acme-api and everything it depends on
mvn -pl acme-core -amd verify      # build acme-core and everything that depends on it
mvn -rf acme-app verify            # resume from acme-app after a mid-reactor failure
```

> Maven 4 adds `mvn --resume` / `-r` (resume the last failed build, auto-skipping already-built
> subprojects) and reliable subfolder builds. See `maven-4-upgrade.md`.

## Conventions

- One concern per module; keep the parent thin (management + plugin config, not code).
- Never hard-code a sibling's version — use `${project.version}`.
- Version the whole reactor together; consider `${revision}` for a single source of truth.
- `mvn verify` from the root builds the whole reactor; use `-pl/-am` to scope local iterations.
