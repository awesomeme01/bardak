package kz.bardak.auth;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.List;
import java.util.Optional;
import java.util.UUID;
import kz.bardak.auth.domain.RefreshToken;
import kz.bardak.auth.domain.RefreshTokenRepository;
import kz.bardak.common.web.ApiException;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

/**
 * Ротация refresh-токенов. Главное здесь — что предъявление уже отозванного токена
 * считается кражей и валит всю серию пользователя.
 */
@ExtendWith(MockitoExtension.class)
class RefreshTokenServiceTest {

    private static final Instant NOW = Instant.parse("2026-08-11T10:00:00Z");
    private static final UUID USER = UUID.fromString("00000000-0000-0000-0000-0000000000aa");

    @Mock
    private RefreshTokenRepository repository;

    @Mock
    private TokenSeriesRevoker seriesRevoker;

    @DisplayName("Should store only the hash When a token is issued")
    @Test
    void shouldStoreOnlyTheHashWhenATokenIsIssued() {
        final RefreshTokenService service = service();

        final String token = service.issue(USER, "JUnit");

        final ArgumentCaptor<RefreshToken> saved = ArgumentCaptor.forClass(RefreshToken.class);
        verify(repository).save(saved.capture());
        assertThat(token).isNotBlank();
        assertThat(saved.getValue().userId()).isEqualTo(USER);
        assertThat(saved.getValue().expiresAt()).isEqualTo(NOW.plus(Duration.ofDays(30)));
    }

    @DisplayName("Should revoke the presented token When it is exchanged")
    @Test
    void shouldRevokeThePresentedTokenWhenItIsExchanged() {
        final RefreshToken stored = usableToken();
        when(repository.findByTokenHash(any())).thenReturn(Optional.of(stored));

        final UUID userId = service().rotate("whatever", "JUnit");

        assertThat(userId).isEqualTo(USER);
        assertThat(stored.isRevoked()).isTrue();
        verify(seriesRevoker, never()).revokeAll(any(), any());
    }

    @DisplayName("Should revoke the whole series When an already revoked token is presented")
    @Test
    void shouldRevokeTheWholeSeriesWhenAnAlreadyRevokedTokenIsPresented() {
        final RefreshToken stored = usableToken();
        stored.revoke(NOW.minusSeconds(60));
        when(repository.findByTokenHash(any())).thenReturn(Optional.of(stored));

        assertThatThrownBy(() -> service().rotate("stolen", "JUnit"))
                .isInstanceOf(ApiException.class)
                .satisfies(thrown -> assertThat(((ApiException) thrown).code())
                        .isEqualTo("REFRESH_TOKEN_INVALID"));

        verify(seriesRevoker).revokeAll(eq(USER), eq(NOW));
    }

    @DisplayName("Should refuse an expired token When it is exchanged")
    @Test
    void shouldRefuseAnExpiredTokenWhenItIsExchanged() {
        final RefreshToken expired = new RefreshToken(UUID.randomUUID(), USER, "hash",
                NOW.minusSeconds(1), "JUnit");
        when(repository.findByTokenHash(any())).thenReturn(Optional.of(expired));

        assertThatThrownBy(() -> service().rotate("old", "JUnit")).isInstanceOf(ApiException.class);
        verify(seriesRevoker, never()).revokeAll(any(), any());
    }

    @DisplayName("Should refuse an unknown token When it is exchanged")
    @Test
    void shouldRefuseAnUnknownTokenWhenItIsExchanged() {
        when(repository.findByTokenHash(any())).thenReturn(Optional.empty());

        assertThatThrownBy(() -> service().rotate("never-issued", "JUnit"))
                .isInstanceOf(ApiException.class);
    }

    @DisplayName("Should issue different tokens every time When called twice")
    @Test
    void shouldIssueDifferentTokensEveryTimeWhenCalledTwice() {
        final RefreshTokenService service = service();

        assertThat(service.issue(USER, "JUnit")).isNotEqualTo(service.issue(USER, "JUnit"));
    }

    private RefreshTokenService service() {
        return new RefreshTokenService(repository, seriesRevoker, properties(), Clock.fixed(NOW, ZoneOffset.UTC));
    }

    private RefreshToken usableToken() {
        return new RefreshToken(UUID.randomUUID(), USER, "hash", NOW.plusSeconds(3600), "JUnit");
    }

    private static AuthProperties properties() {
        return new AuthProperties("test-secret-that-is-long-enough-32+", Duration.ofMinutes(15),
                Duration.ofDays(30), Duration.ofSeconds(30), List.of("invite"));
    }
}
