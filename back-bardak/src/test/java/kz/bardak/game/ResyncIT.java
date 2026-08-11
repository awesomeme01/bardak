package kz.bardak.game;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.net.URI;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
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
 * Догон после разрыва и идемпотентность команд.
 *
 * <p>Таймер хода поднят до минуты: тест про переподключение, а не про автодействие.
 */
@Tag("integration")
@Testcontainers
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = {"bardak.game.turn-timeout=60s", "bardak.game.disconnect-grace=60s"})
class ResyncIT {

    private static final ObjectMapper JSON = new ObjectMapper();

    /** Пересдача после кости может снова начаться с кости — но не бесконечно. */
    private static final int DICE_ATTEMPTS = 5;

    @Container
    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = new PostgreSQLContainer<>("postgres:16-alpine");

    @Autowired
    private TestRestTemplate rest;

    @LocalServerPort
    private int port;

    @DisplayName("Should apply the move once When the same command is sent twice")
    @Test
    void shouldApplyTheMoveOnceWhenTheSameCommandIsSentTwice() throws Exception {
        final Match match = startedMatch("rsy-a", "rsy-b");
        final JsonNode state = match.attackerState();
        final String card = firstAttackCard(state);
        final String commandId = "duplicate-1";

        sendWithId(match.attackerSocket(), commandId, "PLAY_CARD", match.tableId(),
                Map.of("cardCode", card));
        awaitState(match.attackerInbox());
        sendWithId(match.attackerSocket(), commandId, "PLAY_CARD", match.tableId(),
                Map.of("cardCode", card));
        final JsonNode afterRepeat = awaitState(match.attackerInbox());

        // ⭐ Карта на столе одна: повтор не применился, но состояние клиенту всё равно
        // вернулось — иначе он остался бы в неведении, дошла команда или нет.
        assertThat(afterRepeat.get("table")).hasSize(1);
        assertThat(cards(afterRepeat.get("myHand"))).doesNotContain(card);
        match.close();
    }

    @DisplayName("Should send the missed events When a client asks to catch up")
    @Test
    void shouldSendTheMissedEventsWhenAClientAsksToCatchUp() throws Exception {
        final Match match = startedMatch("rsy-c", "rsy-d");
        final JsonNode state = match.attackerState();

        // Наблюдатель уходит и пропускает ход.
        match.otherSocket().close(CloseStatus.NORMAL);
        Thread.sleep(200);
        sendWithId(match.attackerSocket(), "move-1", "PLAY_CARD", match.tableId(),
                Map.of("cardCode", firstAttackCard(state)));
        awaitState(match.attackerInbox());

        final BlockingQueue<String> back = new LinkedBlockingQueue<>();
        final WebSocketSession returned = connect(match.otherToken(), back);
        sendWithId(returned, "resync-1", "RESYNC", match.tableId(), Map.of("lastSeq", 0));

        final List<JsonNode> caughtUp = drain(back, 8);
        assertThat(caughtUp).anySatisfy(message ->
                assertThat(message.path("type").asText()).isEqualTo("CARD_ATTACKED"));
        assertThat(caughtUp).anySatisfy(message -> {
            assertThat(message.path("type").asText()).isEqualTo("STATE_SYNC");
            assertThat(message.path("payload").get("myHand")).hasSize(6);
        });
        // Событие несёт номер: по нему клиент понимает, что догнал.
        assertThat(caughtUp.stream().filter(m -> m.path("type").asText().equals("CARD_ATTACKED"))
                .findFirst().orElseThrow().path("seq").asInt()).isPositive();

        returned.close();
        match.close();
    }

    @DisplayName("Should never replay somebody else's hidden card When catching up")
    @Test
    void shouldNeverReplaySomebodyElsesHiddenCardWhenCatchingUp() throws Exception {
        final Match match = startedMatch("rsy-e", "rsy-f");

        final BlockingQueue<String> back = new LinkedBlockingQueue<>();
        final WebSocketSession returned = connect(match.otherToken(), back);
        sendWithId(returned, "resync-2", "RESYNC", match.tableId(), Map.of("lastSeq", 0));

        final List<JsonNode> caughtUp = drain(back, 8);
        final List<String> hostHand = cards(match.attackerState().get("myHand"));
        for (final JsonNode message : caughtUp) {
            assertThat(message.toString())
                    .withFailMessage("Догон отдал чужую карту: %s", message)
                    .doesNotContain(hostHand.get(0));
        }
        returned.close();
        match.close();
    }

