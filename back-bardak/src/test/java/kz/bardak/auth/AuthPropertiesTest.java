package kz.bardak.auth;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.Duration;
import java.util.List;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

/**
 * Проверка кода приглашения.
 *
 * <p>Регистр отдельно вынесен в тесты: поле кода на экране регистрации поднимает ввод
 * в верхний регистр, и регистрозависимая проверка делала форму нерабочей.
 */
class AuthPropertiesTest {

    @DisplayName("Should accept the code When it is typed exactly as configured")
    @Test
    void shouldAcceptTheCodeWhenItIsTypedExactlyAsConfigured() {
        assertThat(properties("bardak-2026").isInviteCodeValid("bardak-2026")).isTrue();
    }

    @DisplayName("Should accept the code When the case differs")
    @Test
    void shouldAcceptTheCodeWhenTheCaseDiffers() {
        final AuthProperties properties = properties("bardak-2026");

        assertThat(properties.isInviteCodeValid("BARDAK-2026")).isTrue();
        assertThat(properties.isInviteCodeValid("Bardak-2026")).isTrue();
    }

    @DisplayName("Should accept the code When it is padded with spaces")
    @Test
    void shouldAcceptTheCodeWhenItIsPaddedWithSpaces() {
        assertThat(properties("bardak-2026").isInviteCodeValid("  BARDAK-2026  ")).isTrue();
    }

    @DisplayName("Should accept any of the codes When several are configured")
    @Test
    void shouldAcceptAnyOfTheCodesWhenSeveralAreConfigured() {
        final AuthProperties properties = properties("first-code", "second-code");

        assertThat(properties.isInviteCodeValid("FIRST-CODE")).isTrue();
        assertThat(properties.isInviteCodeValid("SECOND-CODE")).isTrue();
    }

    @DisplayName("Should refuse the code When it is not configured")
    @Test
    void shouldRefuseTheCodeWhenItIsNotConfigured() {
        assertThat(properties("bardak-2026").isInviteCodeValid("bardak-2027")).isFalse();
    }

    @DisplayName("Should refuse the code When it is null")
    @Test
    void shouldRefuseTheCodeWhenItIsNull() {
        assertThat(properties("bardak-2026").isInviteCodeValid(null)).isFalse();
    }

    @DisplayName("Should refuse any code When none is configured")
    @Test
    void shouldRefuseAnyCodeWhenNoneIsConfigured() {
        assertThat(properties().isInviteCodeValid("bardak-2026")).isFalse();
    }

    private static AuthProperties properties(final String... inviteCodes) {
        return new AuthProperties("test-secret-that-is-long-enough-32+", Duration.ofMinutes(15),
                Duration.ofDays(30), Duration.ofSeconds(30), List.of(inviteCodes));
    }
}
