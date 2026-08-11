package kz.bardak.history.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.IdClass;
import jakarta.persistence.Table;
import java.io.Serializable;
import java.math.BigDecimal;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.game.rules.LossDegree;

/**
 * Участник матча: место за столом и итог.
 *
 * <p>Строка появляется при старте матча — пустая, только с местом. Итог (место в матче,
 * уровень навесов, степень проигрыша, рейтинг) дописывается при завершении. У отменённого
 * матча итог так и остаётся пустым: рейтинга он не касается.
 */
@Entity
@Table(name = "match_players")
@IdClass(MatchPlayerRecord.Key.class)
public class MatchPlayerRecord {

    @Id
    @Column(name = "match_id")
    private UUID matchId;

    @Id
    @Column(name = "user_id")
    private UUID userId;

    @Column(name = "seat_no", nullable = false)
    private short seatNo;

    /** ⭐ Счёта в очках нет: роль счёта играет уровень навеса (ADR-017). null — «летит 6». */
    @Column(name = "naves_level")
    private String navesLevel;

    @Enumerated(EnumType.STRING)
    @Column(name = "loss_type")
    private LossDegree lossType;

    @Column
    private Short place;

    @Column(name = "rating_before")
    private BigDecimal ratingBefore;

    @Column(name = "rating_after")
    private BigDecimal ratingAfter;

    @Column(name = "rating_delta")
    private BigDecimal ratingDelta;

    protected MatchPlayerRecord() {
        // для JPA
    }

    public MatchPlayerRecord(final UUID matchId, final UUID userId, final int seatNo) {
        this.matchId = Objects.requireNonNull(matchId, "matchId");
        this.userId = Objects.requireNonNull(userId, "userId");
        this.seatNo = (short) seatNo;
    }

    public UUID matchId() {
        return matchId;
    }

    public UUID userId() {
        return userId;
    }

    public int seatNo() {
        return seatNo;
    }

    public Integer place() {
        return place == null ? null : (int) place;
    }

    public String navesLevel() {
        return navesLevel;
    }

    public LossDegree lossType() {
        return lossType;
    }

    public BigDecimal ratingBefore() {
        return ratingBefore;
    }

    public BigDecimal ratingAfter() {
        return ratingAfter;
    }

    public BigDecimal ratingDelta() {
        return ratingDelta;
    }

    public void finish(final int place, final String navesLevel, final LossDegree lossType,
                       final BigDecimal ratingBefore, final BigDecimal ratingAfter) {
        this.place = (short) place;
        this.navesLevel = navesLevel;
        this.lossType = lossType;
        this.ratingBefore = ratingBefore;
        this.ratingAfter = ratingAfter;
        this.ratingDelta = ratingAfter.subtract(ratingBefore);
    }

    /** Составной ключ (match_id, user_id): один игрок садится за матч один раз. */
    public static final class Key implements Serializable {

        private UUID matchId;
        private UUID userId;

        public Key() {
            // для JPA
        }

        public Key(final UUID matchId, final UUID userId) {
            this.matchId = matchId;
            this.userId = userId;
        }

        @Override
        public boolean equals(final Object other) {
            return other instanceof Key key
                    && Objects.equals(matchId, key.matchId)
                    && Objects.equals(userId, key.userId);
        }

        @Override
        public int hashCode() {
            return Objects.hash(matchId, userId);
        }
    }
}
