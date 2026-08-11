package kz.bardak.lobby.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import jakarta.persistence.Version;
import java.time.Instant;
import java.util.Objects;
import java.util.UUID;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

/**
 * Стол. Живёт в БД, а не в памяти: лобби не держит состояние (`02-architecture.md`).
 *
 * <p>{@code version} — оптимистичная блокировка под гонку «двое одновременно занимают
 * последнее место». Второй уровень защиты от той же гонки — уникальный индекс
 * {@code (table_id, seat_no)} в {@link TablePlayer}.
 */
@Entity
@Table(name = "game_tables")
public class GameTable {

    @Id
    private UUID id;

    @Column(nullable = false, unique = true)
    private String code;

    @Column(nullable = false)
    private String name;

    @Column(name = "host_user_id", nullable = false)
    private UUID hostUserId;

    @Column(name = "max_players", nullable = false)
    private short maxPlayers;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false)
    private TableStatus status;

    @Column(name = "card_set_id", nullable = false)
    private UUID cardSetId;

    @Column(name = "theme_id", nullable = false)
    private UUID themeId;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "rules_config", nullable = false)
    private String rulesConfig;

    @Column(name = "is_private", nullable = false)
    private boolean isPrivate;

    @Version
    @Column(nullable = false)
    private int version;

    @Column(name = "created_at", nullable = false, updatable = false, insertable = false)
    private Instant createdAt;

    @Column(name = "closed_at")
    private Instant closedAt;

    protected GameTable() {
        // для JPA
    }

    public GameTable(final UUID id, final String code, final String name, final UUID hostUserId,
                     final int maxPlayers, final UUID cardSetId, final UUID themeId,
                     final String rulesConfig, final boolean isPrivate) {
        this.id = Objects.requireNonNull(id, "id");
        this.code = Objects.requireNonNull(code, "code");
        this.name = Objects.requireNonNull(name, "name");
        this.hostUserId = Objects.requireNonNull(hostUserId, "hostUserId");
        this.maxPlayers = (short) maxPlayers;
        this.status = TableStatus.WAITING;
        this.cardSetId = Objects.requireNonNull(cardSetId, "cardSetId");
        this.themeId = Objects.requireNonNull(themeId, "themeId");
        this.rulesConfig = Objects.requireNonNull(rulesConfig, "rulesConfig");
        this.isPrivate = isPrivate;
    }

    public UUID id() {
        return id;
    }

    public String code() {
        return code;
    }

    public String name() {
        return name;
    }

    public UUID hostUserId() {
        return hostUserId;
    }

    public int maxPlayers() {
        return maxPlayers;
    }

    public TableStatus status() {
        return status;
    }

    public UUID cardSetId() {
        return cardSetId;
    }

    public UUID themeId() {
        return themeId;
    }

    public String rulesConfig() {
        return rulesConfig;
    }

    public boolean isPrivate() {
        return isPrivate;
    }

    public int version() {
        return version;
    }

    public Instant createdAt() {
        return createdAt;
    }

    public boolean isOpenForJoin() {
        return status == TableStatus.WAITING;
    }

    public boolean isHost(final UUID userId) {
        return hostUserId.equals(userId);
    }

    public void close(final Instant now) {
        this.status = TableStatus.CLOSED;
        this.closedAt = Objects.requireNonNull(now, "now");
    }

    public void startMatch() {
        this.status = TableStatus.IN_MATCH;
    }

    /** Матч кончился — стол снова открыт: можно доукомплектоваться и сыграть ещё. */
    public void finishMatch() {
        this.status = TableStatus.WAITING;
    }
}
