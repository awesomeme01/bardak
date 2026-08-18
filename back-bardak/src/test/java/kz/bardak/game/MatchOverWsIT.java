package kz.bardak.game;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.net.URI;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.TimeUnit;
import kz.bardak.TestPostgres;
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
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketHttpHeaders;
import org.springframework.web.socket.WebSocketSession;
import org.springframework.web.socket.client.standard.StandardWebSocketClient;
import org.springframework.web.socket.handler.TextWebSocketHandler;
import org.testcontainers.containers.PostgreSQLContainer;

/**
 * Путь целиком: двое садятся за стол, объявляют готовность, начинают матч и получают
 * персональные проекции.
 *
 * <p>⭐ Главная проверка — не «карты пришли», а <b>чего в них нет</b>: чужой руки
 * не должно быть ни в одном сообщении, ушедшем клиенту (критерий готовности M4).
 */
@Tag("integration")
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class MatchOverWsIT {

    private static final ObjectMapper JSON = new ObjectMapper();

    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

    @Autowired
    private TestRestTemplate rest;

    @LocalServerPort
    private int port;

    @DisplayName("Should deal a hand to everybody and hide the others When a match starts")
    @Test
    void shouldDealAHandToEverybodyAndHideTheOthersWhenAMatchStarts() throws Exception {
        final Player host = register("mws-host");
        final Player guest = register("mws-guest");
        final String tableId = createTable(host);

        final BlockingQueue<String> hostInbox = new LinkedBlockingQueue<>();
        final BlockingQueue<String> guestInbox = new LinkedBlockingQueue<>();
        final WebSocketSession hostSocket = connect(host, hostInbox);
        final WebSocketSession guestSocket = connect(guest, guestInbox);

        send(hostSocket, "TABLE_JOIN", tableId, null);
        send(guestSocket, "TABLE_JOIN", tableId, null);
        send(hostSocket, "TABLE_READY", tableId, Map.of("ready", true));
        send(guestSocket, "TABLE_READY", tableId, Map.of("ready", true));
        Thread.sleep(400);

        send(hostSocket, "MATCH_START", tableId, null);
        final JsonNode hostState = await(hostInbox, "STATE_SYNC");

        send(guestSocket, "STATE_REQUEST", tableId, null);
        final JsonNode guestState = await(guestInbox, "STATE_SYNC");

        assertThat(hostState.get("myHand")).hasSize(6);
        assertThat(guestState.get("myHand")).hasSize(6);
        assertThat(hostState.get("mySeat").asInt()).isNotEqualTo(guestState.get("mySeat").asInt());

        // ⭐ Козыря может ещё не быть: если нижней картой колоды выпал джокер, масть
        // разыгрывается костью (§1.2), и до выбора поля trumpSuit в проекции нет вообще.
        if ("DICE".equals(hostState.get("phase").asText())) {
            assertThat(hostState.has("trumpSuit")).isFalse();
        } else {
            assertThat(hostState.get("trumpSuit").asText()).isNotBlank();
        }

        // ⭐ Ни одна карта из руки соседа не должна встречаться в моей проекции.
        final List<String> hostHand = cards(hostState.get("myHand"));
        final List<String> guestHand = cards(guestState.get("myHand"));
        assertThat(guestState.toString())
                .withFailMessage("Рука хозяина утекла в проекцию гостя")
                .doesNotContain(hostHand.get(0));
        assertThat(hostState.toString())
                .withFailMessage("Рука гостя утекла в проекцию хозяина")
                .doesNotContain(guestHand.get(0));

        // У соседа видно только количество карт и уровень навесов.
        for (final JsonNode seat : hostState.get("players")) {
            assertThat(seat.get("cardsCount").asInt()).isEqualTo(6);
            assertThat(seat.has("hand")).isFalse();
        }

        hostSocket.close();
        guestSocket.close();
    }

    @DisplayName("Should refuse the match When not everybody is ready")
    @Test
    void shouldRefuseTheMatchWhenNotEverybodyIsReady() throws Exception {
        final Player host = register("mws-lonely");
        final String tableId = createTable(host);

        final BlockingQueue<String> inbox = new LinkedBlockingQueue<>();
        final WebSocketSession socket = connect(host, inbox);
        send(socket, "TABLE_JOIN", tableId, null);
        Thread.sleep(200);
        send(socket, "MATCH_START", tableId, null);

        assertThat(await(inbox, "ERROR").get("code").asText()).isEqualTo("TABLE_NOT_READY");
        socket.close();
    }

    @DisplayName("Should refuse a move from somebody who is not playing When a stranger sends it")
    @Test
    void shouldRefuseAMoveFromSomebodyWhoIsNotPlayingWhenAStrangerSendsIt() throws Exception {
        final Player host = register("mws-h2");
        final Player guest = register("mws-g2");
        final Player stranger = register("mws-stranger");
        final String tableId = createTable(host);

        final BlockingQueue<String> hostInbox = new LinkedBlockingQueue<>();
        final BlockingQueue<String> guestInbox = new LinkedBlockingQueue<>();
        final BlockingQueue<String> strangerInbox = new LinkedBlockingQueue<>();
        final WebSocketSession hostSocket = connect(host, hostInbox);
        final WebSocketSession guestSocket = connect(guest, guestInbox);
        final WebSocketSession strangerSocket = connect(stranger, strangerInbox);

        send(hostSocket, "TABLE_JOIN", tableId, null);
        send(guestSocket, "TABLE_JOIN", tableId, null);
        send(hostSocket, "TABLE_READY", tableId, Map.of("ready", true));
        send(guestSocket, "TABLE_READY", tableId, Map.of("ready", true));
        Thread.sleep(400);
        send(hostSocket, "MATCH_START", tableId, null);
        await(hostInbox, "STATE_SYNC");

        send(strangerSocket, "PASS", tableId, null);

        assertThat(await(strangerInbox, "ERROR").get("code").asText()).isEqualTo("NOT_A_PLAYER");
        hostSocket.close();
        guestSocket.close();
        strangerSocket.close();
    }

    private static List<String> cards(final JsonNode hand) {
        final List<String> cards = new ArrayList<>();
        hand.forEach(card -> cards.add(card.asText()));
        return cards;
    }

    private JsonNode await(final BlockingQueue<String> inbox, final String type) throws Exception {
        for (int attempt = 0; attempt < 40; attempt++) {
            final String message = inbox.poll(5, TimeUnit.SECONDS);
            if (message == null) {
                break;
            }
            final JsonNode envelope = JSON.readTree(message);
            if (type.equals(envelope.path("type").asText())) {
                return envelope.get("payload");
            }
        }
        throw new AssertionError("Не дождался сообщения " + type);
    }

    private void send(final WebSocketSession socket, final String type, final String tableId,
                      final Map<String, Object> payload) throws Exception {
        final Map<String, Object> envelope = new java.util.LinkedHashMap<>();
        envelope.put("v", 1);
        envelope.put("id", type + "-" + System.nanoTime());
        envelope.put("type", type);
        envelope.put("tableId", tableId);
        if (payload != null) {
            envelope.put("payload", payload);
        }
        socket.sendMessage(new TextMessage(JSON.writeValueAsString(envelope)));
    }

    private WebSocketSession connect(final Player player, final BlockingQueue<String> inbox)
            throws Exception {
        final HttpHeaders headers = new HttpHeaders();
        headers.setBearerAuth(player.accessToken());
        final JsonNode ticket = rest.exchange("/api/auth/ws-ticket", HttpMethod.POST,
                new HttpEntity<>(headers), JsonNode.class).getBody();
        return new StandardWebSocketClient()
                .execute(new TextWebSocketHandler() {
                    @Override
                    protected void handleTextMessage(final WebSocketSession session, final TextMessage message) {
                        inbox.add(message.getPayload());
                    }
                }, new WebSocketHttpHeaders(),
                        URI.create("ws://localhost:" + port + "/ws?ticket=" + ticket.get("ticket").asText())
                ).get(5, TimeUnit.SECONDS);
    }

    private String createTable(final Player host) {
        final HttpHeaders headers = new HttpHeaders();
        headers.setBearerAuth(host.accessToken());
        headers.setContentType(org.springframework.http.MediaType.APPLICATION_JSON);
        final JsonNode table = rest.exchange("/api/tables", HttpMethod.POST,
                new HttpEntity<>(Map.of("name", "Тест", "maxPlayers", 2, "isPrivate", false), headers),
                JsonNode.class).getBody();
        return table.get("id").asText();
    }

    private Player register(final String username) {
        final JsonNode tokens = rest.postForEntity("/api/auth/register",
                Map.of("username", username, "displayName", "Игрок " + username,
                        "password", "very-secret-password", "inviteCode", "bardak-2026"),
                JsonNode.class).getBody();
        return new Player(tokens.get("accessToken").asText());
    }

    private record Player(String accessToken) {
    }
}
