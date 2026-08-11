package kz.bardak.auth;

import java.time.Instant;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.auth.domain.RefreshTokenRepository;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Propagation;
import org.springframework.transaction.annotation.Transactional;

/**
 * Отзыв всей серии токенов пользователя.
 *
 * <p>⭐ Вынесено в отдельный бин ради {@link Propagation#REQUIRES_NEW}: отзыв происходит
 * при <b>краже</b> токена, а обмен в этот момент завершается исключением. В общей
 * транзакции отзыв откатился бы вместе с ним — и вор остался бы внутри.
 */
@Service
public class TokenSeriesRevoker {

    private final RefreshTokenRepository repository;

    public TokenSeriesRevoker(final RefreshTokenRepository repository) {
        this.repository = Objects.requireNonNull(repository, "repository");
    }

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public int revokeAll(final UUID userId, final Instant now) {
        return repository.revokeAllForUser(userId, now);
    }
}
