package kz.bardak.lobby.ws;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.util.List;
import java.util.Objects;
import java.util.UUID;
import java.util.function.Consumer;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.common.web.ApiException;
import kz.bardak.game.runtime.TableRegistry;
import kz.bardak.game.runtime.TableRuntime;
import kz.bardak.game.ws.Envelope;
import kz.bardak.lobby.LobbyService;
import kz.bardak.lobby.domain.TablePlayer;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

/**
 * Команды лобби: сесть за стол, встать, готовность.
 *
 * <p>Вход за стол идёт по WebSocket, а не REST (`02-architecture.md`): он меняет живое
 * состояние, которое сразу видят остальные, — и потому обязан ехать по тому же каналу,
 * что и события, иначе порядок «сел» и «все увидели» ничем не гарантирован.
 *
 * <p>⭐ Вся работа выполняется на потоке стола (ADR-007): команды одного стола
 * выстраиваются в очередь, и гонок между «сел» и «встал» не существует.
 */
@Component
public class LobbyCommandHandler {

    private static final Logger log = LoggerFactory.getLogger(LobbyCommandHandler.class);
    private static final List<String> COMMANDS = List.of("TABLE_JOIN", "TABLE_LEAVE", "TABLE_READY");

    private final LobbyService lobby;
    private final UserRepository users;
    private final TableRegistry registry;
    private final ObjectMapper objectMapper;

    public LobbyCommandHandler(final LobbyService lobby, final UserRepository users,
                               final TableRegistry registry, final ObjectMapper objectMapper) {
        this.lobby = Objects.requireNonNull(lobby, "lobby");
        this.users = Objects.requireNonNull(users, "users");
        this.registry = Objects.requireNonNull(registry, "registry");
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper");
    }

    public boolean handles(final String type) {
        return COMMANDS.contains(type);
    }

    /**
     * Обработать команду лобби.
     *
     * @param sender куда писать лично отправителю; он же подписывается на события стола
     */
    public void handle(final Envelope command, final UUID userId, final Consumer<String> sender) {
        final UUID tableId = tableIdOf(command);
        if (tableId == null) {
            sender.accept(serialize(error(command, "TABLE_ID_INVALID", "Стол указан неверно")));
            return;
        }
        final TableRuntime runtime = registry.runtimeFor(tableId);
        runtime.submit(() -> execute(command, userId, tableId, runtime, sender));
    }

    /** Обрыв соединения: снимаем подписку и сообщаем остальным, что игрок не на связи. */
    public void onDisconnect(final UUID tableId, final UUID userId) {
        registry.find(tableId).ifPresent(runtime -> runtime.submit(() -> {
            registry.unsubscribe(tableId, userId);
            runtime.broadcast(serialize(event("PLAYER_OFFLINE", tableId, payload(userId, null))));
        }));
    }

    private void execute(final Envelope command, final UUID userId, final UUID tableId,
                         final TableRuntime runtime, final Consumer<String> sender) {
        try {
            switch (command.type()) {
                case "TABLE_JOIN" -> {
                    final TablePlayer seat = lobby.join(tableId, userId);
                    runtime.subscribe(userId, sender);
                    runtime.broadcast(serialize(event("PLAYER_JOINED", tableId, payload(userId, seat))));
                }
                case "TABLE_LEAVE" -> {
                    lobby.leave(tableId, userId);
                    runtime.broadcast(serialize(event("PLAYER_LEFT", tableId, payload(userId, null))));
                    registry.unsubscribe(tableId, userId);
                }
                case "TABLE_READY" -> {
                    final boolean ready = command.payload() == null
                            || command.payload().path("ready").asBoolean(true);
                    final TablePlayer seat = lobby.setReady(tableId, userId, ready);
                    runtime.broadcast(serialize(event("PLAYER_READY", tableId, payload(userId, seat))));
                }
                default -> sender.accept(serialize(
                        error(command, "UNKNOWN_COMMAND", "Неизвестная команда лобби")));
            }
        } catch (final ApiException e) {
            sender.accept(serialize(error(command, e.code(), e.getMessage())));
        } catch (final RuntimeException e) {
            log.error("Команда лобби {} за столом {} упала", command.type(), tableId, e);
            sender.accept(serialize(error(command, "INTERNAL_ERROR", "Что-то пошло не так")));
        }
    }

    private ObjectNode payload(final UUID userId, final TablePlayer seat) {
        final ObjectNode payload = objectMapper.createObjectNode();
        payload.put("userId", userId.toString());
        users.findById(userId).map(User::displayName)
                .ifPresent(displayName -> payload.put("displayName", displayName));
        if (seat != null) {
            payload.put("seatNo", seat.seatNo());
            payload.put("ready", seat.isReady());
        }
        return payload;
    }

    private UUID tableIdOf(final Envelope command) {
        if (command.tableId() == null || command.tableId().isBlank()) {
            return null;
        }
        try {
            return UUID.fromString(command.tableId());
        } catch (final IllegalArgumentException e) {
            return null;
        }
    }

    private Envelope event(final String type, final UUID tableId, final ObjectNode payload) {
        return Envelope.event(type, null, tableId.toString(), payload);
    }

    private Envelope error(final Envelope command, final String code, final String message) {
        final ObjectNode payload = objectMapper.createObjectNode()
                .put("code", code)
                .put("message", message);
        return Envelope.event("ERROR", command.id(), command.tableId(), payload);
    }

    private String serialize(final Envelope envelope) {
        try {
            return objectMapper.writeValueAsString(envelope);
        } catch (final JsonProcessingException e) {
            throw new IllegalStateException("Событие лобби не сериализуется", e);
        }
    }
}
