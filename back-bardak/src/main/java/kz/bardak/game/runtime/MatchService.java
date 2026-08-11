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
import kz.bardak.history.MatchLog;
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
    private final RulesConfigCodec rulesCodec;
    private final Map<UUID, MatchSession> sessions = new ConcurrentHashMap<>();
    private final SecureRandom secureRandom = new SecureRandom();

    public MatchService(final LobbyService lobby, final MatchLog matchLog,
                        final ObjectMapper objectMapper) {
        this.lobby = Objects.requireNonNull(lobby, "lobby");
        this.matchLog = Objects.requireNonNull(matchLog, "matchLog");
        this.rulesCodec = new RulesConfigCodec(objectMapper);
    }

    public Optional<MatchSession> find(final UUID tableId) {
        return Optional.ofNullable(sessions.get(tableId));
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
        final MatchEngine engine = new MatchEngine(config,
                new AttackOrderPolicy.BardakStrictNeighbours(), new DiceResolver.Seeded());
        // ⭐ Seed матча из SecureRandom, а дальше всё случайное выводится из него (§6):
        // матч воспроизводится по паре «seed + последовательность команд».
        final long matchSeed = secureRandom.nextLong();

        // ⭐ rules_snapshot обязателен: правила стола могут поменяться, а матч должен
        // остаться интерпретируемым ровно по тем, по которым игрался.
        final MatchRecord record = matchLog.startMatch(tableId, seatUsers.size(), matchSeed,
                table.rulesConfig());

        final MatchSession session = new MatchSession(tableId, record.id(), seatUsers, engine,
                new StateProjection(config, new kz.bardak.game.rules.DealEngine(config,
                        new AttackOrderPolicy.BardakStrictNeighbours(), new DiceResolver.Seeded())),
                engine.startMatch(seatUsers.size(), matchSeed));

        sessions.put(tableId, session);
        lobby.startMatch(tableId);
        log.info("Матч за столом {} начат: игроков={}, seed скрыт до конца матча", tableId, seatUsers.size());
        return session;
    }

    public void finish(final UUID tableId) {
        sessions.remove(tableId);
    }
}
