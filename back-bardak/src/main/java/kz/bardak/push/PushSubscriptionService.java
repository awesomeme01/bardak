package kz.bardak.push;

import java.util.List;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.push.domain.PushSubscription;
import kz.bardak.push.domain.PushSubscriptionRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

/**
 * Подписки устройств на уведомления. Отправкой занимается {@link PushSender}.
 *
 * <p>Название длиннее очевидного {@code PushService} намеренно: так зовётся класс
 * библиотеки отправки, и два разных {@code PushService} в одном пакете читались бы
 * как один и тот же.
 */
@Service
public class PushSubscriptionService {

    private final PushSubscriptionRepository subscriptions;
    private final PushProperties properties;

    public PushSubscriptionService(final PushSubscriptionRepository subscriptions,
                                   final PushProperties properties) {
        this.subscriptions = Objects.requireNonNull(subscriptions, "subscriptions");
        this.properties = Objects.requireNonNull(properties, "properties");
    }

    /** Открытый ключ для браузера. Пусто — уведомления выключены, подписываться не на что. */
    public String publicKey() {
        return properties.isEnabled() ? properties.publicKey() : null;
    }

    /**
     * Запомнить подписку.
     *
     * <p>⭐ Ключ — endpoint, а не пара «пользователь + устройство»: браузер выдаёт его сам
     * и сам же меняет при обновлении подписки. Один endpoint — одна строка, поэтому
     * повторная подписка обновляет существующую, а не плодит дубли, из-за которых игрок
     * получал бы один и тот же звонок дважды.
     *
     * <p>Устройство могло перейти к другому игроку — тогда у строки просто меняется
     * владелец: звонить на этот телефон прежнему нельзя.
     */
    @Transactional
    public PushSubscription subscribe(final UUID userId, final String endpoint, final String p256dh,
                                      final String auth, final String userAgent) {
        final PushSubscription existing = subscriptions.findByEndpoint(endpoint).orElse(null);
        if (existing == null) {
            return subscriptions.save(new PushSubscription(UUID.randomUUID(), userId, endpoint,
                    p256dh, auth, userAgent));
        }
        existing.reassign(userId, p256dh, auth, userAgent);
        return subscriptions.save(existing);
    }

    /**
     * Отписать устройство.
     *
     * <p>⚠️ Владелец обязателен. Без него отписка была доступна любому вошедшему, кто знал
     * чужой {@code endpoint}: устройство переставало получать уведомления о своём ходе,
     * а его хозяин не мог понять, почему.
     *
     * <p>Отсутствие подписки ошибкой не считается: отписка идемпотентна, повторный вызов
     * с того же устройства — обычное дело.
     */
    @Transactional
    public void unsubscribe(final String endpoint, final UUID userId) {
        subscriptions.deleteByEndpointAndUserId(endpoint, userId);
    }

    @Transactional(readOnly = true)
    public List<PushSubscription> of(final UUID userId) {
        return subscriptions.findByUserId(userId);
    }
}
