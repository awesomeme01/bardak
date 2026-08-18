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
 * Поведение по умолчанию: ход ждёт своего хозяина.
 *
 * <p>Настройки таймаута специально <b>не трогаются</b> — проверяется ровно то, что получает
 * стол «из коробки». Таймаут хода при этом короткий только в {@code TimersIT}; здесь важно
 * обратное: даже когда время вышло бы, за игрока никто не ходит.
 */
@Tag("integration")
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = {"bardak.game.turn-timeout=1s"})
class TurnHoldsIT {

    // База общая с TimersIT: этому классу нужен только другой набор настроек, а не своя СУБД.
    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

    @Autowired
    private TestRestTemplate rest;

    @LocalServerPort
    private int port;

    @DisplayName("Should keep the turn with the player When nobody moves and auto-move is off")
    @Test
    void shouldKeepTheTurnWithThePlayerWhenNobodyMovesAndAutoMoveIsOff() throws Exception {
        final TableSocketHarness harness = new TableSocketHarness(rest, port);
        final TableSocketHarness.Table game = harness.startedMatch("hold-a", "hold-b");

        // Таймаут хода — секунда; ждём заведомо дольше и всё равно не должны увидеть ход.
        assertThat(harness.poll(game.hostInbox(), "TURN_TIMEOUT", 4))
                .as("сервер не ходит за молчащего, пока это не включено настройкой")
                .isNull();

        game.close();
    }

    @DisplayName("Should not count down the turn When auto-move is off")
    @Test
    void shouldNotCountDownTheTurnWhenAutoMoveIsOff() throws Exception {
        final TableSocketHarness harness = new TableSocketHarness(rest, port);
        final TableSocketHarness.Table game = harness.startedMatch("hold-c", "hold-d");

        final JsonNode state = harness.currentState(game);

        assertThat(state.hasNonNull("turnSecondsLeft"))
                .as("часов нет — клиенту нечего показывать в обратном отсчёте")
                .isFalse();

        game.close();
    }
}
