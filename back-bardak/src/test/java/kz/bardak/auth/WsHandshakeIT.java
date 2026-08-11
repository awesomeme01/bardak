package kz.bardak.auth;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.fasterxml.jackson.databind.JsonNode;
import java.net.URI;
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
import org.springframework.http.HttpStatus;
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
 * Рукопожатие WebSocket по одноразовому тикету (ADR-005) — главный критерий готовности M2:
 * незалогиненный не открывает соединение.
 */
@Tag("integration")
@Testcontainers
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class WsHandshakeIT {

    @Container
    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = new PostgreSQLContainer<>("postgres:16-alpine");

    @Autowired
    private TestRestTemplate rest;

    @LocalServerPort
    private int port;

    @DisplayName("Should refuse the handshake When no ticket is given")
    @Test
    void shouldRefuseTheHandshakeWhenNoTicketIsGiven() {
        assertThatThrownBy(() -> connect(null)).hasMessageContaining("401");
    }

    @DisplayName("Should refuse the handshake When the ticket is made up")
    @Test
    void shouldRefuseTheHandshakeWhenTheTicketIsMadeUp() {
        assertThatThrownBy(() -> connect("not-a-real-ticket")).hasMessageContaining("401");
    }

    @DisplayName("Should let the player in and answer the heartbeat When the ticket is valid")
    @Test
    void shouldLetThePlayerInAndAnswerTheHeartbeatWhenTheTicketIsValid() throws Exception {
        final BlockingQueue<String> received = new LinkedBlockingQueue<>();
        final WebSocketSession session = connect(ticketFor("ws-player"), received);

        // Первым приходит приветствие CONNECTED, PONG — ответ на heartbeat.
        assertThat(received.poll(5, TimeUnit.SECONDS)).contains("CONNECTED");
        session.sendMessage(new TextMessage("{\"v\":1,\"id\":\"c-1\",\"type\":\"PING\"}"));

        assertThat(received.poll(5, TimeUnit.SECONDS)).contains("PONG");
        session.close(CloseStatus.NORMAL);
    }

    @DisplayName("Should burn the ticket When the same one is used twice")
    @Test
    void shouldBurnTheTicketWhenTheSameOneIsUsedTwice() throws Exception {
        final String ticket = ticketFor("ws-twice");
        connect(ticket).close(CloseStatus.NORMAL);

        assertThatThrownBy(() -> connect(ticket)).hasMessageContaining("401");
    }

    private String ticketFor(final String username) {
        final JsonNode tokens = rest.postForEntity("/api/auth/register",
                Map.of("username", username, "displayName", "Игрок", "password", "very-secret-password",
                        "inviteCode", "bardak-2026"), JsonNode.class).getBody();
        final HttpHeaders headers = new HttpHeaders();
        headers.setBearerAuth(tokens.get("accessToken").asText());
        final var response = rest.exchange("/api/auth/ws-ticket", org.springframework.http.HttpMethod.POST,
                new HttpEntity<>(headers), JsonNode.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
        return response.getBody().get("ticket").asText();
    }

    private WebSocketSession connect(final String ticket) throws Exception {
        return connect(ticket, new LinkedBlockingQueue<>());
    }

    private WebSocketSession connect(final String ticket, final BlockingQueue<String> received)
            throws Exception {
        final String query = ticket == null ? "" : "?ticket=" + ticket;
        return new StandardWebSocketClient()
                .execute(new TextWebSocketHandler() {
                    @Override
                    protected void handleTextMessage(final WebSocketSession session, final TextMessage message) {
                        received.add(message.getPayload());
                    }
                }, new WebSocketHttpHeaders(), URI.create("ws://localhost:" + port + "/ws" + query))
                .get(5, TimeUnit.SECONDS);
    }
}
