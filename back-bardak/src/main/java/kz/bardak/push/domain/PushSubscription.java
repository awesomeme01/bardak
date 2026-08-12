package kz.bardak.push.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.time.Instant;
import java.util.Objects;
import java.util.UUID;

/**
 * Подписка устройства на уведомления.
 *
 * <p>⭐ Подписка принадлежит устройству, а не человеку: телефон и ноутбук — две строки,
 * и «твой ход» уходит на оба. Ключи хранятся как есть: ими шифруется полезная нагрузка
 * (RFC 8291), и без них уведомление отправить нельзя.
 */
@Entity
@Table(name = "push_subscriptions")
public class PushSubscription {

    @Id
    private UUID id;

    @Column(name = "user_id", nullable = false)
    private UUID userId;

    @Column(nullable = false)
    private String endpoint;

    @Column(nullable = false)
    private String p256dh;

    @Column(nullable = false)
    private String auth;

    @Column(name = "user_agent")
    private String userAgent;

    @Column(name = "created_at", nullable = false, updatable = false, insertable = false)
    private Instant createdAt;

    @Column(name = "last_sent_at")
    private Instant lastSentAt;

    protected PushSubscription() {
        // для JPA
    }

    public PushSubscription(final UUID id, final UUID userId, final String endpoint,
                            final String p256dh, final String auth, final String userAgent) {
        this.id = Objects.requireNonNull(id, "id");
        this.userId = Objects.requireNonNull(userId, "userId");
        this.endpoint = Objects.requireNonNull(endpoint, "endpoint");
        this.p256dh = Objects.requireNonNull(p256dh, "p256dh");
        this.auth = Objects.requireNonNull(auth, "auth");
        this.userAgent = userAgent;
    }

    public UUID id() {
        return id;
    }

    public UUID userId() {
        return userId;
    }

    public String endpoint() {
        return endpoint;
    }

    public String p256dh() {
        return p256dh;
    }

    public String auth() {
        return auth;
    }

    /**
     * Переприсвоить подписку.
     *
     * <p>⭐ Строка обновляется, а не пересоздаётся. Удалить и вставить в одной транзакции
     * нельзя: {@code endpoint} уникален, а Hibernate откладывает удаление до сброса — вставка
     * успевает первой и падает на ограничении. Да и по смыслу это то же устройство:
     * сменился только владелец и ключи.
     */
    public void reassign(final UUID userId, final String p256dh, final String auth,
                         final String userAgent) {
        this.userId = Objects.requireNonNull(userId, "userId");
        this.p256dh = Objects.requireNonNull(p256dh, "p256dh");
        this.auth = Objects.requireNonNull(auth, "auth");
        this.userAgent = userAgent;
    }

    public void sent(final Instant now) {
        this.lastSentAt = Objects.requireNonNull(now, "now");
    }
}
