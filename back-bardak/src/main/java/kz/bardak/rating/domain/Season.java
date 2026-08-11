package kz.bardak.rating.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/** Сезон рейтинга. Закрывается вручную, а не по календарю (ADR-037). */
@Entity
@Table(name = "seasons")
public class Season {

    @Id
    private UUID id;

    @Column(nullable = false)
    private String name;

    /** Дата начала проставляется кодом: её показывают сразу после открытия сезона. */
    @Column(name = "started_at", nullable = false, updatable = false)
    private Instant startedAt;

    @Column(name = "closed_at")
    private Instant closedAt;

    protected Season() {
        // для JPA
    }

    /** Новый открытый сезон. */
    public static Season open(final UUID id, final String name, final Instant now) {
        final Season season = new Season();
        season.id = Objects.requireNonNull(id, "id");
        season.name = Objects.requireNonNull(name, "name");
        season.startedAt = Objects.requireNonNull(now, "now");
        return season;
    }

    public UUID id() {
        return id;
    }

    public String name() {
        return name;
    }

    public Instant startedAt() {
        return startedAt;
    }

    public Instant closedAt() {
        return closedAt;
    }

    public boolean isOpen() {
        return closedAt == null;
    }

    public void close(final Instant now) {
        this.closedAt = Objects.requireNonNull(now, "now");
    }
}
