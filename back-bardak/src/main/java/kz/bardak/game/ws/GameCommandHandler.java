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
import kz.bardak.game.runtime.GameProperties;
import kz.bardak.game.runtime.MatchService;
import kz.bardak.game.runtime.TimeoutPolicy;
import kz.bardak.game.runtime.TurnClock;
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
            "TRANSFER", "HANG_CARD", "HANG_SKIP", "CHOOSE_TRUMP", "REVEAL_FACE_DOWN", "STATE_REQUEST",
            "RESYNC");

    private final MatchService matches;
    private final MatchLog matchLog;
    private final TableRegistry registry;
    private final UserRepository users;
    private final ObjectMapper objectMapper;
    private final TurnClock clock;
    private final GameProperties properties;

    public GameCommandHandler(final MatchService matches, final MatchLog matchLog,
                              final TableRegistry registry, final UserRepository users,
                              final ObjectMapper objectMapper, final TurnClock clock,
                              final GameProperties properties) {
        this.matches = Objects.requireNonNull(matches, "matches");
        this.matchLog = Objects.requireNonNull(matchLog, "matchLog");
        this.clock = Objects.requireNonNull(clock, "clock");
        this.properties = Objects.requireNonNull(properties, "properties");
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
                restartTurnClock(runtime, session);
                return;
            }
            final MatchSession session = matches.find(tableId).orElseThrow(() ->
                    new ApiException(org.springframework.http.HttpStatus.CONFLICT, "NO_MATCH",
                            "За этим столом матч не идёт"));

            if ("RESYNC".equals(command.type())) {
                runtime.subscribe(userId, sender);
                resync(session, userId, sender, command);
                if (session.seatOf(userId).isPresent()) {
                    resumeAfterReconnect(runtime, session);
                }
                return;
            }

            if ("STATE_REQUEST".equals(command.type())) {
                runtime.subscribe(userId, sender);
                sender.accept(serialize(stateSync(session, userId)));
                if (session.seatOf(userId).isPresent()) {
                    resumeAfterReconnect(runtime, session);
                }
                return;
            }

            // ⭐ Клиент переотправляет команду после разрыва: он не знает, дошла ли она.
            // Повтор не применяем, но состояние отдаём — иначе он останется в неведении.
            if (session.alreadyApplied(command.id())) {
                sender.accept(serialize(stateSync(session, userId)));
                return;
            }

            final int seatNo = session.seatOf(userId).orElseThrow(() ->
                    new ApiException(org.springframework.http.HttpStatus.FORBIDDEN, "NOT_A_PLAYER",
                            "Ты не играешь за этим столом"));
            final DealCommand move = GameProtocol.toCommand(command.type(), seatNo, payloadOf(command));
            final MatchResult result = session.apply(move);
            session.remember(command.id());

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
            restartTurnClock(runtime, session);
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
     * Догнать пропущенное после разрыва.
     *
     * <p>⭐ События берутся из лога, но <b>уже отфильтрованные по видимости</b>: сырой лог
     * содержит скрытую информацию и наружу не отдаётся никогда. Следом уходит полный
     * снимок — так клиент сходится, даже если пропустил больше, чем помнит сервер.
     */
    private void resync(final MatchSession session, final UUID userId, final Consumer<String> sender,
                        final Envelope command) {
        final int seat = session.seatOf(userId).orElse(-1);
        final int lastSeq = command.payload() == null ? 0
                : command.payload().path("lastSeq").asInt(0);
        if (seat >= 0) {
            for (final var missed : matchLog.since(session.matchId(), lastSeq, seat)) {
                sender.accept(serialize(new Envelope(Envelope.PROTOCOL_VERSION, null, missed.type(),
                        session.tableId().toString(), missed.seq(),
                        java.time.Instant.now().toEpochMilli(), readPayload(missed.payload()))));
            }
        }
        sender.accept(serialize(stateSync(session, userId)));
    }

    private com.fasterxml.jackson.databind.JsonNode readPayload(final String payload) {
        try {
            return objectMapper.readTree(payload);
        } catch (final com.fasterxml.jackson.core.JsonProcessingException e) {
            return objectMapper.createObjectNode();
        }
    }

    /**
     * Перезапустить таймер хода.
     *
     * <p>⭐ Срабатывание таймера не делает ничего игрового само: оно кладёт команду
     * в очередь стола (ADR-007). Иначе автодействие пришло бы с чужого потока и могло
     * бы пересечься с настоящим ходом игрока, который успел в последнюю секунду.
     */
    private void restartTurnClock(final TableRuntime runtime, final MatchSession session) {
        if (session.state().isOver()) {
            clock.cancel(session.tableId());
            return;
        }
        if (TimeoutPolicy.seatOnTheClock(session.state().deal()).isEmpty()) {
            clock.cancel(session.tableId());
            return;
        }
        clock.start(session.tableId(), properties.turnTimeout(),
                () -> runtime.submit(() -> applyTimeout(runtime, session)));
    }

    /** Ход не сделан за отведённое время — сервер делает самое безобидное действие (§5.1). */
    private void applyTimeout(final TableRuntime runtime, final MatchSession session) {
        TimeoutPolicy.autoActionFor(session.state().deal()).ifPresent(auto -> {
            final MatchResult result = session.apply(auto);
            if (result instanceof MatchResult.Applied applied) {
                session.lastSeq(matchLog.append(session.matchId(), session.nextSeq(),
                        session.state().dealNo(), applied.events()));
                runtime.broadcast(serialize(Envelope.event("TURN_TIMEOUT", null,
                        session.tableId().toString(),
                        objectMapper.createObjectNode().put("seatNo", auto.seatNo()))));
                broadcast(runtime, session, applied.events());
                restartTurnClock(runtime, session);
            }
        });
    }

    /**
     * Игрок пропал: матч встаёт на паузу, таймер хода <b>останавливается</b> (§5.2).
     * Не вернулся за отведённое время — матч отменяется, рейтинг не трогается (§5.3).
     */
    public void onDisconnect(final UUID tableId, final UUID userId) {
        matches.find(tableId).ifPresent(session -> {
            if (session.seatOf(userId).isEmpty()) {
                return;
            }
            registry.find(tableId).ifPresent(runtime -> runtime.submit(() -> {
                final java.time.Duration left = clock.pause(tableId);
                runtime.broadcast(serialize(Envelope.event("MATCH_PAUSED", null, tableId.toString(),
                        objectMapper.createObjectNode()
                                .put("userId", userId.toString())
                                .put("turnMillisLeft", left.toMillis())
                                .put("graceSeconds", properties.disconnectGrace().toSeconds()))));
                clock.scheduleAbort(tableId, properties.disconnectGrace(),
                        () -> runtime.submit(() -> abort(runtime, session, userId)));
            }));
        });
    }

    /** Игрок вернулся: продолжаем с остатка, а не с полного хода. */
    private void resumeAfterReconnect(final TableRuntime runtime, final MatchSession session) {
        clock.cancelAbort(session.tableId());
        clock.resume(session.tableId());
        runtime.broadcast(serialize(Envelope.event("MATCH_RESUMED", null,
                session.tableId().toString(), objectMapper.createObjectNode())));
    }

    private void abort(final TableRuntime runtime, final MatchSession session, final UUID userId) {
        clock.cancel(session.tableId());
        matchLog.abort(session.matchId(), "Игрок не вернулся за отведённое время");
        matches.finish(session.tableId());
        runtime.broadcast(serialize(Envelope.event("MATCH_ABORTED", null,
                session.tableId().toString(),
                objectMapper.createObjectNode().put("userId", userId.toString()))));
    }

    /**
     * Разослать события и персональные снимки.
     *
     * <p>Порядок важен: сначала события — что произошло, потом снимок — как теперь.
     * Иначе клиент увидит новое состояние раньше причины и не сможет его анимировать.
     */
    private void broadcast(final TableRuntime runtime, final MatchSession session,
                           final List<DealEvent> events) {
        final int firstSeq = session.lastSeq() - events.size() + 1;
        for (final UUID player : session.players()) {
            int seq = firstSeq;
            for (final DealEvent event : events) {
                if (isVisibleTo(event, session, player)) {
                    runtime.sendTo(player, serialize(eventMessage(session, event, seq)));
                }
                seq++;
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

    /**
     * ⭐ Событие уходит с номером: по нему клиент понимает, что пропустил, и просит `RESYNC`.
     * Номер сквозной по матчу, а не по раздаче — счётчик у клиента один.
     */
    private Envelope eventMessage(final MatchSession session, final DealEvent event, final int seq) {
        return new Envelope(Envelope.PROTOCOL_VERSION, null, GameProtocol.eventType(event),
                session.tableId().toString(), seq, java.time.Instant.now().toEpochMilli(),
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
