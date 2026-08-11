package kz.bardak.lobby;

import java.security.SecureRandom;
import java.time.Clock;
import java.util.List;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;
import kz.bardak.common.web.ApiException;
import kz.bardak.lobby.domain.GameTable;
import kz.bardak.lobby.domain.GameTableRepository;
import kz.bardak.lobby.domain.TablePlayer;
import kz.bardak.lobby.domain.TablePlayerRepository;
import kz.bardak.lobby.domain.TableStatus;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.http.HttpStatus;
import org.springframework.orm.ObjectOptimisticLockingFailureException;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

/**
 * Столы: создание, список, посадка и выход.
 *
 * <p>⭐ Главное здесь — посадка за последнее место. Двое могут нажать «сесть» одновременно,
 * и проверка «есть ли свободное место» у обоих пройдёт. Поэтому защиты две:
 * уникальный индекс {@code (table_id, seat_no)} в БД и повтор попытки на нарушении —
 * второй игрок либо займёт другое место, либо получит {@code TABLE_FULL}.
 */
@Service
public class LobbyService {

    /** Буквы кода приглашения: без похожих друг на друга — код диктуют голосом. */
    private static final String CODE_ALPHABET = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789";
    private static final int CODE_LENGTH = 6;
    private static final int SEAT_ATTEMPTS = 5;

    private final GameTableRepository tables;
    private final TablePlayerRepository players;
    private final SeatAllocator seatAllocator;
    private final Clock clock;
    private final SecureRandom random = new SecureRandom();

    public LobbyService(final GameTableRepository tables, final TablePlayerRepository players,
                        final SeatAllocator seatAllocator, final Clock clock) {
        this.tables = Objects.requireNonNull(tables, "tables");
        this.players = Objects.requireNonNull(players, "players");
        this.seatAllocator = Objects.requireNonNull(seatAllocator, "seatAllocator");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    @Transactional(readOnly = true)
    public List<GameTable> openTables() {
        return tables.findByStatusAndIsPrivateFalseOrderByCreatedAtDesc(TableStatus.WAITING);
    }

    @Transactional(readOnly = true)
    public GameTable byId(final UUID tableId) {
        return tables.findById(tableId).orElseThrow(LobbyService::tableNotFound);
    }

    @Transactional(readOnly = true)
    public GameTable byCode(final String code) {
        return tables.findByCode(code.toUpperCase()).orElseThrow(LobbyService::tableNotFound);
    }

    @Transactional(readOnly = true)
    public List<TablePlayer> seats(final UUID tableId) {
        return players.findByTableIdOrderBySeatNo(tableId);
    }

    public GameTable create(final UUID hostUserId, final String name, final int maxPlayers,
                            final UUID cardSetId, final UUID themeId, final String rulesConfig,
                            final boolean isPrivate) {
        // save репозитория транзакционен сам по себе; своей @Transactional здесь быть
        // не должно — вызов из этого же класса всё равно прошёл бы мимо прокси.
        final GameTable table = tables.save(new GameTable(UUID.randomUUID(), newCode(), name,
                hostUserId, maxPlayers, cardSetId, themeId, rulesConfig, isPrivate));
        join(table.id(), hostUserId);
        return table;
    }

    /**
     * Посадить игрока за стол. Возвращает занятое место.
     *
     * <p>⚠️ Метод намеренно <b>без</b> {@code @Transactional}: каждая попытка занять место
     * идёт в своей транзакции ({@link SeatAllocator}), потому что после нарушения
     * уникального индекса продолжать в той же транзакции нельзя.
     */
    public TablePlayer join(final UUID tableId, final UUID userId) {
        final GameTable table = byId(tableId);
        if (!table.isOpenForJoin()) {
            throw new ApiException(HttpStatus.CONFLICT, "TABLE_NOT_OPEN",
                    "За этот стол уже нельзя сесть");
        }
        final Optional<TablePlayer> existing = players.findByTableIdAndUserId(tableId, userId);
        if (existing.isPresent()) {
            return existing.get();
        }
        for (int attempt = 0; attempt < SEAT_ATTEMPTS; attempt++) {
            try {
                final TablePlayer seat = seatAllocator.allocate(tableId, userId, table.maxPlayers());
                if (seat == null) {
                    throw new ApiException(HttpStatus.CONFLICT, "TABLE_FULL",
                            "За столом нет свободных мест");
                }
                return seat;
            } catch (final DataIntegrityViolationException | ObjectOptimisticLockingFailureException e) {
                // Место увели между выбором и вставкой — считаем расклад заново.
            }
        }
        throw new ApiException(HttpStatus.CONFLICT, "TABLE_FULL", "За столом нет свободных мест");
    }

    /**
     * Встать из-за стола.
     *
     * <p>⭐ Строка удаляется, а не помечается: место обязано освободиться для других, а его
     * занятость определяет уникальный индекс {@code (table_id, seat_no)} — пометка в строке
     * его не отпускает. Факт участия в матче хранит {@code match_players}, а не лобби.
     */
    @Transactional
    public void leave(final UUID tableId, final UUID userId) {
        players.findByTableIdAndUserId(tableId, userId).ifPresent(players::delete);
    }

    @Transactional
    public TablePlayer setReady(final UUID tableId, final UUID userId, final boolean ready) {
        final TablePlayer seat = players.findByTableIdAndUserId(tableId, userId)
                .orElseThrow(() -> new ApiException(HttpStatus.CONFLICT, "NOT_AT_TABLE",
                        "Ты не за этим столом"));
        seat.setReady(ready);
        return players.save(seat);
    }

    /** Стол переходит в матч: новые игроки за него уже не сядут. */
    @Transactional
    public void startMatch(final UUID tableId) {
        final GameTable table = tables.findById(tableId).orElseThrow(LobbyService::tableNotFound);
        table.startMatch();
        tables.save(table);
    }

    @Transactional
    public void close(final UUID tableId, final UUID userId) {
        final GameTable table = tables.findById(tableId).orElseThrow(LobbyService::tableNotFound);
        if (!table.isHost(userId)) {
            throw new ApiException(HttpStatus.FORBIDDEN, "NOT_TABLE_HOST", "Стол закрывает только хозяин");
        }
        if (table.status() == TableStatus.IN_MATCH) {
            throw new ApiException(HttpStatus.CONFLICT, "MATCH_IN_PROGRESS",
                    "Нельзя закрыть стол посреди матча");
        }
        table.close(clock.instant());
        tables.save(table);
    }

    /** Все ли за столом готовы и хватает ли игроков для старта. */
    @Transactional(readOnly = true)
    public boolean isReadyToStart(final UUID tableId) {
        final List<TablePlayer> seated = seats(tableId);
        return seated.size() >= 2 && seated.stream().allMatch(TablePlayer::isReady);
    }

    private String newCode() {
        for (int attempt = 0; attempt < SEAT_ATTEMPTS; attempt++) {
            final StringBuilder code = new StringBuilder(CODE_LENGTH);
            for (int index = 0; index < CODE_LENGTH; index++) {
                code.append(CODE_ALPHABET.charAt(random.nextInt(CODE_ALPHABET.length())));
            }
            if (!tables.existsByCode(code.toString())) {
                return code.toString();
            }
        }
        throw new IllegalStateException("Не удалось подобрать свободный код стола");
    }

    private static ApiException tableNotFound() {
        return new ApiException(HttpStatus.NOT_FOUND, "TABLE_NOT_FOUND", "Стол не найден");
    }
}
