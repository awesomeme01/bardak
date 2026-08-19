package kz.bardak.game;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
import kz.bardak.TestPostgres;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.boot.test.web.server.LocalServerPort;
import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.testcontainers.containers.PostgreSQLContainer;

/**
 * Устойчивость протокола к тому, что клиент прислал не то.
 *
 * <p>⚠️ Проверки появились после разбора: кривой идентификатор стола РВАЛ соединение,
 * а отклонённая команда запоминалась как применённая. И то и другое клиент видел как
 * «непонятно что произошло», а не как ошибку.
 */
@Tag("integration")
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class ProtocolRobustnessIT {

    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

    @Autowired
    private TestRestTemplate rest;

    @LocalServerPort
    private int port;

    @DisplayName("Should answer with an error and keep the socket When the table id is malformed")
    @Test
    void shouldAnswerWithAnErrorAndKeepTheSocketWhenTheTableIdIsMalformed() throws Exception {
        final TableSocketHarness harness = new TableSocketHarness(rest, port);
        final TableSocketHarness.Table game = harness.startedMatch("robust-host", "robust-guest");
        try {
            harness.send(game.hostSocket(), "STATE_REQUEST", "это-не-uuid", null);

            final JsonNode error = harness.await(game.hostInbox(), "ERROR", 5);
            assertThat(error.get("code").asText()).isEqualTo("TABLE_ID_INVALID");

            // ⭐ Главное: соединение живо. Раньше Spring закрывал его с SERVER_ERROR,
            // и клиент получал обрыв вместо ошибки.
            assertThat(game.hostSocket().isOpen()).isTrue();

            // И сокет продолжает работать: следующая команда доходит.
            assertThat(harness.currentState(game).get("phase")).isNotNull();
        } finally {
            game.close();
        }
    }

    @DisplayName("Should report the reason again When a rejected command is repeated")
    @Test
    void shouldReportTheReasonAgainWhenARejectedCommandIsRepeated() throws Exception {
        final TableSocketHarness harness = new TableSocketHarness(rest, port);
        final TableSocketHarness.Table game = harness.startedMatch("repeat-host", "repeat-guest");
        try {
            // ⭐ Отказ должен прийти от ДВИЖКА, а не от разбора команды: путь идемпотентности
            // лежит после разбора, и невалидный код карты его просто не достигает.
            // Поэтому берём защищающегося и заставляем его атаковать — это NOT_YOUR_TURN.
            final JsonNode state = harness.currentState(game);
            final int mySeat = state.get("mySeat").asInt();
            final int defenderSeat = state.get("defenderSeat").asInt();

            final org.springframework.web.socket.WebSocketSession socket =
                    mySeat == defenderSeat ? game.hostSocket() : game.guestSocket();
            final java.util.concurrent.BlockingQueue<String> inbox =
                    mySeat == defenderSeat ? game.hostInbox() : game.guestInbox();

            final JsonNode hand = mySeat == defenderSeat
                    ? state.get("myHand")
                    : otherHand(harness, game);
            final String cardCode = hand.get(0).asText();

            final String commandId = "same-command-id";
            attack(socket, game.tableId(), commandId, cardCode);
            final JsonNode first = harness.await(inbox, "ERROR", 5);
            final String firstCode = first.get("code").asText();
            assertThat(firstCode)
                    .as("нужен отказ движка, а не ошибка разбора")
                    .isNotEqualTo("BAD_COMMAND");

            // ⚠️ Тот же id ещё раз: клиент повторяет команду сам после обрыва (ADR-052).
            attack(socket, game.tableId(), commandId, cardCode);

            final JsonNode second = harness.await(inbox, "ERROR", 5);
            assertThat(second.get("code").asText())
                    .as("повтор отклонённой команды обязан вернуть причину, а не снимок состояния")
                    .isEqualTo(firstCode);
        } finally {
            game.close();
        }
    }

    /** Рука второго игрока: у защищающегося она своя. */
    private JsonNode otherHand(final TableSocketHarness harness,
                               final TableSocketHarness.Table game) throws Exception {
        harness.send(game.guestSocket(), "STATE_REQUEST", game.tableId(), null);
        return harness.await(game.guestInbox(), "STATE_SYNC", 10).get("myHand");
    }

    /** Атака с заданным id команды: идемпотентность считается именно по нему. */
    private void attack(final org.springframework.web.socket.WebSocketSession socket,
                        final String tableId, final String commandId, final String cardCode)
            throws Exception {
        final String envelope = """
                {"v":1,"id":"%s","type":"PLAY_CARD","tableId":"%s","payload":{"cardCode":"%s"}}"""
                .formatted(commandId, tableId, cardCode);
        socket.sendMessage(new org.springframework.web.socket.TextMessage(envelope));
    }
}
