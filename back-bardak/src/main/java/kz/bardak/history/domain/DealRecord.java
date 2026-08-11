package kz.bardak.history.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.time.Instant;
import java.util.Objects;
import java.util.UUID;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

/**
 * Сыгранная раздача.
 *
 * <p>Строка появляется, когда раздача уже сочтена: до этого её состояние живёт в памяти
 * стола и в снимках, а история интересуется только итогом.
 */
@Entity
@Table(name = "deals")
public class DealRecord {

    @Id
    private UUID id;

    @Column(name = "match_id", nullable = false)
    private UUID matchId;

    @Column(name = "deal_no", nullable = false)
    private short dealNo;

    @Column(name = "trump_suit")
    private String trumpSuit;

    @Column(name = "started_at", nullable = false, updatable = false, insertable = false)
    private Instant startedAt;

    @Column(name = "finished_at")
    private Instant finishedAt;

    @Column(name = "loser_seat")
    private Short loserSeat;

    /** ⭐ Что было выложено в последней атаке, а не что попало в руку: от этого зависят степени. */
    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "last_attack_cards", nullable = false)
    private String lastAttackCards;

    protected DealRecord() {
        // для JPA
    }

    public DealRecord(final UUID id, final UUID matchId, final int dealNo, final String trumpSuit,
                      final int loserSeat, final String lastAttackCards, final Instant finishedAt) {
        this.id = Objects.requireNonNull(id, "id");
        this.matchId = Objects.requireNonNull(matchId, "matchId");
        this.dealNo = (short) dealNo;
        this.trumpSuit = trumpSuit;
        this.loserSeat = (short) loserSeat;
        this.lastAttackCards = Objects.requireNonNull(lastAttackCards, "lastAttackCards");
        this.finishedAt = Objects.requireNonNull(finishedAt, "finishedAt");
    }

    public UUID id() {
        return id;
    }

    public int dealNo() {
        return dealNo;
    }

    public String trumpSuit() {
        return trumpSuit;
    }

    public int loserSeat() {
        return loserSeat;
    }

    public String lastAttackCards() {
        return lastAttackCards;
    }

    public Instant finishedAt() {
        return finishedAt;
    }
}
