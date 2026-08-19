package kz.bardak.web;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.Map;
import java.util.concurrent.Callable;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import kz.bardak.TestPostgres;
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
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.testcontainers.containers.PostgreSQLContainer;

/**
 * Ответы на кривые и недобросовестные запросы.
 *
 * <p>⚠️ Проверки появились после разбора кода: всё перечисленное отвечало <b>500</b>
 * либо пускало туда, куда не должно. Клиент не мог отличить свою ошибку от поломки
 * сервера, а в лог сыпались стеки там, где ничего не ломалось.
 */
@Tag("integration")
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class ApiHardeningIT {

    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

    @Autowired
    private TestRestTemplate rest;

    @Autowired
    private kz.bardak.push.domain.PushSubscriptionRepository subscriptions;

    @DisplayName("Should answer bad request When the path variable is not a uuid")
    @Test
    void shouldAnswerBadRequestWhenThePathVariableIsNotAUuid() {
        final String token = register("hard-uuid");

        final ResponseEntity<JsonNode> response = rest.exchange("/api/tables/это-не-uuid",
                HttpMethod.GET, new HttpEntity<>(authorized(token)), JsonNode.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
        assertThat(response.getBody().get("code").asText()).isEqualTo("BAD_REQUEST");
        assertThat(response.getBody().get("traceId").asText()).isNotBlank();
    }

    @DisplayName("Should answer bad request When the body is not valid json")
    @Test
    void shouldAnswerBadRequestWhenTheBodyIsNotValidJson() {
        final HttpHeaders headers = new HttpHeaders();
        headers.setContentType(MediaType.APPLICATION_JSON);

        final ResponseEntity<JsonNode> response = rest.exchange("/api/auth/login", HttpMethod.POST,
                new HttpEntity<>("{это не json", headers), JsonNode.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
        assertThat(response.getBody().get("code").asText()).isEqualTo("BAD_REQUEST");
    }

    @DisplayName("Should answer method not allowed When the http method is wrong")
    @Test
    void shouldAnswerMethodNotAllowedWhenTheHttpMethodIsWrong() {
        final ResponseEntity<JsonNode> response = rest.exchange("/api/health", HttpMethod.DELETE,
                HttpEntity.EMPTY, JsonNode.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.METHOD_NOT_ALLOWED);
    }

    @DisplayName("Should refuse a blank season name When a season is opened")
    @Test
    void shouldRefuseABlankSeasonNameWhenASeasonIsOpened() {
        final String token = register("hard-season");

        final ResponseEntity<JsonNode> response = rest.exchange("/api/rating/seasons",
                HttpMethod.POST, new HttpEntity<>(java.util.Collections.singletonMap("name", null),
                        authorized(token)), JsonNode.class);

        // ⚠️ Раньше null доходил до Season.open и давал 500. Теперь это отказ по валидации;
        // не-ведущий при этом получил бы 403 — оба варианта означают «не 500».
        assertThat(response.getStatusCode())
                .isIn(HttpStatus.BAD_REQUEST, HttpStatus.FORBIDDEN);
    }

    @DisplayName("Should keep the subscription When another player tries to unsubscribe it")
    @Test
    void shouldKeepTheSubscriptionWhenAnotherPlayerTriesToUnsubscribeIt() {
        final String owner = register("hard-owner");
        final String stranger = register("hard-stranger");
        final String endpoint = "https://push.example/endpoint-" + System.nanoTime();

        rest.exchange("/api/push/subscriptions", HttpMethod.POST,
                new HttpEntity<>(Map.of("endpoint", endpoint, "p256dh", "key", "auth", "secret"),
                        authorized(owner)), Void.class);

        // ⚠️ Чужой знает endpoint и пытается отписать чужое устройство.
        rest.exchange("/api/push/subscriptions", HttpMethod.DELETE,
                new HttpEntity<>(Map.of("endpoint", endpoint), authorized(stranger)), Void.class);

        // Подписка обязана уцелеть: иначе человек молча перестаёт получать зов к столу.
        assertThat(subscriptions.findByEndpoint(endpoint))
                .as("чужой не должен отписывать чужое устройство")
                .isPresent();

        // А владелец отписывает свою.
        rest.exchange("/api/push/subscriptions", HttpMethod.DELETE,
                new HttpEntity<>(Map.of("endpoint", endpoint), authorized(owner)), Void.class);
        assertThat(subscriptions.findByEndpoint(endpoint))
                .as("владелец обязан отписывать свою подписку")
                .isEmpty();
    }

    @DisplayName("Should answer conflict for exactly one When the same login is registered at once")
    @Test
    void shouldAnswerConflictForExactlyOneWhenTheSameLoginIsRegisteredAtOnce() throws Exception {
        final String username = "hard-race-" + System.nanoTime();
        final Callable<ResponseEntity<JsonNode>> attempt = () -> rest.postForEntity("/api/auth/register",
                Map.of("username", username, "displayName", "Гонка",
                        "password", "very-secret-password", "inviteCode", "bardak-2026"),
                JsonNode.class);

        final ExecutorService pool = Executors.newFixedThreadPool(2);
        try {
            final List<Future<ResponseEntity<JsonNode>>> futures =
                    pool.invokeAll(List.of(attempt, attempt));
            final List<HttpStatus> statuses = List.of(
                    (HttpStatus) futures.get(0).get().getStatusCode(),
                    (HttpStatus) futures.get(1).get().getStatusCode());

            // ⭐ Ровно один заводит учётку, второй получает готовый код, а не 500.
            assertThat(statuses).filteredOn(HttpStatus.OK::equals).hasSize(1);
            assertThat(statuses)
                    .as("проигравший гонку обязан получить 409, а не «что-то пошло не так»")
                    .doesNotContain(HttpStatus.INTERNAL_SERVER_ERROR)
                    .filteredOn(HttpStatus.CONFLICT::equals).hasSize(1);
        } finally {
            pool.shutdownNow();
        }
    }

    private HttpHeaders authorized(final String token) {
        final HttpHeaders headers = new HttpHeaders();
        headers.setBearerAuth(token);
        headers.setContentType(MediaType.APPLICATION_JSON);
        return headers;
    }

    private String register(final String prefix) {
        return rest.postForEntity("/api/auth/register",
                Map.of("username", prefix + "-" + System.nanoTime(), "displayName", "Игрок",
                        "password", "very-secret-password", "inviteCode", "bardak-2026"),
                JsonNode.class).getBody().get("accessToken").asText();
    }
}
