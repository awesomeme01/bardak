package kz.bardak.push;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.nio.charset.StandardCharsets;
import java.security.Security;
import java.time.Clock;
import java.util.List;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import kz.bardak.push.domain.PushSubscription;
import kz.bardak.push.domain.PushSubscriptionRepository;
import nl.martijndwars.webpush.Notification;
import nl.martijndwars.webpush.PushService;
import org.bouncycastle.jce.provider.BouncyCastleProvider;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;

/**
 * Отправка push-уведомлений.
 *
 * <p>⭐ Отправка идёт в отдельном потоке и никогда — на потоке стола. Push-сервис браузера
 * это чужой сервер в интернете: он может отвечать секунды, а стол на это время замер бы
 * для всех, кто за ним сидит (ADR-007).
 *
 * <p>⚠️ Просроченную подписку push-сервис отвергает кодами 404/410. Такую строку надо
 * удалять: устройство её больше не примет никогда, а копить мусор и стучаться в него
 * при каждом ходе — верный способ упереться в лимиты сервиса.
 */
@Service
public class PushSender {

    private static final Logger log = LoggerFactory.getLogger(PushSender.class);

    /** Коды, после которых подписка мертва навсегда. */
    private static final List<Integer> GONE = List.of(404, 410);

    private final PushSubscriptionRepository subscriptions;
    private final ObjectMapper objectMapper;
    private final Clock clock;
    private final ExecutorService sender;
    private final PushService pushService;

    public PushSender(final PushSubscriptionRepository subscriptions, final PushProperties properties,
                      final ObjectMapper objectMapper, final Clock clock) {
        this.subscriptions = Objects.requireNonNull(subscriptions, "subscriptions");
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper");
        this.clock = Objects.requireNonNull(clock, "clock");
        this.sender = Executors.newSingleThreadExecutor(runnable -> {
            final Thread thread = new Thread(runnable, "push-sender");
            thread.setDaemon(true);
            return thread;
        });
        this.pushService = createService(properties);
    }

    /**
     * Служба отправки или {@code null}, если ключей нет.
     *
     * <p>Отсутствие ключей — не поломка: локально играют с открытой вкладкой, и заводить
     * VAPID ради этого незачем. А вот кривые ключи — поломка, и о ней надо узнать
     * при старте, а не при первом же ходе.
     */
    private static PushService createService(final PushProperties properties) {
        if (!properties.isEnabled()) {
            log.info("Push-уведомления выключены: ключи VAPID не заданы");
            return null;
        }
        // Криптография VAPID построена на кривой P-256; JDK её в нужном виде не даёт.
        Security.addProvider(new BouncyCastleProvider());
        try {
            return new PushService(properties.publicKey(), properties.privateKey(),
                    properties.subject());
        } catch (final java.security.GeneralSecurityException e) {
            throw new IllegalStateException("Ключи VAPID не приняты", e);
        }
    }

    public boolean isEnabled() {
        return pushService != null;
    }

    /** Уведомить игрока, что его ход. Возвращает управление сразу — отправка асинхронна. */
    public void notifyTurn(final UUID userId, final String tableName, final String tableId) {
        send(userId, turnPayload(tableName, tableId));
    }

    /**
     * Текст уведомления.
     *
     * <p>⭐ Название стола в теле, а не только «твой ход»: у человека может идти несколько
     * партий, и уведомление без стола заставляет открывать приложение, чтобы понять, где
     * именно ждут.
     */
    String turnPayload(final String tableName, final String tableId) {
        final ObjectNode payload = objectMapper.createObjectNode()
                .put("type", "YOUR_TURN")
                .put("title", "Твой ход")
                .put("body", tableName == null || tableName.isBlank()
                        ? "За столом ждут тебя" : "Стол «%s» ждёт".formatted(tableName));
        if (tableId != null) {
            payload.put("tableId", tableId);
        }
        return payload.toString();
    }

    /**
     * Позвать игрока обратно: матч встал на паузу из-за него.
     *
     * <p>⭐ Это и есть главный повод для уведомления. «Ход перешёл к отсутствующему»
     * в игре почти не случается: стоит игроку пропасть, как матч встаёт на паузу (§5.2)
     * и ждёт его — а через отведённое время отменяется совсем. Звонок в эту минуту —
     * единственный способ вернуть человека вовремя.
     */
    public void notifyPaused(final UUID userId, final String tableName, final String tableId,
                             final long secondsLeft) {
        send(userId, pausedPayload(tableName, tableId, secondsLeft));
    }

    String pausedPayload(final String tableName, final String tableId, final long secondsLeft) {
        final ObjectNode payload = objectMapper.createObjectNode()
                .put("type", "MATCH_PAUSED")
                .put("title", "Тебя ждут за столом")
                .put("body", "%s на паузе: вернись за %d с, иначе матч отменят".formatted(
                        tableName == null || tableName.isBlank() ? "Партия" : "Стол «%s»".formatted(tableName),
                        secondsLeft));
        if (tableId != null) {
            payload.put("tableId", tableId);
        }
        return payload.toString();
    }

    public void send(final UUID userId, final String payload) {
        if (!isEnabled()) {
            return;
        }
        sender.execute(() -> deliverAll(userId, payload));
    }

    /**
     * ⚠️ Транзакции здесь намеренно нет.
     *
     * <p>Метод вызывается с собственного потока отправителя, и {@code @Transactional}
     * на нём всё равно не сработал бы — вызов идёт мимо прокси. Каждое обращение
     * к репозиторию само по себе транзакция, и этого достаточно: строки независимы,
     * общего инварианта между ними нет.
     */
    private void deliverAll(final UUID userId, final String payload) {
        for (final PushSubscription subscription : subscriptions.findByUserId(userId)) {
            deliver(subscription, payload);
        }
    }

    private void deliver(final PushSubscription subscription, final String payload) {
        try {
            final int status = pushService.send(new Notification(subscription.endpoint(),
                            subscription.p256dh(), subscription.auth(),
                            payload.getBytes(StandardCharsets.UTF_8)))
                    .getStatusLine().getStatusCode();
            if (GONE.contains(status)) {
                log.info("Подписка {} мертва ({}), удаляю", subscription.id(), status);
                subscriptions.delete(subscription);
                return;
            }
            if (status >= 300) {
                log.warn("Push-сервис ответил {} на подписку {}", status, subscription.id());
                return;
            }
            subscription.sent(clock.instant());
            subscriptions.save(subscription);
        } catch (final InterruptedException e) {
            Thread.currentThread().interrupt();
        } catch (final Exception e) {
            // Недоступный push-сервис не должен мешать игре: уведомление — не часть партии.
            log.warn("Не удалось отправить уведомление на подписку {}: {}",
                    subscription.id(), e.toString());
        }
    }
}
