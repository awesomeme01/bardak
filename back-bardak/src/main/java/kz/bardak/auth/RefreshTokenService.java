package kz.bardak.auth;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.security.SecureRandom;
import java.time.Clock;
import java.time.Instant;
import java.util.Base64;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.auth.domain.RefreshToken;
import kz.bardak.auth.domain.RefreshTokenRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import kz.bardak.common.web.ApiException;

/**
 * Выдача, ротация и отзыв refresh-токенов.
 *
 * <p>⭐ Токен непрозрачный и случайный, а не JWT: его всё равно надо проверять по базе,
 * чтобы уметь отзывать, — подпись тут ничего не добавляет. В базе лежит только SHA-256
 * хеш (`V1__users_and_auth.sql`).
 */
@Service
public class RefreshTokenService {

    private static final Logger log = LoggerFactory.getLogger(RefreshTokenService.class);
    private static final int TOKEN_BYTES = 32;

    private final RefreshTokenRepository repository;
    private final TokenSeriesRevoker seriesRevoker;
    private final AuthProperties properties;
    private final Clock clock;
    private final SecureRandom random = new SecureRandom();

    public RefreshTokenService(final RefreshTokenRepository repository, final TokenSeriesRevoker seriesRevoker,
                               final AuthProperties properties, final Clock clock) {
        this.repository = Objects.requireNonNull(repository, "repository");
        this.seriesRevoker = Objects.requireNonNull(seriesRevoker, "seriesRevoker");
        this.properties = Objects.requireNonNull(properties, "properties");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    /** Новый токен для пользователя. Возвращается открытая часть — её больше негде взять. */
    @Transactional
    public String issue(final UUID userId, final String userAgent) {
        final byte[] bytes = new byte[TOKEN_BYTES];
        random.nextBytes(bytes);
        final String token = Base64.getUrlEncoder().withoutPadding().encodeToString(bytes);
        repository.save(new RefreshToken(UUID.randomUUID(), userId, hash(token),
                clock.instant().plus(properties.refreshTtl()), userAgent));
        return token;
    }

    /**
     * Обмен токена на новый.
     *
     * <p>⭐ Предъявленный, но уже отозванный токен означает кражу: настоящий владелец
     * и вор не могут ротировать одну и ту же строку одновременно. В этом случае отзывается
     * <b>вся серия</b> пользователя — дешевле выкинуть всех, чем оставить вора внутри.
     */
    @Transactional
    public UUID rotate(final String presentedToken, final String userAgent) {
        final Instant now = clock.instant();
        final RefreshToken stored = repository.findByTokenHash(hash(presentedToken))
                .orElseThrow(RefreshTokenService::invalidToken);

        if (stored.isRevoked()) {
            // Отдельная транзакция: этот метод сейчас упадёт, и общая откатилась бы.
            final int revoked = seriesRevoker.revokeAll(stored.userId(), now);
            log.warn("Повторное использование отозванного refresh-токена, отозвана вся серия: "
                    + "user={} tokens={}", stored.userId(), revoked);
            throw invalidToken();
        }
        if (stored.isExpiredAt(now)) {
            throw invalidToken();
        }
        stored.revoke(now);
        repository.save(stored);
        return stored.userId();
    }

    @Transactional
    public void revoke(final String presentedToken) {
        repository.findByTokenHash(hash(presentedToken))
                .filter(token -> token.isUsableAt(clock.instant()))
                .ifPresent(token -> {
                    token.revoke(clock.instant());
                    repository.save(token);
                });
    }

    public long refreshTtlSeconds() {
        return properties.refreshTtl().toSeconds();
    }

    private static ApiException invalidToken() {
        return new ApiException(HttpStatus.UNAUTHORIZED, "REFRESH_TOKEN_INVALID",
                "Сессия истекла, войдите заново");
    }

    private String hash(final String token) {
        Objects.requireNonNull(token, "token");
        try {
            final MessageDigest digest = MessageDigest.getInstance("SHA-256");
            return Base64.getEncoder().encodeToString(digest.digest(token.getBytes(StandardCharsets.UTF_8)));
        } catch (final NoSuchAlgorithmException e) {
            throw new IllegalStateException("SHA-256 недоступен", e);
        }
    }
}
