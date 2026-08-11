package kz.bardak.game;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.net.URI;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.TimeUnit;
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
import org.springframework.web.socket.CloseStatus;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketHttpHeaders;
import org.springframework.web.socket.WebSocketSession;
import org.springframework.web.socket.client.standard.StandardWebSocketClient;
import org.springframework.web.socket.handler.TextWebSocketHandler;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

/**
 * Таймеры стола (§5.1–5.3). Времена взяты короткими из конфига: правило проверяется
 * то же самое, а тест не ждёт минуту.
 */
@Tag("integration")
@Testcontainers
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = {"bardak.game.turn-timeout=1s", "bardak.game.disconnect-grace=2s"})
class TimersIT {

    private static final ObjectMapper JSON = new ObjectMapper();

    @Container
    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = new PostgreSQLContainer<>("postgres:16-alpine");

    @Autowired
    private TestRestTemplate rest;

    @LocalServerPort
    private int port;

    @DisplayName("Should act for the silent player When the turn times out")
    @Test
    void shouldActForTheSilentPlayerWhenTheTurnTimesOut() throws Exception {
        final Table table = startedMatch("tmr-a", "tmr-b");

        // Никто не ходит: сервер обязан сделать самое безобидное действие сам.
        assertThat(await(table.hostInbox(), "TURN_TIMEOUT", 10)).isNotNull();

        table.close();
    }

    @DisplayName("Should pause the match When a player drops out")
    @Test
    void shouldPauseTheMatchWhenAPlayerDropsOut() throws Exception {
        final Table table = startedMatch("tmr-c", "tmr-d");

        table.guestSocket().close(CloseStatus.NORMAL);

        final JsonNode paused = await(table.hostInbox(), "MATCH_PAUSED", 10);
        assertThat(paused.get("graceSeconds").asInt()).isEqualTo(2);
        table.hostSocket().close();
    }

    @DisplayName("Should abort the match When the player never comes back")
    @Test
    void shouldAbortTheMatchWhenThePlayerNeverComesBack() throws Exception {
        final Table table = startedMatch("tmr-e", "tmr-f");

        table.guestSocket().close(CloseStatus.NORMAL);
        await(table.hostInbox(), "MATCH_PAUSED", 10);

        assertThat(await(table.hostInbox(), "MATCH_ABORTED", 20)).isNotNull();
        table.hostSocket().close();
    }

    private Table startedMatch(final String hostName, final String guestName) throws Exception {
        final String hostToken = register(hostName);
        final String guestToken = register(guestName);
        final String tableId = createTable(hostToken);

        final BlockingQueue<String> hostInbox = new LinkedBlockingQueue<>();
        final BlockingQueue<String> guestInbox = new LinkedBlockingQueue<>();
        final WebSocketSession hostSocket = connect(hostToken, hostInbox);
        final WebSocketSession guestSocket = connect(guestToken, guestInbox);

        send(hostSocket, "TABLE_JOIN", tableId, null);
        send(guestSocket, "TABLE_JOIN", tableId, null);
        send(hostSocket, "TABLE_READY", tableId, Map.of("ready", true));
        send(guestSocket, "TABLE_READY", tableId, Map.of("ready", true));
        Thread.sleep(400);
        send(hostSocket, "MATCH_START", tableId, null);
        await(hostInbox, "STATE_SYNC", 10);

        return new Table(hostSocket, guestSocket, hostInbox);
    }

    private JsonNode await(final BlockingQueue<String> inbox, final String type, final int seconds)
            throws Exception {
        final long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(seconds);
        while (System.nanoTime() < deadline) {
            final String message = inbox.poll(1, TimeUnit.SECONDS);
            if (message == null) {
                continue;
            }
            final JsonNode envelope = JSON.readTree(message);
            if (type.equals(envelope.path("type").asText())) {
                return envelope.path("payload");
            }
        }
        throw new AssertionError("Не дождался сообщения " + type);
    }

    private void send(final WebSocketSession socket, final String type, final String tableId,
                      final Map<String, Object> payload) throws Exception {
        final Map<String, Object> envelope = new LinkedHashMap<>();
        envelope.put("v", 1);
        envelope.put("id", type + "-" + System.nanoTime());
        envelope.put("type", type);
        envelope.put("tableId", tableId);
        if (payload != null) {
            envelope.put("payload", payload);
        }
        socket.sendMessage(new TextMessage(JSON.writeValueAsString(envelope)));
    }

    private WebSocketSession connect(final String token, final BlockingQueue<String> inbox)
            throws Exception {
        final HttpHeaders headers = new HttpHeaders();
        headers.setBearerAuth(token);
        final JsonNode ticket = rest.exchange("/api/auth/ws-ticket", HttpMethod.POST,
                new HttpEntity<>(headers), JsonNode.class).getBody();
        return new StandardWebSocketClient().execute(new TextWebSocketHandler() {
            @Override
            protected void handleTextMessage(final WebSocketSession session, final TextMessage message) {
                inbox.add(message.getPayload());
            }
        }, new WebSocketHttpHeaders(),
                URI.create("ws://localhost:" + port + "/ws?ticket=" + ticket.get("ticket").asText())
        ).get(5, TimeUnit.SECONDS);
    }

    private String createTable(final String token) {
        final HttpHeaders headers = new HttpHeaders();
        headers.setBearerAuth(token);
        headers.setContentType(org.springframework.http.MediaType.APPLICATION_JSON);
        return rest.exchange("/api/tables", HttpMethod.POST,
                new HttpEntity<>(Map.of("name", "Таймеры", "maxPlayers", 2, "isPrivate", false), headers),
                JsonNode.class).getBody().get("id").asText();
    }

    private String register(final String username) {
        return rest.postForEntity("/api/auth/register",
                Map.of("username", username, "displayName", "Игрок", "password", "very-secret-password",
                        "inviteCode", "bardak-2026"), JsonNode.class)
                .getBody().get("accessToken").asText();
    }

    private record Table(WebSocketSession hostSocket, WebSocketSession guestSocket,
                         BlockingQueue<String> hostInbox) {

        void close() throws Exception {
            hostSocket.close();
            guestSocket.close();
        }
    }
}
