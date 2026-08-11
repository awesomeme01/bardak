package kz.bardak.history.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.IdClass;
import jakarta.persistence.Table;
import java.io.Serializable;
import java.time.Instant;
import java.util.Objects;
import java.util.UUID;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

/**
 * Снимок состояния матча после события с номером {@code seq}.
 *
 * <p>Из него матч поднимается после рестарта, не проигрывая лог с начала. Внутри —
 * полное состояние, включая скрытые карты: наружу снимок не отдаётся никогда.
 */
@Entity
@Table(name = "match_snapshots")
@IdClass(MatchSnapshotRecord.Key.class)
public class MatchSnapshotRecord {

    @Id
    @Column(name = "match_id", nullable = false)
    private UUID matchId;

    @Id
    @Column(nullable = false)
    private int seq;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(nullable = false)
    private String state;

    @Column(name = "created_at", nullable = false, updatable = false, insertable = false)
    private Instant createdAt;

    protected MatchSnapshotRecord() {
        // для JPA
    }

    public MatchSnapshotRecord(final UUID matchId, final int seq, final String state) {
        this.matchId = Objects.requireNonNull(matchId, "matchId");
        this.seq = seq;
        this.state = Objects.requireNonNull(state, "state");
    }

    public int seq() {
        return seq;
    }

    public String state() {
        return state;
    }

    public record Key(UUID matchId, int seq) implements Serializable {

        public Key() {
            this(null, 0);
        }
    }
}
