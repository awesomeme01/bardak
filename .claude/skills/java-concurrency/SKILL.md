---
name: java-concurrency
description: Use when writing or reviewing concurrent/parallel Java — fanning out blocking calls, thread pools/executors, virtual threads, parallel streams, CompletableFuture, or shared mutable state. Covers choosing the right executor, submitting all tasks before joining, bounding concurrency, executor lifecycle, virtual-thread pinning, and visibility. Catches parallelStream misuse, lazy-stream sequential fan-out, unbounded fan-out, and leaked executors.
---

# Java Concurrency

## Overview

Make concurrency **explicit, bounded, and testable**, and avoid shared mutable state. The two
recurring bugs: fan-out that is *accidentally sequential*, and fan-out that is *unbounded*.

## Choosing the tool

- **Blocking / I/O-bound work → virtual threads** (Java 21+): `Executors.newVirtualThreadPerTaskExecutor()`.
  Cheap, one per task, releases the carrier while blocked.
- **CPU-bound work → a bounded pool** sized near the core count, or a parallel stream (CPU-bound,
  side-effect-free, large data only).
- **Never `parallelStream()` for I/O-bound or side-effecting work.** It runs on the shared common
  `ForkJoinPool` (sized to cores), so it gives almost no parallelism for blocking calls *and* starves
  the rest of the app that shares that pool.

## Fan out correctly

- **Submit all tasks first, then join.** Materialize the futures (`.map(submit).toList()`) before
  calling `get()`/`join()`. A single lazy pipeline `stream().map(submit).map(get)` runs
  **sequentially** — each element is joined before the next is even submitted.
- **Bound the concurrency.** Don't fire thousands of simultaneous calls at a downstream — cap
  in-flight work with a `Semaphore` (or a sized executor), or it becomes a self-inflicted DoS.

```java
private static final int MAX_IN_FLIGHT = 50;

public List<Enriched> enrichAll(final List<Item> items) {
    final var limiter = new Semaphore(MAX_IN_FLIGHT);
    try (var executor = Executors.newVirtualThreadPerTaskExecutor()) {   // closed = waits for tasks
        final List<Future<Enriched>> futures = items.stream()
            .map(item -> executor.submit(() -> {
                limiter.acquire();
                try { return enrichmentClient.enrich(item); }            // blocking I/O
                finally { limiter.release(); }
            }))
            .toList();                                                   // submit ALL before joining
        return futures.stream().map(this::join).toList();
    }
}

private Enriched join(final Future<Enriched> f) {
    try {
        return f.get();
    } catch (final ExecutionException e) {
        throw new EnrichmentException("enrichment failed", e.getCause()); // preserve cause
    } catch (final InterruptedException e) {
        Thread.currentThread().interrupt();                              // restore the flag
        throw new EnrichmentException("interrupted", e);
    }
}
```

## Executor lifecycle

- **Always shut executors down.** For call-scoped work use **try-with-resources** (`ExecutorService`
  is `AutoCloseable` since Java 19 — `close()` awaits/cancels tasks). For a shared executor, call
  `shutdown()` + `awaitTermination(...)` on application stop.
- Don't create a new pool per call for hot paths; share a bounded one (virtual-thread executors are
  cheap, but still close them).

## Virtual-thread pitfalls

- **Don't pool virtual threads** — use the per-task executor; don't wrap them in a fixed pool.
- **Pinning:** blocking inside a `synchronized` block pins the carrier thread (defeats virtual
  threads). Use a `ReentrantLock` around blocking sections instead of `synchronized`.
- Don't block the common `ForkJoinPool` (parallel streams, `CompletableFuture.*Async` without an
  explicit executor) with I/O.

## Shared state & visibility

- Prefer **immutability and thread confinement.** For genuinely shared mutable state use concurrent
  collections (`ConcurrentHashMap`), atomics (`AtomicLong`), or explicit locks — not bare fields.
- Use `volatile` for visibility of simple flags; never rely on un-synchronized reads of mutable state.
- `ThreadLocal` (and MDC) on pooled/carrier threads must be cleared in `finally` to avoid leakage —
  see `java-observability`.

## Structured concurrency (preview)

`StructuredTaskScope` is the clean "fork several, fail fast, auto-cancel siblings" tool, but it is
**preview through Java 25** — only use it if the project enables `--enable-preview` (see the
version-gating rules in `java-development`). Otherwise use the bounded executor + join pattern above.

## Common mistakes

| Rationalization                                    | Reality                                                                                                                    |
|----------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------|
| "`parallelStream()` makes it concurrent"           | It's the shared common pool, sized to cores — useless for blocking I/O and starves the app. Use a virtual-thread executor. |
| "`stream().map(submit).map(get)` runs in parallel" | Lazy streams join each element before submitting the next → sequential. Materialize futures with `.toList()` first.        |
| "Fire a task per item, they're cheap"              | Unbounded fan-out hammers the downstream. Cap in-flight work with a `Semaphore`.                                           |
| "Virtual threads don't need pools, so no cleanup"  | Still close the executor (try-with-resources).                                                                             |
| "`synchronized` is fine around the remote call"    | It pins the carrier thread under virtual threads — use `ReentrantLock`.                                                    |
| "One thread writes, one reads a plain field"       | Without `volatile`/synchronization the read may never see the write.                                                       |

## Red flags — stop

- `parallelStream()` over blocking/I/O or side-effecting work
- `stream().map(...submit...).map(...get/join...)` in one lazy pipeline (sequential)
- Submitting one task per element with no `Semaphore`/bound
- An `ExecutorService` created and never closed/shut down
- A blocking call inside `synchronized` on a virtual-thread path
- Shared mutable field read across threads without `volatile`/lock
