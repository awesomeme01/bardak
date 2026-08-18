package kz.bardak.social.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.IdClass;
import jakarta.persistence.Table;
import java.io.Serializable;
import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Дружба или заявка на неё — одна строка на пару.
 *
 * <p>⭐ Пара нормализована: в {@code lowUserId} всегда меньший из двух идентификаторов.
 * Благодаря этому «А дружит с Б» и «Б дружит с А» — физически одна и та же строка, и
 * рассинхрон, при котором человек у тебя в друзьях, а ты у него нет, невозможен.
 *
 * <p>Кто кого позвал, хранится отдельно в {@link #requestedBy}: порядок в ключе про
 * сортировку, а не про отношения.
 */
@Entity
@Table(name = "friendships")
@IdClass(Friendship.Key.class)
public class Friendship {

    @Id
    @Column(name = "low_user_id", nullable = false)
    private UUID lowUserId;

    @Id
    @Column(name = "high_user_id", nullable = false)
    private UUID highUserId;

    @Column(name = "requested_by", nullable = false)
    private UUID requestedBy;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false)
    private FriendshipStatus status;

    @Column(name = "created_at", nullable = false, updatable = false, insertable = false)
    private Instant createdAt;

    @Column(name = "decided_at")
    private Instant decidedAt;

    protected Friendship() {
        // для JPA
    }

    private Friendship(final UUID lowUserId, final UUID highUserId, final UUID requestedBy) {
        this.lowUserId = lowUserId;
        this.highUserId = highUserId;
        this.requestedBy = requestedBy;
        this.status = FriendshipStatus.PENDING;
    }

    /**
     * Порядок пары — тот же, что у Postgres.
     *
     * <p>⚠️ Здесь нельзя брать {@link UUID#compareTo}: он сравнивает UUID как два
     * <b>знаковых</b> long, а Postgres — побайтово. На идентификаторах со старшим единичным
     * битом эти порядки противоположны, и строка, «правильная» по мнению Java, падала
     * на проверке {@code low_user_id < high_user_id}.
     *
     * <p>Сравнение по канонической записи совпадает с побайтовым: шестнадцатеричные цифры
     * упорядочены так же, как байты, которые они изображают.
     */
    public static int comparePairOrder(final UUID one, final UUID two) {
        return one.toString().compareTo(two.toString());
    }

    /** Заявка от одного к другому. Порядок в ключе выравнивается здесь, а не у вызывающего. */
    public static Friendship requested(final UUID from, final UUID to) {
        Objects.requireNonNull(from, "from");
        Objects.requireNonNull(to, "to");
        if (from.equals(to)) {
            throw new IllegalArgumentException("Нельзя подружиться с самим собой");
        }
        final boolean fromIsLower = comparePairOrder(from, to) < 0;
        return new Friendship(fromIsLower ? from : to, fromIsLower ? to : from, from);
    }

    public void accept(final Instant now) {
        this.status = FriendshipStatus.ACCEPTED;
        this.decidedAt = Objects.requireNonNull(now, "now");
    }

    /** Второй участник пары. Спрашивают всегда «а кто там со мной». */
    public UUID otherThan(final UUID userId) {
        return lowUserId.equals(userId) ? highUserId : lowUserId;
    }

    public boolean involves(final UUID userId) {
        return lowUserId.equals(userId) || highUserId.equals(userId);
    }

    /** Заявку принимает тот, кто её не отправлял. */
    public boolean canBeAcceptedBy(final UUID userId) {
        return status == FriendshipStatus.PENDING && involves(userId) && !requestedBy.equals(userId);
    }

    public boolean isAccepted() {
        return status == FriendshipStatus.ACCEPTED;
    }

    public UUID lowUserId() {
        return lowUserId;
    }

    public UUID highUserId() {
        return highUserId;
    }

    public UUID requestedBy() {
        return requestedBy;
    }

    public FriendshipStatus status() {
        return status;
    }

    public Instant createdAt() {
        return createdAt;
    }

    /** Составной ключ пары. */
    public static final class Key implements Serializable {

        private UUID lowUserId;
        private UUID highUserId;

        public Key() {
        }

        public Key(final UUID lowUserId, final UUID highUserId) {
            this.lowUserId = lowUserId;
            this.highUserId = highUserId;
        }

        @Override
        public boolean equals(final Object other) {
            if (this == other) {
                return true;
            }
            return other instanceof Key key
                    && Objects.equals(lowUserId, key.lowUserId)
                    && Objects.equals(highUserId, key.highUserId);
        }

        @Override
        public int hashCode() {
            return Objects.hash(lowUserId, highUserId);
        }
    }
}
