package kz.bardak.push;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.Map;
import java.util.UUID;
import kz.bardak.TestPostgres;
import kz.bardak.push.domain.PushSubscriptionRepository;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.springframework.http.HttpEntity;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpMethod;
import org.springframework.http.HttpStatus;
import org.testcontainers.containers.PostgreSQLContainer;

/**
 * Подписка устройства на уведомления.
 *
 * <p>⭐ Главное — что повторная подписка не плодит строк: браузер присылает ту же
 * подписку при каждом запуске приложения, и без склейки игрок получал бы один и тот же
 * звонок столько раз, сколько раз открывал вкладку.
 */
@Tag("integration")
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = {
                // ⭐ Ключи настоящие, но одноразовые — сгенерированы для тестов и никуда
                // не отправляют: библиотека проверяет, что это точка на кривой P-256,
                // и на выдуманной строке падает ещё при старте контекста.
                "bardak.push.public-key=BKaJamQidNeXWPgyhpgqBFtOo9H4g6Iyyws0wBL_ub1VeMgtdLM18dpNvuWeJj7LcTlxYBwttoYUnhrN_fOTL08",
                "bardak.push.private-key=9PZ-3BcLpSMDdYZqJ1bx526i08qJzHqNpFGZUu6L_K8"})
class PushSubscriptionIT {

    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

    @Autowired
    private TestRestTemplate rest;

    @Autowired
    private PushSubscriptionRepository subscriptions;

    @DisplayName("Should give the public key When push is configured")
    @Test
    void shouldGiveThePublicKeyWhenPushIsConfigured() {
        final String token = register("push-key");

        final JsonNode key = get("/api/push/key", token);

        assertThat(key.get("enabled").asBoolean()).isTrue();
        assertThat(key.get("publicKey").asText()).isNotBlank();
    }

    @DisplayName("Should keep a single row When the same device subscribes twice")
    @Test
    void shouldKeepASingleRowWhenTheSameDeviceSubscribesTwice() {
        final String token = register("push-twice");
        final String endpoint = "https://push.example.test/" + UUID.randomUUID();

        subscribe(token, endpoint);
        subscribe(token, endpoint);

        assertThat(subscriptions.findByEndpoint(endpoint)).isPresent();
        assertThat(subscriptions.findAll().stream()
                .filter(row -> row.endpoint().equals(endpoint))
                .count()).isEqualTo(1);
    }

    @DisplayName("Should move the device When another player subscribes with the same endpoint")
    @Test
    void shouldMoveTheDeviceWhenAnotherPlayerSubscribesWithTheSameEndpoint() {
        final String first = register("push-owner-a");
        final String second = register("push-owner-b");
        final String endpoint = "https://push.example.test/" + UUID.randomUUID();
        subscribe(first, endpoint);

        subscribe(second, endpoint);

        // ⭐ Телефон перешёл к другому игроку: звонить на него прежнему владельцу нельзя.
        final UUID ownerId = subscriptions.findByEndpoint(endpoint).orElseThrow().userId();
        assertThat(ownerId).isEqualTo(UUID.fromString(get("/api/profile", second).get("id").asText()));
    }

    @DisplayName("Should forget the device When it unsubscribes")
    @Test
    void shouldForgetTheDeviceWhenItUnsubscribes() {
        final String token = register("push-off");
        final String endpoint = "https://push.example.test/" + UUID.randomUUID();
        subscribe(token, endpoint);

        rest.exchange("/api/push/subscriptions", HttpMethod.DELETE,
                new HttpEntity<>(Map.of("endpoint", endpoint), authorized(token)), Void.class);

        assertThat(subscriptions.findByEndpoint(endpoint)).isEmpty();
    }

    @DisplayName("Should refuse the subscription When there is no token")
    @Test
    void shouldRefuseTheSubscriptionWhenThereIsNoToken() {
        final var response = rest.postForEntity("/api/push/subscriptions",
                Map.of("endpoint", "https://push.example.test/anon", "p256dh", "key", "auth", "salt"),
                JsonNode.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
    }

    private void subscribe(final String token, final String endpoint) {
        final var response = rest.exchange("/api/push/subscriptions", HttpMethod.POST,
                new HttpEntity<>(Map.of("endpoint", endpoint, "p256dh", "BPublicKey", "auth", "salt"),
                        authorized(token)), Void.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.NO_CONTENT);
    }

    private JsonNode get(final String path, final String token) {
        return rest.exchange(path, HttpMethod.GET, new HttpEntity<>(authorized(token)),
                JsonNode.class).getBody();
    }

    private HttpHeaders authorized(final String token) {
        final HttpHeaders headers = new HttpHeaders();
        headers.setBearerAuth(token);
        headers.setContentType(org.springframework.http.MediaType.APPLICATION_JSON);
        return headers;
    }

    private String register(final String username) {
        final String suffix = UUID.randomUUID().toString().substring(0, 6);
        return rest.postForEntity("/api/auth/register",
                        Map.of("username", username + "-" + suffix, "displayName", "Игрок",
                                "password", "very-secret-password", "inviteCode", "bardak-2026"),
                        JsonNode.class)
                .getBody().get("accessToken").asText();
    }
}
