package kz.bardak.social;

import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.function.Consumer;
import java.util.stream.Collectors;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

/**
 * Кто сейчас в сети и как до него достучаться.
 *
 * <p>⭐ Присутствие считается по живым сокетам, а не по «последней активности»: игра идёт
 * через WebSocket, и открытое соединение — это и есть присутствие. Отметка времени врала бы
 * в обе стороны: закрывший вкладку числился бы онлайн ещё минуту, а задумавшийся над ходом
 * успел бы «уйти».
 *
 * <p>⭐ Здесь же лежит и способ доставки. «Кто онлайн» и «куда ему писать» — один и тот же
 * факт, и разносить их по двум реестрам значит однажды разойтись: числится в сети, а письмо
 * ушло в закрытый сокет.
 *
 * <p>⚠️ У одного игрока соединений бывает несколько — второе устройство, вкладка рядом.
 * Поэтому хранится набор, а не одно: онлайн заканчивается на последнем сокете, а сообщение
 * уходит во все сразу, иначе приглашение придёт не на то устройство, где человек сидит.
 *
 * <p>Реестр живёт в памяти узла и переживать перезапуск не должен: соединения его тоже
 * не переживают.
 */
@Component
public class Presence {

    private static final Logger log = LoggerFactory.getLogger(Presence.class);

    private final Map<UUID, Map<String, Consumer<String>>> channels = new ConcurrentHashMap<>();

    public void connected(final UUID userId, final String sessionId, final Consumer<String> sender) {
        if (userId == null || sessionId == null) {
            return;
        }
        channels.computeIfAbsent(userId, key -> new ConcurrentHashMap<>()).put(sessionId, sender);
    }

    public void disconnected(final UUID userId, final String sessionId) {
        if (userId == null || sessionId == null) {
            return;
        }
        channels.computeIfPresent(userId, (key, sessions) -> {
            sessions.remove(sessionId);
            return sessions.isEmpty() ? null : sessions;
        });
    }

    public boolean isOnline(final UUID userId) {
        return userId != null && channels.containsKey(userId);
    }

    /** Кто из перечисленных сейчас в сети. Спрашивают всегда про список друзей, не про всех. */
    public Set<UUID> onlineAmong(final Set<UUID> candidates) {
        Objects.requireNonNull(candidates, "candidates");
        return candidates.stream().filter(this::isOnline).collect(Collectors.toSet());
    }

    /**
     * Отправить сообщение игроку во все его соединения.
     *
     * @return дошло ли хоть куда-то; не дошло — значит человек не в сети, и звать надо push-ом
     */
    public boolean send(final UUID userId, final String message) {
        final Map<String, Consumer<String>> sessions = channels.get(userId);
        if (sessions == null || sessions.isEmpty()) {
            return false;
        }
        boolean delivered = false;
        for (final Consumer<String> sender : sessions.values()) {
            try {
                sender.accept(message);
                delivered = true;
            } catch (final RuntimeException e) {
                // Одно мёртвое соединение не должно мешать остальным получить сообщение.
                log.debug("Не доставил сообщение игроку {}: {}", userId, e.toString());
            }
        }
        return delivered;
    }
}
