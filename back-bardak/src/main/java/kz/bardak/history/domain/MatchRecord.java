package kz.bardak.history.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.time.Instant;
import java.util.Objects;
import java.util.UUID;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

/**
 * Матч в базе. Живое состояние матча лежит в памяти стола, здесь — то, что переживает
 * рестарт: seed, снимок правил и итог.
 */
@Entity
@Table(name = "matches")
public class MatchRecord {

    @Id
    private UUID id;

    @Column(name = "table_id", nullable = false)
    private UUID tableId;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false)
    private MatchRecordStatus status;

    @Column(name = "players_count", nullable = false)
    private short playersCount;

    @Column(name = "deals_played", nullable = false)
    private short dealsPlayed;

    /** ⭐ Не отдаётся клиенту, пока матч идёт: по нему вычисляется вся колода (§6). */
    @Column(name = "rng_seed", nullable = false)
    private long rngSeed;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "rules_snapshot", nullable = false)
    private String rulesSnapshot;

    @Column(name = "started_at", nullable = false, updatable = false, insertable = false)
    private Instant startedAt;

    @Column(name = "finished_at")
    private Instant finishedAt;

    @Column(name = "loser_user_id")
    private UUID loserUserId;

    @Column(name = "abort_reason")
    private String abortReason;

    protected MatchRecord() {
        // для JPA
    }

    public MatchRecord(final UUID id, final UUID tableId, final int playersCount, final long rngSeed,
                       final String rulesSnapshot) {
        this.id = Objects.requireNonNull(id, "id");
        this.tableId = Objects.requireNonNull(tableId, "tableId");
        this.status = MatchRecordStatus.IN_PROGRESS;
        this.playersCount = (short) playersCount;
        this.dealsPlayed = 0;
        this.rngSeed = rngSeed;
        this.rulesSnapshot = Objects.requireNonNull(rulesSnapshot, "rulesSnapshot");
    }

    public UUID id() {
        return id;
    }

    public UUID tableId() {
        return tableId;
    }

    public MatchRecordStatus status() {
        return status;
    }

    public int dealsPlayed() {
        return dealsPlayed;
    }

    public long rngSeed() {
        return rngSeed;
    }

    public int playersCount() {
        return playersCount;
    }

    public String rulesSnapshot() {
        return rulesSnapshot;
    }

    public Instant startedAt() {
        return startedAt;
    }

    public Instant finishedAt() {
        return finishedAt;
    }

    public UUID loserUserId() {
        return loserUserId;
    }

    public String abortReason() {
        return abortReason;
    }

    public void dealsPlayed(final int value) {
        this.dealsPlayed = (short) value;
    }

    public void finish(final UUID loserUserId, final Instant now) {
        this.status = MatchRecordStatus.FINISHED;
        this.loserUserId = loserUserId;
        this.finishedAt = Objects.requireNonNull(now, "now");
    }

    /** ⭐ Отменённый матч не влияет на рейтинг, но сохраняется целиком и виден в истории. */
    public void abort(final String reason, final Instant now) {
        this.status = MatchRecordStatus.ABORTED;
        this.abortReason = reason;
        this.finishedAt = Objects.requireNonNull(now, "now");
    }
}
