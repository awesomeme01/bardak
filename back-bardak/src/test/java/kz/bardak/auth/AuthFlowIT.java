package kz.bardak.auth;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.Map;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.boot.test.web.server.LocalServerPort;
import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.springframework.http.HttpEntity;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpMethod;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

/**
 * Полный путь авторизации на настоящем Postgres — той же версии, что в проде.
 *
 * <p>Проверяется не только счастливый путь: закрытая регистрация, неразличимые ответы
 * на неверный логин и пароль, ротация refresh и отзыв всей серии при повторном
 * использовании.
 */
@Tag("integration")
@Testcontainers
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class AuthFlowIT {

    private static final String INVITE = "bardak-2026";

    @Container
    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = new PostgreSQLContainer<>("postgres:16-alpine");

    @Autowired
    private TestRestTemplate rest;

    @LocalServerPort
    private int port;

    @DisplayName("Should refuse the registration When the invite code is wrong")
    @Test
    void shouldRefuseTheRegistrationWhenTheInviteCodeIsWrong() {
        final ResponseEntity<JsonNode> response = rest.postForEntity("/api/auth/register",
                registration("nocode", "wrong-code"), JsonNode.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.FORBIDDEN);
        assertThat(response.getBody().get("code").asText()).isEqualTo("INVALID_INVITE_CODE");
    }

    @DisplayName("Should hand out a token pair When the invite code is right")
    @Test
    void shouldHandOutATokenPairWhenTheInviteCodeIsRight() {
        final JsonNode tokens = register("newcomer");

        assertThat(tokens.get("accessToken").asText()).isNotBlank();
        assertThat(tokens.get("refreshToken").asText()).isNotBlank();
        assertThat(tokens.get("user").get("username").asText()).isEqualTo("newcomer");
        assertThat(tokens.get("user").has("passwordHash")).isFalse();
    }

    @DisplayName("Should answer the same way When the login or the password is wrong")
    @Test
    void shouldAnswerTheSameWayWhenTheLoginOrThePasswordIsWrong() {
        register("existing");

        final ResponseEntity<JsonNode> wrongPassword = rest.postForEntity("/api/auth/login",
                Map.of("username", "existing", "password", "not-the-password"), JsonNode.class);
        final ResponseEntity<JsonNode> unknownUser = rest.postForEntity("/api/auth/login",
                Map.of("username", "ghost-user", "password", "not-the-password"), JsonNode.class);

        assertThat(wrongPassword.getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
        assertThat(unknownUser.getStatusCode()).isEqualTo(wrongPassword.getStatusCode());
        assertThat(unknownUser.getBody().get("code").asText())
                .isEqualTo(wrongPassword.getBody().get("code").asText())
                .isEqualTo("INVALID_CREDENTIALS");
    }

    @DisplayName("Should refuse the profile When no token is presented")
    @Test
    void shouldRefuseTheProfileWhenNoTokenIsPresented() {
        assertThat(rest.getForEntity("/api/profile", JsonNode.class).getStatusCode())
                .isEqualTo(HttpStatus.UNAUTHORIZED);
    }

    @DisplayName("Should show my name When the access token is presented")
    @Test
    void shouldShowMyNameWhenTheAccessTokenIsPresented() {
        final JsonNode tokens = register("profile-owner");

        final ResponseEntity<JsonNode> response = rest.exchange("/api/profile", HttpMethod.GET,
                new HttpEntity<>(bearer(tokens)), JsonNode.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
        assertThat(response.getBody().get("username").asText()).isEqualTo("profile-owner");
    }

    @DisplayName("Should rotate the pair and kill the old token When refresh is used")
    @Test
    void shouldRotateThePairAndKillTheOldTokenWhenRefreshIsUsed() {
        final JsonNode first = register("rotator");
        final String oldRefresh = first.get("refreshToken").asText();

        final ResponseEntity<JsonNode> rotated = rest.postForEntity("/api/auth/refresh",
                Map.of("refreshToken", oldRefresh), JsonNode.class);

        assertThat(rotated.getStatusCode()).isEqualTo(HttpStatus.OK);
        assertThat(rotated.getBody().get("refreshToken").asText()).isNotEqualTo(oldRefresh);

        final ResponseEntity<JsonNode> reused = rest.postForEntity("/api/auth/refresh",
                Map.of("refreshToken", oldRefresh), JsonNode.class);
        assertThat(reused.getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
    }

    @DisplayName("Should kill the whole series When a revoked refresh token comes back")
    @Test
    void shouldKillTheWholeSeriesWhenARevokedRefreshTokenComesBack() {
        final JsonNode first = register("victim");
        final String stolen = first.get("refreshToken").asText();
        final String live = rest.postForEntity("/api/auth/refresh", Map.of("refreshToken", stolen),
                JsonNode.class).getBody().get("refreshToken").asText();

        rest.postForEntity("/api/auth/refresh", Map.of("refreshToken", stolen), JsonNode.class);

        final ResponseEntity<JsonNode> afterBreach = rest.postForEntity("/api/auth/refresh",
                Map.of("refreshToken", live), JsonNode.class);
        assertThat(afterBreach.getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
    }

    @DisplayName("Should stop the session When logout is called")
    @Test
    void shouldStopTheSessionWhenLogoutIsCalled() {
        final JsonNode tokens = register("leaver");
        final String refresh = tokens.get("refreshToken").asText();

        rest.postForEntity("/api/auth/logout", Map.of("refreshToken", refresh), Void.class);

        assertThat(rest.postForEntity("/api/auth/refresh", Map.of("refreshToken", refresh),
                JsonNode.class).getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
    }

    @DisplayName("Should refuse the ws ticket When the caller is anonymous")
    @Test
    void shouldRefuseTheWsTicketWhenTheCallerIsAnonymous() {
        assertThat(rest.postForEntity("/api/auth/ws-ticket", null, JsonNode.class).getStatusCode())
                .isEqualTo(HttpStatus.UNAUTHORIZED);
    }

    private JsonNode register(final String username) {
        final ResponseEntity<JsonNode> response = rest.postForEntity("/api/auth/register",
                registration(username, INVITE), JsonNode.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
        return response.getBody();
    }

    private static Map<String, String> registration(final String username, final String invite) {
        return Map.of("username", username, "displayName", "Игрок " + username,
                "password", "very-secret-password", "inviteCode", invite);
    }

    private HttpHeaders bearer(final JsonNode tokens) {
        final HttpHeaders headers = new HttpHeaders();
        headers.setBearerAuth(tokens.get("accessToken").asText());
        return headers;
    }
}
