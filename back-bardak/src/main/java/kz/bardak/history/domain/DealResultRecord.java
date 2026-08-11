package kz.bardak.history.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.IdClass;
import jakarta.persistence.Table;
import java.io.Serializable;
import java.util.Objects;
import java.util.UUID;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

/** Итог раздачи для одного места: место, навешенное и из чего сложился сдвиг уровня. */
@Entity
@Table(name = "deal_results")
@IdClass(DealResultRecord.Key.class)
public class DealResultRecord {

    @Id
    @Column(name = "deal_id")
    private UUID dealId;

    @Id
    @Column(name = "seat_no")
    private short seatNo;

    @Column
    private Short place;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "hung_cards", nullable = false)
    private String hungCards;

    @Column(name = "naves_level_before")
    private String navesLevelBefore;

    @Column(name = "naves_level_after")
    private String navesLevelAfter;

    /** ⭐ Почему уровень поменялся: сдвигов четыре вида, и в одной раздаче они сочетаются. */
    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "level_changes", nullable = false)
    private String levelChanges;

    protected DealResultRecord() {
        // для JPA
    }

    public DealResultRecord(final UUID dealId, final int seatNo, final int place,
                            final String hungCards, final String navesLevelBefore,
                            final String navesLevelAfter, final String levelChanges) {
        this.dealId = Objects.requireNonNull(dealId, "dealId");
        this.seatNo = (short) seatNo;
        this.place = (short) place;
        this.hungCards = Objects.requireNonNull(hungCards, "hungCards");
        this.navesLevelBefore = navesLevelBefore;
        this.navesLevelAfter = navesLevelAfter;
        this.levelChanges = Objects.requireNonNull(levelChanges, "levelChanges");
    }

    public UUID dealId() {
        return dealId;
    }

    public int seatNo() {
        return seatNo;
    }

    public Integer place() {
        return place == null ? null : (int) place;
    }

    public String hungCards() {
        return hungCards;
    }

    public String navesLevelBefore() {
        return navesLevelBefore;
    }

    public String navesLevelAfter() {
        return navesLevelAfter;
    }

    public String levelChanges() {
        return levelChanges;
    }

    /** Составной ключ (deal_id, seat_no). */
    public static final class Key implements Serializable {

        private UUID dealId;
        private short seatNo;

        public Key() {
            // для JPA
        }

        public Key(final UUID dealId, final short seatNo) {
            this.dealId = dealId;
            this.seatNo = seatNo;
        }

        @Override
        public boolean equals(final Object other) {
            return other instanceof Key key
                    && Objects.equals(dealId, key.dealId)
                    && seatNo == key.seatNo;
        }

        @Override
        public int hashCode() {
            return Objects.hash(dealId, seatNo);
        }
    }
}
