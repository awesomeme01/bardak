---
name: java-observability
description: Use when adding, reviewing, or retrofitting logging, metrics, tracing, or diagnostics in Java — including "improve the logging/observability of this code" requests. Covers the logging facade with parameterized structured messages, the full level palette (DEBUG/INFO/WARN/ERROR) across a unit of work, correlation IDs / MDC, never logging secrets/PII, preserving stack traces, and Micrometer / OpenTelemetry. Catches string-concatenated logs, System.out, sensitive-data logging, and missing correlation/telemetry.
---

# Java Observability

## Overview

Make a running system diagnosable. Three pillars: **logs** (what happened), **metrics**
(how much/how often), **traces** (where time went).

## Logging

- **Use the project's facade** — prefer **SLF4J**; else the project standard (Log4j2). Never
  introduce a new framework (including `java.util.logging`). Never use `System.out`/`System.err`.
- **Parameterized, not concatenated:** `log.info("created order {}", id)` — never
  `"created order " + id`. Placeholders defer string building until the level is enabled.
- **Levels deliberately:** TRACE (rare low-level), DEBUG (diagnostics), INFO (major events),
  WARN (recoverable/unexpected), ERROR (failures needing investigation).
- **Preserve stack traces:** pass the exception as the last arg (`log.error("charge failed {}", id, ex)`),
  never just `ex.getMessage()`.
- **Log once** at the boundary where the error is best understood; not in tight loops/getters.
- **Never log sensitive data:** no passwords, tokens, card numbers, PII, or full request/response
  payloads. Log **identifiers and counts** (IDs, status, sizes), not whole objects.

```java
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public final class PaymentProcessor {
    private static final Logger LOGGER = LoggerFactory.getLogger(PaymentProcessor.class);

    public Receipt process(final Payment payment) {
        LOGGER.info("Processing payment customerId={} amount={}", payment.customerId(), payment.amount());
        // never log cardNumber/cvv/tokens
        ...
    }
}
```

## Structured logging & correlation

- Prefer **structured/key-value** output (logstash/ECS encoder, or consistent `key={}` pairs) so logs
  are queryable, not just human-readable.
- Carry a **correlation/trace id** across a request via **MDC** (`MDC.put("traceId", id)`); set it at
  the entry boundary (filter/interceptor) and **clear it in `finally`** to avoid thread-pool leakage.
  For keys you add **deeper in the flow**, `MDC.remove(key)` just those in `finally` — `MDC.clear()`
  there would wipe the context the boundary owns. Put stable context (userId, operation) in MDC too,
  not hand-concatenated into every message.

```java
MDC.put("traceId", traceId);
try {
    handle(request);
} finally {
    MDC.clear();          // prevent context bleeding across pooled threads
}
```

## Metrics & tracing

- **Metrics:** use the project's metrics facade — **Micrometer** (`MeterRegistry`) in Spring — for
  counters/timers/gauges; don't hand-roll. Name consistently (`orders.placed`), tag with low
  cardinality (avoid user ids as tags). Time critical paths with a `Timer`.
- **Tracing:** prefer **OpenTelemetry** (or the project's tracer). Don't manually thread span/trace
  ids through method signatures — propagate via context (and mirror the trace id into MDC so logs
  correlate with spans). Instrument at boundaries (HTTP, messaging, DB), not every method.
- Emit telemetry as a side concern: it must not change control flow or behavior.

```java
public final class PaymentProcessor {
    private final Timer chargeTimer;

    public PaymentProcessor(final MeterRegistry registry) {
        this.chargeTimer = registry.timer("payments.charge");   // stable name, no per-user tags
    }

    public Receipt charge(final Payment payment) {
        return chargeTimer.record(() -> gateway.charge(payment));
    }
}
```

**Time in `finally`.** Manual timing (elapsed-time logs, `Timer.Sample`) must stop/log in a
**`finally`** block — placed after the call, an exception skips it, losing timing for exactly the
calls you care about. `Timer.record(...)` does this for you.

```java
// ❌ skipped when charge() throws
final long start = System.nanoTime();
final Receipt receipt = gateway.charge(payment);
LOGGER.debug("charge durationMs={}", (System.nanoTime() - start) / 1_000_000);

// ✅ recorded on success and failure alike
final Timer.Sample sample = Timer.start(registry);
try {
    return gateway.charge(payment);
} finally {
    sample.stop(chargeTimer);
}
```

## Retrofitting existing code

Asked to "improve the logging/observability" of existing code? Work per **unit of work**
(request/message/job), not per statement:

1. **Correlate first:** MDC ids at the entry boundary, cleared in `finally` (`MDC.remove` for keys
   added mid-flow).
2. **Replace offenders:** `System.out` / `printStackTrace()` / concatenation → facade + `key={}` pairs.
3. **Narrate the unit of work with the full level palette:**
    - **INFO** — start and outcome: identifiers + counts, one line each, never per item
    - **DEBUG** — per-item / per-step detail inside loops and branches
    - **WARN** — recoverable anomalies: retries, fallbacks, skipped items, **partial success**
      (`WARN "Fulfilled 3 of 5 lines"` beats an INFO that hides the mismatch in two numbers)
    - **ERROR** — the unit of work failed; exception as last arg, once, at the boundary
4. **Measure rates/latency instead of logging them:** a `Timer` on the critical path, `Counter`s for
   failure/skip events.
5. **Sweep before finishing:** no PII/secrets/payloads crept in; telemetry changed no behavior.

## Common mistakes

| Rationalization                                   | Reality                                                                      |
|---------------------------------------------------|------------------------------------------------------------------------------|
| "`"user: " + name`"                               | Use `{}` placeholders — concatenation allocates even when the level is off.  |
| "`java.util.logging` is built in"                 | Use the project facade (SLF4J/Log4j2); don't add a framework.                |
| "Log the whole request to debug"                  | Never log secrets/PII/full payloads; log identifiers.                        |
| "`log.error(e.getMessage())`"                     | Drops the stack trace — pass the exception.                                  |
| "I'll pass the traceId as a parameter everywhere" | Use MDC / context propagation; clear MDC in `finally`.                       |
| "A `userId` tag on the metric is handy"           | High-cardinality tags blow up metrics storage; tag low-cardinality only.     |
| "Log the duration after the call returns"         | An exception skips it — stop/log timing in `finally`, or use `Timer.record`. |

## Red flags — stop

- `+` inside a `log.*` call; `System.out`/`System.err`; a new logging framework
- A secret/token/card number/PII or whole payload in a log call; `log.error(e.getMessage())`
- `MDC.put` without a matching `MDC.clear()` in `finally`
- Hand-rolled counters/timers instead of Micrometer; high-cardinality metric tags
- A duration log or `Timer.Sample.stop` that isn't in a `finally` block
- Telemetry that alters control flow
