package kz.bardak.game.runtime;

import com.fasterxml.jackson.databind.ObjectMapper;
import java.security.SecureRandom;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import kz.bardak.common.web.ApiException;
import kz.bardak.game.rules.AttackOrderPolicy;
import kz.bardak.game.rules.DiceResolver;
import kz.bardak.game.rules.MatchEngine;
import kz.bardak.game.rules.RulesConfig;
import kz.bardak.game.rules.StateProjection;
import kz.bardak.game.protocol.MatchStateCodec;
import kz.bardak.history.MatchLog;
import kz.bardak.history.domain.MatchPlayerRecord;
import kz.bardak.history.domain.MatchPlayerRepository;
import kz.bardak.history.domain.MatchRecord;
import kz.bardak.lobby.LobbyService;
import kz.bardak.lobby.domain.GameTable;
import kz.bardak.lobby.domain.TablePlayer;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;

/**
 * Запуск и хранение идущих матчей.
 *
 * <p>Матч живёт в памяти на своём столе; в БД уезжает лог событий (M4, дальше по плану).
 * Пока стол один инстанс — этого достаточно, восстановление из лога появится вместе
 * с персистом.
 */
@Service
public class MatchService {

    private static final Logger log = LoggerFactory.getLogger(MatchService.class);

    private final LobbyService lobby;
    private final MatchLog matchLog;
    private final MatchPlayerRepository matchPlayers;
    private final RulesConfigCodec rulesCodec;
    private final MatchStateCodec stateCodec;
    private final Map<UUID, MatchSession> sessions = new ConcurrentHashMap<>();
    private final SecureRandom secureRandom = new SecureRandom();

    public MatchService(final LobbyService lobby, final MatchLog matchLog,
                        final MatchPlayerRepository matchPlayers, final ObjectMapper objectMapper) {
        this.lobby = Objects.requireNonNull(lobby, "lobby");
        this.matchLog = Objects.requireNonNull(matchLog, "matchLog");
        this.matchPlayers = Objects.requireNonNull(matchPlayers, "matchPlayers");
        this.rulesCodec = new RulesConfigCodec(objectMapper);
        this.stateCodec = new MatchStateCodec(objectMapper);
    }

    public MatchStateCodec stateCodec() {
        return stateCodec;
    }

    public Optional<MatchSession> find(final UUID tableId) {
        return Optional.ofNullable(sessions.get(tableId)).or(() -> restore(tableId));
    }

    /**
     * ⭐ Поднять матч из снимка после рестарта.
     *
     * <p>В памяти матча нет, а в базе он числится идущим — значит, сервер перезапускали.
     * Игроки за столом ничего об этом знать не должны: они переподключатся и продолжат
     * с того же места (ADR-004).
     */
    private Optional<MatchSession> restore(final UUID tableId) {
        return matchLog.activeMatchFor(tableId).flatMap(record ->
                matchLog.latestSnapshot(record.id()).map(snapshot -> {
                    final GameTable table = lobby.byId(tableId);
                    final RulesConfig config = rulesCodec.parse(table.rulesConfig());
                    final MatchSession session = new MatchSession(tableId, record.id(),
                            seatsOfMatch(record.id(), tableId),
                            engineFor(config), projectionFor(config), config,
                            stateCodec.decode(snapshot.state()));
                    session.lastSeq(snapshot.seq());
                    sessions.put(tableId, session);
                    log.info("Матч за столом {} поднят из снимка на seq={}", tableId, snapshot.seq());
                    return session;
                }));
    }

    /**
     * ⭐ Места берутся из матча, а не из лобби.
     *
     * <p>Лобби живёт своей жизнью: кто-то встал, кто-то сел, порядок мест изменился.
     * Матч же раздан по местам, зафиксированным на старте (§1.2), и в снимке состояния
     * места — это индексы. Возьми их из лобби — и после рестарта игрок получил бы чужую
     * руку, причём молча: расклад выглядел бы совершенно правдоподобно.
     */
    private List<UUID> seatsOfMatch(final UUID matchId, final UUID tableId) {
        final List<UUID> seats = matchPlayers.findByMatchIdOrderBySeatNo(matchId).stream()
                .map(MatchPlayerRecord::userId)
                .toList();
        if (!seats.isEmpty()) {
            return seats;
        }
        // Матчи, начатые до появления match_players, восстанавливаются по-старому.
        log.warn("У матча {} нет записанных мест, беру порядок из лобби", matchId);
        return lobby.seats(tableId).stream().map(TablePlayer::userId).toList();
    }

    private MatchEngine engineFor(final RulesConfig config) {
        return new MatchEngine(config, new AttackOrderPolicy.BardakStrictNeighbours(),
                new DiceResolver.Seeded());
    }

    private StateProjection projectionFor(final RulesConfig config) {
        return new StateProjection(config, new kz.bardak.game.rules.DealEngine(config,
                new AttackOrderPolicy.BardakStrictNeighbours(), new DiceResolver.Seeded()));
    }

    /**
     * Начать матч за столом.
     *
     * <p>⭐ Порядок мест в движке — порядок мест за столом. Он определяет очерёдность хода
     * и фиксируется на весь матч (§1.2), поэтому берётся один раз здесь.
     */
    public MatchSession start(final UUID tableId) {
        if (sessions.containsKey(tableId)) {
            throw new ApiException(HttpStatus.CONFLICT, "MATCH_ALREADY_STARTED", "Матч уже идёт");
        }
        if (!lobby.isReadyToStart(tableId)) {
            throw new ApiException(HttpStatus.CONFLICT, "TABLE_NOT_READY",
                    "Не все за столом готовы");
        }
        final GameTable table = lobby.byId(tableId);
        final List<UUID> seatUsers = lobby.seats(tableId).stream()
                .map(TablePlayer::userId)
                .toList();

        final RulesConfig config = rulesCodec.parse(table.rulesConfig());
        final MatchEngine engine = engineFor(config);
        // ⭐ Seed матча из SecureRandom, а дальше всё случайное выводится из него (§6):
        // матч воспроизводится по паре «seed + последовательность команд».
        final long matchSeed = secureRandom.nextLong();

        // ⭐ rules_snapshot обязателен: правила стола могут поменяться, а матч должен
        // остаться интерпретируемым ровно по тем, по которым игрался.
        final MatchRecord record = matchLog.startMatch(tableId, seatUsers.size(), matchSeed,
                table.rulesConfig());

        final MatchSession session = new MatchSession(tableId, record.id(), seatUsers, engine,
                projectionFor(config), config, engine.startMatch(seatUsers.size(), matchSeed));

        sessions.put(tableId, session);
        lobby.startMatch(tableId);
        log.info("Матч за столом {} начат: игроков={}, seed скрыт до конца матча", tableId, seatUsers.size());
        return session;
    }

    public void finish(final UUID tableId) {
        sessions.remove(tableId);
    }
}
