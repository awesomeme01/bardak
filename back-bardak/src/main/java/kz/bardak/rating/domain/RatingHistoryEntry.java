package kz.bardak.rating.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.GeneratedValue;
import jakarta.persistence.GenerationType;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.math.BigDecimal;
import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Одна строка истории рейтинга.
 *
 * <p>⭐ Хранится подробно ради двух вещей: график в профиле и возможность <b>пересчитать
 * всё с нуля</b>, если формула изменится. Переход Elo → Glicko-2 выполняется прогоном
 * этой истории в хронологическом порядке — без неё он означал бы потерю прошлого.
 */
@Entity
@Table(name = "rating_history")
public class RatingHistoryEntry {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "user_id", nullable = false)
    private UUID userId;

    @Column(name = "match_id", nullable = false)
    private UUID matchId;

    @Column(name = "rating_before", nullable = false)
    private BigDecimal ratingBefore;

    @Column(name = "rating_after", nullable = false)
    private BigDecimal ratingAfter;

    @Column(name = "deviation_after", nullable = false)
    private BigDecimal deviationAfter;

    @Column(nullable = false)
    private short place;

    @Column(name = "players_count", nullable = false)
    private short playersCount;

    @Column(name = "season_id")
    private UUID seasonId;

    @Column(name = "created_at", nullable = false, updatable = false, insertable = false)
    private Instant createdAt;

    protected RatingHistoryEntry() {
        // для JPA
    }

    public RatingHistoryEntry(final UUID userId, final UUID matchId, final BigDecimal ratingBefore,
                              final BigDecimal ratingAfter, final BigDecimal deviationAfter,
                              final int place, final int playersCount, final UUID seasonId) {
        this.userId = Objects.requireNonNull(userId, "userId");
        this.matchId = Objects.requireNonNull(matchId, "matchId");
        this.ratingBefore = Objects.requireNonNull(ratingBefore, "ratingBefore");
        this.ratingAfter = Objects.requireNonNull(ratingAfter, "ratingAfter");
        this.deviationAfter = Objects.requireNonNull(deviationAfter, "deviationAfter");
        this.place = (short) place;
        this.playersCount = (short) playersCount;
        this.seasonId = seasonId;
    }

    public UUID userId() {
        return userId;
    }

    public BigDecimal ratingBefore() {
        return ratingBefore;
    }

    public BigDecimal ratingAfter() {
        return ratingAfter;
    }

    public UUID matchId() {
        return matchId;
    }

    public int playersCount() {
        return playersCount;
    }

    public int place() {
        return place;
    }

    public Instant createdAt() {
        return createdAt;
    }
}