    @DisplayName("Should keep a rejected attempt to its author When somebody else catches up")
    @Test
    void shouldKeepARejectedAttemptToItsAuthorWhenSomebodyElseCatchesUp() throws Exception {
        final Match match = startedMatch("rsy-g", "rsy-h");

        // Ходит не он: попытка будет отклонена и попадёт в лог как приватная (§2.1).
        sendWithId(match.otherSocket(), "wrong-1", "PASS", match.tableId(), null);
        Thread.sleep(400);

        // ⭐ Автор попытки видит её при догоне...
        final BlockingQueue<String> ownInbox = new LinkedBlockingQueue<>();
        final WebSocketSession own = connect(match.otherToken(), ownInbox);
        sendWithId(own, "resync-own", "RESYNC", match.tableId(), Map.of("lastSeq", 0));
        assertThat(drain(ownInbox, 8)).anySatisfy(message ->
                assertThat(message.path("type").asText()).isEqualTo("MOVE_REJECTED"));

        // ...а сосед — нет: наружу уходит только факт и рубашка, но не через чужой догон.
        sendWithId(match.attackerSocket(), "resync-other", "RESYNC", match.tableId(),
                Map.of("lastSeq", 0));
        assertThat(drain(match.attackerInbox(), 8)).noneSatisfy(message ->
                assertThat(message.path("type").asText()).isEqualTo("MOVE_REJECTED"));

        own.close();
        match.close();
    }

    private static List<String> cards(final JsonNode hand) {
        final List<String> cards = new ArrayList<>();
        hand.forEach(card -> cards.add(card.asText()));
        return cards;
    }

    /** Карта из руки атакующего: первая, которой он вправе пойти. */
    private static String firstAttackCard(final JsonNode state) {
        for (final JsonNode action : state.get("availableActions")) {
            if ("PLAY_CARD".equals(action.get("type").asText())) {
                return action.get("payload").get("cardCode").asText();
            }
        }
        throw new AssertionError("У атакующего нет ни одного разрешённого хода");
    }

    /**
     * Всё, что пришло клиенту, но не больше {@code expected}.
     *
     * <p>Сколько именно сообщений придёт, тест знать не может: раздача могла начаться
     * с кости, и тогда перед ходом идёт ещё и выбор козыря. Поэтому ждём не количество,
     * а тишину: пауза без сообщений означает, что сервер всё сказал.
     */
    private List<JsonNode> drain(final BlockingQueue<String> inbox, final int expected) throws Exception {
        final List<JsonNode> messages = new ArrayList<>();
        final long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(10);
        while (messages.size() < expected && System.nanoTime() < deadline) {
            final String raw = inbox.poll(1500, TimeUnit.MILLISECONDS);
            if (raw == null) {
                break;
            }
            messages.add(JSON.readTree(raw));
        }
        return messages;
    }

    private JsonNode awaitState(final BlockingQueue<String> inbox) throws Exception {
        final long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(10);
        while (System.nanoTime() < deadline) {
            final String raw = inbox.poll(1, TimeUnit.SECONDS);
            if (raw == null) {
                continue;
            }
            final JsonNode envelope = JSON.readTree(raw);
            if ("STATE_SYNC".equals(envelope.path("type").asText())) {
                return envelope.get("payload");
            }
        }
        throw new AssertionError("Не дождался STATE_SYNC");
    }

    private Match startedMatch(final String hostName, final String guestName) throws Exception {
        final String hostToken = register(hostName);
        final String guestToken = register(guestName);
        final String tableId = createTable(hostToken);

        final BlockingQueue<String> hostInbox = new LinkedBlockingQueue<>();
        final BlockingQueue<String> guestInbox = new LinkedBlockingQueue<>();
        final WebSocketSession hostSocket = connect(hostToken, hostInbox);
        final WebSocketSession guestSocket = connect(guestToken, guestInbox);

        sendWithId(hostSocket, "j1", "TABLE_JOIN", tableId, null);
        sendWithId(guestSocket, "j2", "TABLE_JOIN", tableId, null);
        sendWithId(hostSocket, "r1", "TABLE_READY", tableId, Map.of("ready", true));
        sendWithId(guestSocket, "r2", "TABLE_READY", tableId, Map.of("ready", true));
        Thread.sleep(400);
        sendWithId(hostSocket, "start", "MATCH_START", tableId, null);

        Match match = whoAttacks(tableId, hostSocket, hostInbox, hostToken, guestSocket,
                guestInbox, guestToken, 0);

        // ⭐ Раздача может начаться с кости: нижней картой колоды выпал джокер, и масть
        // козыря разыгрывается (§1.2). До её выбора разрешённых ходов нет вообще, а после
        // выбора первый ход достаётся тому, у кого младший козырь, — то есть, возможно,
        // другому игроку. Сдача случайна, поэтому тест обязан пройти этот шаг сам: иначе
        // он падает примерно раз в восемь прогонов и выглядит как поломка ресинка.
        for (int attempt = 0; attempt < DICE_ATTEMPTS
                && "DICE".equals(match.attackerState().path("phase").asText()); attempt++) {
            chooseTrump(match, attempt);
            match = whoAttacks(tableId, hostSocket, hostInbox, hostToken, guestSocket,
                    guestInbox, guestToken, attempt + 1);
        }
        return match;
    }

