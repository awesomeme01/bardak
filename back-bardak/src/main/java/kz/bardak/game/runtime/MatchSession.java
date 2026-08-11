package kz.bardak.game.runtime;

import java.util.LinkedHashSet;
import java.util.List;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;
import kz.bardak.game.rules.DealCommand;
import kz.bardak.game.rules.MatchEngine;
import kz.bardak.game.rules.MatchResult;
import kz.bardak.game.rules.MatchState;
import kz.bardak.game.rules.PlayerView;
import kz.bardak.game.rules.StateProjection;

/**
 * Идущий матч за одним столом.
 *
 * <p>⭐ Здесь же живёт единственное отображение «место за столом → место в движке».
 * Движок нумерует игроков подряд от нуля, а за столом места могут идти с пропусками —
 * кто-то встал до начала. Считать это отображение в двух местах означает рано или поздно
 * разойтись и посадить игрока не туда.
 *
 * <p>Объект не потокобезопасен намеренно: он живёт на потоке своего стола (ADR-007).
 */
public final class MatchSession {

    private final UUID tableId;
    private final UUID matchId;
    private final List<UUID> seatUsers;
    private final MatchEngine engine;
    private final StateProjection projection;

    private MatchState state;

    /** Сквозной номер события по матчу. Живёт на потоке стола, поэтому просто int. */
    private int lastSeq;

    /**
     * ⭐ Уже применённые команды.
     *
     * <p>Клиент переотправляет команду после разрыва — он не знает, дошла ли предыдущая.
     * Без дедупликации ход применился бы дважды: карта ушла бы со стола дважды или пас
     * закрыл бы чужой раунд. Помним последние {@value #REMEMBERED_COMMANDS} — этого
     * с запасом хватает на любую очередь переотправки, а расти бесконечно набору незачем.
     */
    private final LinkedHashSet<String> appliedCommands = new LinkedHashSet<>();

    public MatchSession(final UUID tableId, final UUID matchId, final List<UUID> seatUsers,
                        final MatchEngine engine, final StateProjection projection,
                        final MatchState state) {
        this.tableId = Objects.requireNonNull(tableId, "tableId");
        this.matchId = Objects.requireNonNull(matchId, "matchId");
        this.seatUsers = List.copyOf(Objects.requireNonNull(seatUsers, "seatUsers"));
        this.engine = Objects.requireNonNull(engine, "engine");
        this.projection = Objects.requireNonNull(projection, "projection");
        this.state = Objects.requireNonNull(state, "state");
    }

    private static final int REMEMBERED_COMMANDS = 200;

    public UUID tableId() {
        return tableId;
    }

    /** Команда с этим идентификатором уже применялась. */
    public boolean alreadyApplied(final String commandId) {
        return commandId != null && appliedCommands.contains(commandId);
    }

    public void remember(final String commandId) {
        if (commandId == null) {
            return;
        }
        appliedCommands.add(commandId);
        final var iterator = appliedCommands.iterator();
        while (appliedCommands.size() > REMEMBERED_COMMANDS && iterator.hasNext()) {
            iterator.next();
            iterator.remove();
        }
    }

    public UUID matchId() {
        return matchId;
    }

    public int lastSeq() {
        return lastSeq;
    }

    public int nextSeq() {
        return lastSeq + 1;
    }

    public void lastSeq(final int value) {
        this.lastSeq = value;
    }

    public MatchState state() {
        return state;
    }

    public List<UUID> players() {
        return seatUsers;
    }

    /** Место игрока в движке. Пусто — этот игрок за столом не играет (зашёл смотреть). */
    public Optional<Integer> seatOf(final UUID userId) {
        final int seat = seatUsers.indexOf(userId);
        return seat < 0 ? Optional.empty() : Optional.of(seat);
    }

    public UUID userAt(final int seatNo) {
        return seatUsers.get(seatNo);
    }

    /** Персональная проекция: чужих карт в ней нет физически (ADR-026). */
    public PlayerView viewFor(final UUID userId) {
        final int seat = seatOf(userId).orElseThrow(() ->
                new IllegalArgumentException("Игрок не за этим столом: " + userId));
        return projection.project(state.deal(), seat);
    }

    /** Применить команду. Состояние меняется, только если движок её принял. */
    public MatchResult apply(final DealCommand command) {
        final MatchResult result = engine.apply(state, command);
        if (result instanceof MatchResult.Applied applied) {
            state = applied.state();
        }
        return result;
    }
}
