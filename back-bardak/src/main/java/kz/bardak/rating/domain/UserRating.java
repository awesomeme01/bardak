package kz.bardak.rating.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.math.BigDecimal;
import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/** Текущий рейтинг игрока. Считается по матчу, а не по раздаче (`07-rating-system.md`). */
@Entity
@Table(name = "user_rating")
public class UserRating {

    /** Стартовый рейтинг новичка. */
    public static final BigDecimal INITIAL = BigDecimal.valueOf(1000);

    @Id
    @Column(name = "user_id")
    private UUID userId;

    @Column(nullable = false)
    private BigDecimal rating;

    /** Не используется MVP: заведено под Glicko-2 / OpenSkill, чтобы переезд не стоил миграции. */
    @Column(nullable = false)
    private BigDecimal deviation;

    @Column(nullable = false)
    private BigDecimal volatility;

    @Column(name = "matches_played", nullable = false)
    private int matchesPlayed;

    @Column(name = "updated_at", nullable = false)
    private Instant updatedAt;

    protected UserRating() {
        // для JPA
    }

    public UserRating(final UUID userId, final Instant now) {
        this.userId = Objects.requireNonNull(userId, "userId");
        this.rating = INITIAL;
        this.deviation = BigDecimal.valueOf(350);
        this.volatility = BigDecimal.valueOf(0.06);
        this.matchesPlayed = 0;
        this.updatedAt = Objects.requireNonNull(now, "now");
    }

    public UUID userId() {
        return userId;
    }

    public BigDecimal rating() {
        return rating;
    }

    public BigDecimal deviation() {
        return deviation;
    }

    public int matchesPlayed() {
        return matchesPlayed;
    }

    public void apply(final BigDecimal newRating, final Instant now) {
        this.rating = Objects.requireNonNull(newRating, "newRating");
        this.matchesPlayed = matchesPlayed + 1;
        this.updatedAt = Objects.requireNonNull(now, "now");
    }
}
