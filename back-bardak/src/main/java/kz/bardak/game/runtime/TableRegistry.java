package kz.bardak.game.runtime;

import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentMap;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

/**
 * Реестр живых столов. Стол поднимается по первому подключению и выгружается, когда
 * за ним никого не осталось: держать поток на пустой стол незачем.
 */
@Component
public class TableRegistry {

    private static final Logger log = LoggerFactory.getLogger(TableRegistry.class);

    private final ConcurrentMap<UUID, TableRuntime> tables = new ConcurrentHashMap<>();

    public TableRuntime runtimeFor(final UUID tableId) {
        return tables.computeIfAbsent(tableId, id -> {
            log.debug("Поднимаю стол {}", id);
            return new TableRuntime(id);
        });
    }

    public Optional<TableRuntime> find(final UUID tableId) {
        return Optional.ofNullable(tables.get(tableId));
    }

    /** Отписать игрока и выгрузить стол, если он опустел. */
    public void unsubscribe(final UUID tableId, final UUID userId) {
        final TableRuntime runtime = tables.get(tableId);
        if (runtime == null) {
            return;
        }
        runtime.unsubscribe(userId);
        if (!runtime.hasListeners()) {
            tables.remove(tableId, runtime);
            runtime.close();
            log.debug("Стол {} опустел и выгружен", tableId);
        }
    }

    public int size() {
        return tables.size();
    }
}
