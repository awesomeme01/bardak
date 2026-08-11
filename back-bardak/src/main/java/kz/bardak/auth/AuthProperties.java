package kz.bardak.auth;

import java.time.Duration;
import java.util.List;
import java.util.Set;
import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Настройки авторизации. Ни одного из этих значений нет в коде: секрет приходит
 * из окружения, сроки жизни — из конфига стенда.
 *
 * @param jwtSecret   ключ подписи access-токенов, HMAC-SHA256
 * @param accessTtl   срок жизни access-токена
 * @param refreshTtl  срок жизни refresh-токена
 * @param wsTicketTtl срок жизни одноразового WS-тикета (ADR-005)
 * @param inviteCodes коды приглашения; регистрация закрытая
 */
@ConfigurationProperties(prefix = "bardak.auth")
public record AuthProperties(
        String jwtSecret,
        Duration accessTtl,
        Duration refreshTtl,
        Duration wsTicketTtl,
        List<String> inviteCodes) {

    /** Минимальная длина ключа для HS256 — иначе Nimbus справедливо ругается. */
    private static final int MIN_SECRET_LENGTH = 32;

    public AuthProperties {
        if (jwtSecret == null || jwtSecret.length() < MIN_SECRET_LENGTH) {
            throw new IllegalArgumentException(
                    "bardak.auth.jwt-secret должен быть не короче %d символов".formatted(MIN_SECRET_LENGTH));
        }
        inviteCodes = List.copyOf(inviteCodes == null ? List.of() : inviteCodes);
    }

    public boolean isInviteCodeValid(final String code) {
        return code != null && Set.copyOf(inviteCodes).contains(code.trim());
    }
}
