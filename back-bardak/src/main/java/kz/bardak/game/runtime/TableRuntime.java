package kz.bardak.game.runtime;

import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.function.Consumer;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Один стол — одна очередь команд и один поток-обработчик (ADR-007).
 *
 * <p>⭐ У стола до пяти источников команд плюс таймер, и каждая команда меняет состояние
 * целиком. Очередь даёт линейный порядок бесплатно и убирает целый класс гонок; движок
 * при этом остаётся однопоточным и о синхронизации не думает вообще.
 *
 * <p>⚠️ На потоке стола нельзя делать долгое — стол «залипнет» для всех за ним.
 */
public final class TableRuntime implements AutoCloseable {

    private static final Logger log = LoggerFactory.getLogger(TableRuntime.class);

    private final UUID tableId;
    private final ExecutorService queue;
    private final Map<UUID, Consumer<String>> listeners = new ConcurrentHashMap<>();

    public TableRuntime(final UUID tableId) {
        this.tableId = Objects.requireNonNull(tableId, "tableId");
        this.queue = Executors.newSingleThreadExecutor(runnable -> {
            final Thread thread = new Thread(runnable, "table-" + tableId);
            thread.setDaemon(true);
            return thread;
        });
    }

    public UUID tableId() {
        return tableId;
    }

    /**
     * Подписать соединение игрока на события стола. Один игрок — одна подписка: при
     * переподключении новая заменяет старую, и в мёртвый сокет уже никто не пишет.
     */
    public void subscribe(final UUID userId, final Consumer<String> onMessage) {
        listeners.put(userId, onMessage);
    }

    public void unsubscribe(final UUID userId) {
        listeners.remove(userId);
    }

    public boolean hasListeners() {
        return !listeners.isEmpty();
    }

    public List<UUID> subscribers() {
        return List.copyOf(listeners.keySet());
    }

    /** Поставить работу в очередь стола. Порядок выполнения — порядок постановки. */
    public void submit(final Runnable work) {
        queue.execute(() -> {
            try {
                work.run();
            } catch (final RuntimeException e) {
                log.error("Ошибка при обработке команды стола {}", tableId, e);
            }
        });
    }

    /** Всем за столом. Рассылка идёт с потока стола — порядок сообщений тот же, что и команд. */
    public void broadcast(final String message) {
        listeners.values().forEach(listener -> deliver(listener, message));
    }

    /** Персонально: проекции состояния у каждого игрока свои (ADR-002). */
    public void sendTo(final UUID userId, final String message) {
        final Consumer<String> listener = listeners.get(userId);
        if (listener != null) {
            deliver(listener, message);
        }
    }

    @Override
    public void close() {
        queue.shutdownNow();
        listeners.clear();
    }

    private void deliver(final Consumer<String> listener, final String message) {
        try {
            listener.accept(message);
        } catch (final RuntimeException e) {
            // Отвалившийся сокет не должен ронять рассылку остальным.
            log.debug("Не удалось доставить сообщение за столом {}: {}", tableId, e.toString());
        }
    }
}
