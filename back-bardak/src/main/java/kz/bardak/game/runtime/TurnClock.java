package kz.bardak.game.runtime;

import java.time.Duration;
import java.util.Map;
import java.util.Optional;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

/**
 * Таймеры хода и ожидания вернувшегося игрока.
 *
 * <p>⭐ Таймер при отключении <b>останавливается, а не перезапускается</b> (§5.2): игрок,
 * у которого оставалось три секунды, после возвращения получает свои три секунды, а не
 * полные тридцать. Приостанавливаемый таймер — не то же самое, что перезапускаемый,
 * и именно это различие правила требуют явно.
 *
 * <p>Само срабатывание не делает ничего игрового: оно кладёт задачу в очередь стола
 * (ADR-007), и решение принимается там, где принимаются все остальные.
 */
@Component
public class TurnClock {

    private static final Logger log = LoggerFactory.getLogger(TurnClock.class);

    private final ScheduledExecutorService scheduler = Executors.newScheduledThreadPool(1, runnable -> {
        final Thread thread = new Thread(runnable, "turn-clock");
        thread.setDaemon(true);
        return thread;
    });

    private final Map<UUID, Pending> pending = new ConcurrentHashMap<>();

    /** Запустить отсчёт заново. Предыдущий таймер стола отменяется. */
    public void start(final UUID tableId, final Duration timeout, final Runnable onExpiry) {
        cancel(tableId);
        schedule(tableId, timeout, onExpiry);
    }

    /**
     * Остановить отсчёт и запомнить остаток. Возвращает, сколько оставалось.
     */
    public Duration pause(final UUID tableId) {
        final Pending current = pending.remove(tableId);
        if (current == null) {
            return Duration.ZERO;
        }
        // Остаток снимается с самого задания: оно знает, сколько ему осталось лучше,
        // чем любая наша копия этого числа.
        final Duration left = Duration.ofMillis(Math.max(0,
                current.future().getDelay(TimeUnit.MILLISECONDS)));
        current.future().cancel(false);
        paused.put(tableId, new Paused(left, current.onExpiry()));
        return left;
    }

    /**
     * Сколько осталось на ход. Пусто — часы не идут: либо ждать некого, либо матч на паузе.
     *
     * <p>⭐ Остаток снимается с самого задания, а не считается по своей копии времени:
     * два счётчика одного и того же неизбежно разъезжаются, и клиент увидел бы не то,
     * по чему сервер на самом деле сходит за игрока.
     */
    public Optional<Duration> remaining(final UUID tableId) {
        final Pending current = pending.get(tableId);
        if (current == null) {
            return Optional.empty();
        }
        return Optional.of(Duration.ofMillis(Math.max(0,
                current.future().getDelay(TimeUnit.MILLISECONDS))));
    }

    /** Продолжить с остатка. Если паузы не было — ничего не делает. */
    public void resume(final UUID tableId) {
        final Paused stopped = paused.remove(tableId);
        if (stopped == null) {
            return;
        }
        schedule(tableId, stopped.remaining(), stopped.onExpiry());
    }

    public void cancel(final UUID tableId) {
        final Pending current = pending.remove(tableId);
        if (current != null) {
            current.future().cancel(false);
        }
        paused.remove(tableId);
    }

    /** Отдельный таймер: сколько ждём вернувшегося, прежде чем отменить матч (§5.3). */
    public void scheduleAbort(final UUID tableId, final Duration grace, final Runnable onExpiry) {
        cancelAbort(tableId);
        aborts.put(tableId, scheduler.schedule(() -> runSafely(tableId, onExpiry),
                grace.toMillis(), TimeUnit.MILLISECONDS));
    }

    public void cancelAbort(final UUID tableId) {
        final ScheduledFuture<?> future = aborts.remove(tableId);
        if (future != null) {
            future.cancel(false);
        }
    }

    private void schedule(final UUID tableId, final Duration timeout, final Runnable onExpiry) {
        final ScheduledFuture<?> future = scheduler.schedule(() -> {
            pending.remove(tableId);
            runSafely(tableId, onExpiry);
        }, timeout.toMillis(), TimeUnit.MILLISECONDS);
        pending.put(tableId, new Pending(future, onExpiry));
    }

    private void runSafely(final UUID tableId, final Runnable task) {
        try {
            task.run();
        } catch (final RuntimeException e) {
            log.error("Таймер стола {} упал", tableId, e);
        }
    }

    private final Map<UUID, Paused> paused = new ConcurrentHashMap<>();
    private final Map<UUID, ScheduledFuture<?>> aborts = new ConcurrentHashMap<>();

    private record Pending(ScheduledFuture<?> future, Runnable onExpiry) {
    }

    private record Paused(Duration remaining, Runnable onExpiry) {
    }
}
