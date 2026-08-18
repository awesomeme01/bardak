package kz.bardak.game;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.UUID;
import kz.bardak.lobby.LobbyService;
import kz.bardak.lobby.domain.TableStatus;
import kz.bardak.TestPostgres;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.boot.test.web.server.LocalServerPort;
import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.springframework.web.socket.CloseStatus;
import org.testcontainers.containers.PostgreSQLContainer;

/**
 * Таймеры стола (§5.1–5.3). Времена взяты короткими из конфига: правило проверяется
 * то же самое, а тест не ждёт минуту.
 *
 * <p>⭐ Ход за молчащего включён здесь <b>явно</b>: по умолчанию сервер за игрока не ходит,
 * и без этой строки проверять было бы нечего. Обратное поведение — в {@code TurnHoldsIT}.
 */
@Tag("integration")
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = {"bardak.game.turn-timeout=1s", "bardak.game.disconnect-grace=2s",
                "bardak.game.auto-move-on-timeout=true"})
class TimersIT {

    // База общая с TurnHoldsIT — см. SharedPostgres.
    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

    @Autowired
    private TestRestTemplate rest;

    @Autowired
    private LobbyService lobby;

    @LocalServerPort
    private int port;

    private TableSocketHarness table() {
        return new TableSocketHarness(rest, port);
    }

    @DisplayName("Should act for the silent player When the turn times out")
    @Test
    void shouldActForTheSilentPlayerWhenTheTurnTimesOut() throws Exception {
        final TableSocketHarness harness = table();
        final TableSocketHarness.Table game = harness.startedMatch("tmr-a", "tmr-b");

        // Никто не ходит: сервер обязан сделать самое безобидное действие сам.
        assertThat(harness.await(game.hostInbox(), "TURN_TIMEOUT", 10)).isNotNull();

        game.close();
    }

    @DisplayName("Should pause the match When a player drops out")
    @Test
    void shouldPauseTheMatchWhenAPlayerDropsOut() throws Exception {
        final TableSocketHarness harness = table();
        final TableSocketHarness.Table game = harness.startedMatch("tmr-c", "tmr-d");

        game.guestSocket().close(CloseStatus.NORMAL);

        final JsonNode paused = harness.await(game.hostInbox(), "MATCH_PAUSED", 10);
        assertThat(paused.get("graceSeconds").asInt()).isEqualTo(2);
        game.hostSocket().close();
    }

    @DisplayName("Should abort the match When the player never comes back")
    @Test
    void shouldAbortTheMatchWhenThePlayerNeverComesBack() throws Exception {
        final TableSocketHarness harness = table();
        final TableSocketHarness.Table game = harness.startedMatch("tmr-e", "tmr-f");

        game.guestSocket().close(CloseStatus.NORMAL);
        harness.await(game.hostInbox(), "MATCH_PAUSED", 10);

        assertThat(harness.await(game.hostInbox(), "MATCH_ABORTED", 20)).isNotNull();

        // ⚠️ Регрессия: отменённый матч оставлял стол в IN_MATCH навсегда — сесть за него
        // было уже нельзя, а лобби до конца дней показывало «матч идёт».
        assertThat(lobby.byId(UUID.fromString(game.tableId())).status())
                .as("после отмены стол снова ждёт игроков")
                .isEqualTo(TableStatus.WAITING);
        game.hostSocket().close();
    }
}
