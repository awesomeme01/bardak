package kz.bardak.game;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.net.URI;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.TimeUnit;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.http.HttpEntity;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpMethod;
import org.springframework.http.MediaType;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketHttpHeaders;
import org.springframework.web.socket.WebSocketSession;
import org.springframework.web.socket.client.standard.StandardWebSocketClient;
import org.springframework.web.socket.handler.TextWebSocketHandler;

/**
 * Двое за столом на настоящих сокетах: регистрация, стол, готовность, старт матча.
 *
 * <p>Вынесено из тестов затем, что таймеры проверяются двумя наборами настроек — с ходом
 * за молчащего и без него, — а два набора настроек означают два контекста Spring и,
 * значит, два тестовых класса. Копия обвязки в каждом расходилась бы уже назавтра.
 */
final class TableSocketHarness {

    private static final ObjectMapper JSON = new ObjectMapper();

    private final TestRestTemplate rest;
    private final int port;

    TableSocketHarness(final TestRestTemplate rest, final int port) {
        this.rest = rest;
        this.port = port;
    }

    /** Стол на двоих с уже начатым матчем. */
    Table startedMatch(final String hostName, final String guestName) throws Exception {
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

        return new Table(tableId, hostSocket, guestSocket, hostInbox, guestInbox);
    }

    /**
     * Свежий снимок состояния.
     *
     * <p>Нужен отдельным запросом: снимок после старта матча уже разобран в {@code startedMatch},
     * а нового по своей воле сервер не пришлёт — за столом никто не ходит.
     */
    JsonNode currentState(final Table game) throws Exception {
        send(game.hostSocket(), "STATE_REQUEST", game.tableId(), null);
        return await(game.hostInbox(), "STATE_SYNC", 10);
    }

    /** Дождаться сообщения нужного типа. Не дождался — тест падает здесь же. */
    JsonNode await(final BlockingQueue<String> inbox, final String type, final int seconds)
            throws Exception {
        final JsonNode payload = poll(inbox, type, seconds);
        if (payload == null) {
            throw new AssertionError("Не дождался сообщения " + type);
        }
        return payload;
    }

    /** То же, но отсутствие сообщения — законный результат, а не провал. */
    JsonNode poll(final BlockingQueue<String> inbox, final String type, final int seconds)
            throws Exception {
        final long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(seconds);
        while (System.nanoTime() < deadline) {
            final String message = inbox.poll(200, TimeUnit.MILLISECONDS);
            if (message == null) {
                continue;
            }
            final JsonNode envelope = JSON.readTree(message);
            if (type.equals(envelope.path("type").asText())) {
                return envelope.path("payload");
            }
        }
        return null;
    }

    void send(final WebSocketSession socket, final String type, final String tableId,
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
        headers.setContentType(MediaType.APPLICATION_JSON);
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

    record Table(String tableId, WebSocketSession hostSocket, WebSocketSession guestSocket,
                 BlockingQueue<String> hostInbox, BlockingQueue<String> guestInbox) {

        void close() throws Exception {
            hostSocket.close();
            guestSocket.close();
        }
    }
}
