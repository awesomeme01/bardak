package kz.bardak.auth.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Выданный refresh-токен.
 *
 * <p>⭐ Хранится <b>только хеш</b>: утечка дампа БД не должна давать возможность войти
 * за пользователя — то же соображение, что и с паролями. Сам токен видит только клиент.
 */
@Entity
@Table(name = "refresh_tokens")
public class RefreshToken {

    @Id
    private UUID id;

    @Column(name = "user_id", nullable = false)
    private UUID userId;

    @Column(name = "token_hash", nullable = false, unique = true)
    private String tokenHash;

    @Column(name = "expires_at", nullable = false)
    private Instant expiresAt;

    @Column(name = "revoked_at")
    private Instant revokedAt;

    @Column(name = "user_agent")
    private String userAgent;

    @Column(name = "created_at", nullable = false, updatable = false, insertable = false)
    private Instant createdAt;

    protected RefreshToken() {
        // для JPA
    }

    public RefreshToken(final UUID id, final UUID userId, final String tokenHash,
                        final Instant expiresAt, final String userAgent) {
        this.id = Objects.requireNonNull(id, "id");
        this.userId = Objects.requireNonNull(userId, "userId");
        this.tokenHash = Objects.requireNonNull(tokenHash, "tokenHash");
        this.expiresAt = Objects.requireNonNull(expiresAt, "expiresAt");
        this.userAgent = userAgent;
    }

    public UUID id() {
        return id;
    }

    public UUID userId() {
        return userId;
    }

    public Instant expiresAt() {
        return expiresAt;
    }

    public boolean isRevoked() {
        return revokedAt != null;
    }

    public boolean isExpiredAt(final Instant now) {
        return !expiresAt.isAfter(now);
    }

    /** Токен ещё пригоден к обмену. */
    public boolean isUsableAt(final Instant now) {
        return !isRevoked() && !isExpiredAt(now);
    }

    public void revoke(final Instant now) {
        if (revokedAt == null) {
            revokedAt = Objects.requireNonNull(now, "now");
        }
    }
}