    /** Масть по кости названа не наугад: берётся то, что предложил сервер. */
    private void chooseTrump(final Match match, final int attempt) throws Exception {
        for (final JsonNode action : match.attackerState().get("availableActions")) {
            if ("CHOOSE_TRUMP".equals(action.get("type").asText())) {
                sendWithId(match.attackerSocket(), "dice-" + attempt, "CHOOSE_TRUMP",
                        match.tableId(), Map.of("suit", action.get("payload").get("suit").asText()));
                return;
            }
        }
        throw new AssertionError("Козырь разыгрывается костью, но выбрать масть некому");
    }

    /** Перечитать оба состояния и собрать стол вокруг того, кто ходит. */
    private Match whoAttacks(final String tableId, final WebSocketSession hostSocket,
                             final BlockingQueue<String> hostInbox,
                             final String hostToken, final WebSocketSession guestSocket,
                             final BlockingQueue<String> guestInbox, final String guestToken,
                             final int round) throws Exception {
        sendWithId(hostSocket, "st-h-" + round, "STATE_REQUEST", tableId, null);
        sendWithId(guestSocket, "st-g-" + round, "STATE_REQUEST", tableId, null);
        final JsonNode hostState = latestState(hostInbox);
        final JsonNode guestState = latestState(guestInbox);

        return hostState.get("canAttackSeat").asInt() == hostState.get("mySeat").asInt()
                ? new Match(tableId, hostSocket, hostInbox, hostState, guestSocket, guestToken)
                : new Match(tableId, guestSocket, guestInbox, guestState, hostSocket, hostToken);
    }

    /**
     * ⭐ Самое свежее состояние, а очередь при этом опустошается.
     *
     * <p>Снимок приходит после каждого применённого хода, и на подготовке их накапливается
     * несколько. Оставить лишние в очереди — значит, что первый же {@code awaitState}
     * в самом тесте вернёт вчерашний снимок, и тест будет проверять не то, что проверяет.
     */
    private JsonNode latestState(final BlockingQueue<String> inbox) throws Exception {
        JsonNode latest = awaitState(inbox);
        for (String raw = inbox.poll(300, TimeUnit.MILLISECONDS); raw != null;
             raw = inbox.poll(300, TimeUnit.MILLISECONDS)) {
            final JsonNode envelope = JSON.readTree(raw);
            if ("STATE_SYNC".equals(envelope.path("type").asText())) {
                latest = envelope.get("payload");
            }
        }
        return latest;
    }

    private void sendWithId(final WebSocketSession socket, final String id, final String type,
                            final String tableId, final Map<String, Object> payload) throws Exception {
        final Map<String, Object> envelope = new LinkedHashMap<>();
        envelope.put("v", 1);
        envelope.put("id", id);
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
                new HttpEntity<>(Map.of("name", "Ресинк", "maxPlayers", 2, "isPrivate", false), headers),
                JsonNode.class).getBody().get("id").asText();
    }

    private String register(final String username) {
        return rest.postForEntity("/api/auth/register",
                Map.of("username", username, "displayName", "Игрок", "password", "very-secret-password",
                        "inviteCode", "bardak-2026"), JsonNode.class)
                .getBody().get("accessToken").asText();
    }

    private record Match(String tableId, WebSocketSession attackerSocket,
                         BlockingQueue<String> attackerInbox, JsonNode attackerState,
                         WebSocketSession otherSocket, String otherToken) {

        void close() throws Exception {
            attackerSocket.close();
            if (otherSocket.isOpen()) {
                otherSocket.close();
            }
        }
    }
}
