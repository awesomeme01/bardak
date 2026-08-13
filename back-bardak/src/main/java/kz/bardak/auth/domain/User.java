package kz.bardak.auth.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Игрок. Схема — `V1__users_and_auth.sql`.
 *
 * <p>⚠️ Наружу эта сущность не отдаётся никогда: в ней лежит {@code passwordHash}.
 * Для ответов есть отдельные DTO в {@code kz.bardak.auth.api}.
 */
@Entity
@Table(name = "users")
public class User {

    @Id
    private UUID id;

    @Column(nullable = false, unique = true)
    private String username;

    @Column(name = "display_name", nullable = false)
    private String displayName;

    private String email;

    @Column(name = "password_hash", nullable = false)
    private String passwordHash;

    @Column(name = "avatar_url")
    private String avatarUrl;

    /** ⭐ Эмодзи, а не ссылка: за столом мордочка, а не фотография (см. V8). */
    @Column
    private String avatar;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false)
    private UserStatus status;

    @Column(name = "created_at", nullable = false, updatable = false, insertable = false)
    private Instant createdAt;

    @Column(name = "updated_at", nullable = false, insertable = false)
    private Instant updatedAt;

    protected User() {
        // для JPA
    }

    public User(final UUID id, final String username, final String displayName,
                final String email, final String passwordHash) {
        this.id = Objects.requireNonNull(id, "id");
        this.username = Objects.requireNonNull(username, "username");
        this.displayName = Objects.requireNonNull(displayName, "displayName");
        this.email = email;
        this.passwordHash = Objects.requireNonNull(passwordHash, "passwordHash");
        this.status = UserStatus.ACTIVE;
    }

    public UUID id() {
        return id;
    }

    public String username() {
        return username;
    }

    public String displayName() {
        return displayName;
    }

    public String email() {
        return email;
    }

    public String passwordHash() {
        return passwordHash;
    }

    public String avatar() {
        return avatar;
    }

    /** Сменить то, что видно за столом. Логин не меняется — по нему входят. */
    public void rename(final String displayName, final String avatar) {
        this.displayName = Objects.requireNonNull(displayName, "displayName");
        this.avatar = avatar;
    }

    public void changePassword(final String passwordHash) {
        this.passwordHash = Objects.requireNonNull(passwordHash, "passwordHash");
    }

    public String avatarUrl() {
        return avatarUrl;
    }

    public UserStatus status() {
        return status;
    }

    public boolean isActive() {
        return status == UserStatus.ACTIVE;
    }

    public Instant createdAt() {
        return createdAt;
    }

    public Instant updatedAt() {
        return updatedAt;
    }
}
