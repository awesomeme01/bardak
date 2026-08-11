package kz.bardak.history.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.GeneratedValue;
import jakarta.persistence.GenerationType;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.util.Objects;
import java.util.UUID;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

/**
 * Событие матча. Только вставка: записанное событие неизменяемо (ADR-004).
 *
 * <p>⚠️ {@code payload} содержит <b>полную</b> информацию, включая скрытую — это внутренний
 * лог. Наружу он никогда не отдаётся сырым, только через проекцию.
 */
@Entity
@Table(name = "match_events")
public class MatchEventRecord {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "match_id", nullable = false)
    private UUID matchId;

    /** Сквозной номер по матчу, а не по раздаче: клиент отслеживает один счётчик. */
    @Column(nullable = false)
    private int seq;

    @Column(name = "deal_no")
    private Short dealNo;

    @Column(nullable = false)
    private String type;

    @Column(name = "actor_seat")
    private Short actorSeat;

    /**
     * ⭐ Кому видно событие; {@code null} — всем. Записывается вместе с событием, а не
     * вычисляется заново при догоне: иначе правило видимости жило бы в двух местах.
     */
    @Column(name = "private_to_seat")
    private Short privateToSeat;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(nullable = false)
    private String payload;

    protected MatchEventRecord() {
        // для JPA
    }

    public MatchEventRecord(final UUID matchId, final int seq, final Integer dealNo, final String type,
                            final Integer actorSeat, final String payload, final Integer privateToSeat) {
        this.matchId = Objects.requireNonNull(matchId, "matchId");
        this.seq = seq;
        this.dealNo = dealNo == null ? null : dealNo.shortValue();
        this.type = Objects.requireNonNull(type, "type");
        this.actorSeat = actorSeat == null ? null : actorSeat.shortValue();
        this.payload = Objects.requireNonNull(payload, "payload");
        this.privateToSeat = privateToSeat == null ? null : privateToSeat.shortValue();
    }

    public int seq() {
        return seq;
    }

    public String type() {
        return type;
    }

    public String payload() {
        return payload;
    }

    public Integer privateToSeat() {
        return privateToSeat == null ? null : privateToSeat.intValue();
    }

    /** Видит ли это событие игрок на указанном месте. */
    public boolean isVisibleTo(final int seatNo) {
        return privateToSeat == null || privateToSeat == seatNo;
    }

    public Integer actorSeat() {
        return actorSeat == null ? null : actorSeat.intValue();
    }

    public Integer dealNo() {
        return dealNo == null ? null : dealNo.intValue();
    }
}
