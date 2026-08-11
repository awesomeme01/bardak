package kz.bardak.lobby.domain;

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
 * Игрок за столом. Ключ составной — стол плюс игрок: одному человеку за одним столом
 * два места ни к чему.
 */
@Entity
@Table(name = "table_players")
@IdClass(TablePlayer.Key.class)
public class TablePlayer {

    @Id
    @Column(name = "table_id", nullable = false)
    private UUID tableId;

    @Id
    @Column(name = "user_id", nullable = false)
    private UUID userId;

    @Column(name = "seat_no", nullable = false)
    private short seatNo;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false)
    private SeatState state;

    @Column(name = "joined_at", nullable = false, updatable = false, insertable = false)
    private Instant joinedAt;

    protected TablePlayer() {
        // для JPA
    }

    public TablePlayer(final UUID tableId, final UUID userId, final int seatNo) {
        this.tableId = Objects.requireNonNull(tableId, "tableId");
        this.userId = Objects.requireNonNull(userId, "userId");
        this.seatNo = (short) seatNo;
        this.state = SeatState.JOINED;
    }

    public UUID tableId() {
        return tableId;
    }

    public UUID userId() {
        return userId;
    }

    public int seatNo() {
        return seatNo;
    }

    public SeatState state() {
        return state;
    }

    public boolean isAtTable() {
        return state != SeatState.LEFT;
    }

    public boolean isReady() {
        return state == SeatState.READY;
    }

    public void setReady(final boolean ready) {
        this.state = ready ? SeatState.READY : SeatState.JOINED;
    }

    public void leave() {
        this.state = SeatState.LEFT;
    }

    /** Вернулся за тот же стол — занимает своё прежнее место. */
    public void rejoin() {
        this.state = SeatState.JOINED;
    }

    /** Составной ключ. Нужен JPA, в коде не используется. */
    public record Key(UUID tableId, UUID userId) implements Serializable {

        public Key() {
            this(null, null);
        }
    }
}
