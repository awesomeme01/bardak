package kz.bardak.game.ws;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import java.util.function.Consumer;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.common.web.ApiException;
import kz.bardak.game.protocol.GameProtocol;
import kz.bardak.game.runtime.MatchService;
import kz.bardak.game.runtime.MatchSession;
import kz.bardak.game.runtime.TableRegistry;
import kz.bardak.game.runtime.TableRuntime;
import kz.bardak.game.rules.DealCommand;
import kz.bardak.game.rules.DealEvent;
import kz.bardak.game.rules.MatchResult;
import kz.bardak.history.MatchLog;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

/**
 * Игровые команды поверх WebSocket.
 *
 * <p>⭐ После каждой принятой команды каждому игроку уходит <b>своя</b> проекция состояния
 * (ADR-002): одно общее сообщение здесь невозможно в принципе — в нём были бы чужие карты.
 *
 * <p>События рассылаются отдельно от снимка и фильтруются по видимости: вскрытие скрытой
 * карты видит только владелец (§1.8).
 */
@Component
public class GameCommandHandler {

    private static final Logger log = LoggerFactory.getLogger(GameCommandHandler.class);
    private static final List<String> COMMANDS = List.of("MATCH_START", "PLAY_CARD", "PASS", "TAKE",
            "TRANSFER", "HANG_CARD", "HANG_SKIP", "CHOOSE_TRUMP", "REVEAL_FACE_DOWN", "STATE_REQUEST");

    private final MatchService matches;
    private final MatchLog matchLog;
    private final TableRegistry registry;
    private final UserRepository users;
    private final ObjectMapper objectMapper;

    public GameCommandHandler(final MatchService matches, final MatchLog matchLog,
                              final TableRegistry registry, final UserRepository users,
                              final ObjectMapper objectMapper) {
        this.matches = Objects.requireNonNull(matches, "matches");
        this.matchLog = Objects.requireNonNull(matchLog, "matchLog");
        this.registry = Objects.requireNonNull(registry, "registry");
        this.users = Objects.requireNonNull(users, "users");
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper");
    }

    public boolean handles(final String type) {
        return COMMANDS.contains(type);
    }

    public void handle(final Envelope command, final UUID userId, final Consumer<String> sender) {
        final UUID tableId = tableIdOf(command);
        if (tableId == null) {
            sender.accept(serialize(error(command, "TABLE_ID_INVALID", "Стол указан неверно")));
            return;
        }
        final TableRuntime runtime = registry.runtimeFor(tableId);
        runtime.submit(() -> execute(command, userId, tableId, runtime, sender));
    }

    private void execute(final Envelope command, final UUID userId, final UUID tableId,
                         final TableRuntime runtime, final Consumer<String> sender) {
        try {
            if ("MATCH_START".equals(command.type())) {
                final MatchSession session = matches.start(tableId);
                runtime.subscribe(userId, sender);
                broadcast(runtime, session, List.of());
                return;
            }
            final MatchSession session = matches.find(tableId).orElseThrow(() ->
                    new ApiException(org.springframework.http.HttpStatus.CONFLICT, "NO_MATCH",
                            "За этим столом матч не идёт"));

            if ("STATE_REQUEST".equals(command.type())) {
                runtime.subscribe(userId, sender);
                sender.accept(serialize(stateSync(session, userId)));
                return;
            }

            final int seatNo = session.seatOf(userId).orElseThrow(() ->
                    new ApiException(org.springframework.http.HttpStatus.FORBIDDEN, "NOT_A_PLAYER",
                            "Ты не играешь за этим столом"));
            final DealCommand move = GameProtocol.toCommand(command.type(), seatNo, payloadOf(command));
            final MatchResult result = session.apply(move);

            if (result instanceof MatchResult.Rejected rejected) {
                // Отклонённая попытка — часть истории стола, хотя состояние не меняет (§2.1).
                matchLog.appendRejected(session.matchId(), session.nextSeq(),
                        session.state().dealNo(), seatNo, command.type(), rejected.reason().name());
                session.lastSeq(session.nextSeq());
                sender.accept(serialize(error(command, rejected.reason().name(), "Ход отклонён")));
                return;
            }
            final List<DealEvent> events = ((MatchResult.Applied) result).events();
            // ⭐ Сначала лог, потом рассылка (ADR-004): иначе после падения между ними
            // клиенты видели бы ход, которого в истории нет.
            session.lastSeq(matchLog.append(session.matchId(), session.nextSeq(),
                    session.state().dealNo(), events));
            matchLog.dealsPlayed(session.matchId(), session.state().results().size());
            broadcast(runtime, session, events);
        } catch (final ApiException e) {
            sender.accept(serialize(error(command, e.code(), e.getMessage())));
        } catch (final IllegalArgumentException e) {
            sender.accept(serialize(error(command, "BAD_COMMAND", e.getMessage())));
        } catch (final RuntimeException e) {
            log.error("Игровая команда {} за столом {} упала", command.type(), tableId, e);
            sender.accept(serialize(error(command, "INTERNAL_ERROR", "Что-то пошло не так")));
        }
    }

    /**
     * Разослать события и персональные снимки.
     *
     * <p>Порядок важен: сначала события — что произошло, потом снимок — как теперь.
     * Иначе клиент увидит новое состояние раньше причины и не сможет его анимировать.
     */
    private void broadcast(final TableRuntime runtime, final MatchSession session,
                           final List<DealEvent> events) {
        for (final UUID player : session.players()) {
            for (final DealEvent event : events) {
                if (isVisibleTo(event, session, player)) {
                    runtime.sendTo(player, serialize(eventMessage(session, event)));
                }
            }
            runtime.sendTo(player, serialize(stateSync(session, player)));
        }
    }

    private boolean isVisibleTo(final DealEvent event, final MatchSession session, final UUID player) {
        return event.privateToSeat()
                .map(seat -> session.seatOf(player).filter(own -> own == seat).isPresent())
                .orElse(true);
    }

    private Envelope stateSync(final MatchSession session, final UUID userId) {
        final var dto = GameProtocol.toDto(session.viewFor(userId), session.tableId(),
                session.state().dealNo(), session::userAt, seat -> displayName(session.userAt(seat)));
        return Envelope.event("STATE_SYNC", null, session.tableId().toString(),
                objectMapper.valueToTree(dto));
    }

    private Envelope eventMessage(final MatchSession session, final DealEvent event) {
        return Envelope.event(GameProtocol.eventType(event), null, session.tableId().toString(),
                objectMapper.valueToTree(GameProtocol.toEventPayload(event)));
    }

    private String displayName(final UUID userId) {
        return users.findById(userId).map(User::displayName).orElse("—");
    }

    @SuppressWarnings("unchecked")
    private Map<String, Object> payloadOf(final Envelope command) {
        if (command.payload() == null || command.payload().isNull()) {
            return Map.of();
        }
        return objectMapper.convertValue(command.payload(), Map.class);
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
            throw new IllegalStateException("Игровое сообщение не сериализуется", e);
        }
    }
}
