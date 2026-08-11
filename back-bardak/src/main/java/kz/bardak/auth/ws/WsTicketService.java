package kz.bardak.auth.ws;

import java.security.SecureRandom;
import java.time.Clock;
import java.time.Instant;
import java.util.Base64;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import kz.bardak.auth.AuthProperties;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;

/**
 * Одноразовые тикеты для WS-рукопожатия (ADR-005).
 *
 * <p>Браузерный {@code WebSocket} не умеет слать заголовок {@code Authorization}, а полный
 * JWT в query утекает в логи прокси и живёт часами. Тикет живёт секунды и сгорает при
 * первом использовании — в тех же логах он бесполезен.
 *
 * <p>⭐ Погашение — через {@link ConcurrentHashMap#remove}: операция атомарная, поэтому
 * двух одновременных подключений по одному тикету быть не может.
 *
 * <p>Хранилище в памяти, то есть тикет действует только на том инстансе, что его выдал.
 * Пока инстанс один — этого достаточно; общий кеш появится вместе с Redis на M8.
 */
@Service
public class WsTicketService {

    private static final int TICKET_BYTES = 24;

    private final Map<String, Issued> tickets = new ConcurrentHashMap<>();
    private final AuthProperties properties;
    private final Clock clock;
    private final SecureRandom random = new SecureRandom();

    public WsTicketService(final AuthProperties properties, final Clock clock) {
        this.properties = Objects.requireNonNull(properties, "properties");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    public String issue(final UUID userId) {
        Objects.requireNonNull(userId, "userId");
        final byte[] bytes = new byte[TICKET_BYTES];
        random.nextBytes(bytes);
        final String ticket = Base64.getUrlEncoder().withoutPadding().encodeToString(bytes);
        tickets.put(ticket, new Issued(userId, clock.instant().plus(properties.wsTicketTtl())));
        return ticket;
    }

    /** Гасит тикет и возвращает игрока. Повторный вызов с тем же тикетом вернёт пусто. */
    public Optional<UUID> consume(final String ticket) {
        if (ticket == null) {
            return Optional.empty();
        }
        return Optional.ofNullable(tickets.remove(ticket))
                .filter(issued -> issued.expiresAt().isAfter(clock.instant()))
                .map(Issued::userId);
    }

    public long ttlSeconds() {
        return properties.wsTicketTtl().toSeconds();
    }

    /** Уборка протухших: их никто не погасит, а память они занимают. */
    @Scheduled(fixedDelayString = "PT1M")
    public void evictExpired() {
        final Instant now = clock.instant();
        tickets.values().removeIf(issued -> !issued.expiresAt().isAfter(now));
    }

    private record Issued(UUID userId, Instant expiresAt) {
    }
}
