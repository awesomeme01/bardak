package kz.bardak.rating;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import kz.bardak.game.rules.DealOutcome;
import kz.bardak.game.rules.DealStateFixtureAccess;
import kz.bardak.game.rules.LossDegree;
import kz.bardak.game.rules.MatchPhase;
import kz.bardak.game.rules.MatchState;
import kz.bardak.game.rules.NavesScale;
import kz.bardak.game.rules.PlayerOutcome;
import kz.bardak.history.MatchLog;
import kz.bardak.lobby.LobbyService;
import kz.bardak.lobby.domain.CardSetRepository;
import kz.bardak.lobby.domain.TableThemeRepository;
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
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

/**
 * Статистика игрока.
 *
 * <p>⭐ Проверяется прежде всего то, что легко посчитать неправильно: серия побед и то,
 * что отменённые матчи в счёт не идут. Ошибка здесь тихая — числа выглядят правдоподобно.
 */
@Tag("integration")
@Testcontainers
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class StatsIT {

    @Container
    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = new PostgreSQLContainer<>("postgres:16-alpine");

    @Autowired
    private TestRestTemplate rest;

    @Autowired
    private MatchLog matchLog;

    @Autowired
    private MatchResultService results;

    @Autowired
    private LobbyService lobby;

    @Autowired
    private CardSetRepository cardSets;

    @Autowired
    private TableThemeRepository themes;

    @DisplayName("Should show nothing When the player has not played yet")
    @Test
    void shouldShowNothingWhenThePlayerHasNotPlayedYet() {
        final Player rookie = register("stat-rookie");

        final JsonNode stats = get(rookie, "/api/stats/me");

        assertThat(stats.get("matches").asInt()).isZero();
        assertThat(stats.get("streak").get("kind").asText()).isEqualTo("NONE");
        assertThat(stats.get("degrees")).isEmpty();
    }

    @DisplayName("Should count the winning streak When the player won twice in a row")
    @Test
    void shouldCountTheWinningStreakWhenThePlayerWonTwiceInARow() {
        final Player winner = register("stat-winner");
        final Player loser = register("stat-loser");
        finishMatch(winner, loser);
        finishMatch(winner, loser);

        final JsonNode stats = get(winner, "/api/stats/me");

        assertThat(stats.get("matches").asInt()).isEqualTo(2);
        assertThat(stats.get("wins").asInt()).isEqualTo(2);
        assertThat(stats.get("streak").get("kind").asText()).isEqualTo("WIN");
        assertThat(stats.get("streak").get("length").asInt()).isEqualTo(2);
        assertThat(stats.get("avgPlace").asDouble()).isEqualTo(1.0);
    }

    @DisplayName("Should count the losing streak and its degrees When the player keeps losing")
    @Test
    void shouldCountTheLosingStreakAndItsDegreesWhenThePlayerKeepsLosing() {
        final Player winner = register("stat-w2");
        final Player loser = register("stat-l2");
        finishMatch(winner, loser);
        finishMatch(winner, loser);

        final JsonNode stats = get(loser, "/api/stats/me");

        assertThat(stats.get("losses").asInt()).isEqualTo(2);
        assertThat(stats.get("streak").get("kind").asText()).isEqualTo("LOSS");
        assertThat(stats.get("degrees").get(0).get("degree").asText()).isEqualTo("SUPER_MEGA_FAIL");
        assertThat(stats.get("degrees").get(0).get("count").asInt()).isEqualTo(2);
    }

    @DisplayName("Should ignore the aborted match When statistics are counted")
    @Test
    void shouldIgnoreTheAbortedMatchWhenStatisticsAreCounted() {
        final Player winner = register("stat-w3");
        final Player loser = register("stat-l3");
        finishMatch(winner, loser);
        // ⭐ Отменённый матч рейтинга не касается (§5.3) — и в статистике его быть не должно.
        final UUID abortedId = startMatch(winner, loser);
        matchLog.abort(abortedId, "Игрок не вернулся");

        assertThat(get(winner, "/api/stats/me").get("matches").asInt()).isEqualTo(1);
    }

    private void finishMatch(final Player winner, final Player loser) {
        final UUID matchId = startMatch(winner, loser);
        final DealOutcome outcome = new DealOutcome(List.of(
                new PlayerOutcome(0, 1, 0, null),
                new PlayerOutcome(1, 8, 9, LossDegree.SUPER_MEGA_FAIL)), 1);
        results.finishMatch(matchId, List.of(winner.id(), loser.id()),
                new MatchState(MatchPhase.MATCH_OVER, List.of(0, 9), 1, 5L,
                        DealStateFixtureAccess.finished(), List.of(outcome)), NavesScale.full());
    }

    private UUID startMatch(final Player first, final Player second) {
        final UUID tableId = lobby.create(first.id(), "Статистика", 2,
                cardSets.findByIsDefaultTrue().orElseThrow().id(),
                themes.findByIsDefaultTrue().orElseThrow().id(), "{}", false).id();
        final UUID matchId = matchLog.startMatch(tableId, 2, 5L, "{}").id();
        results.startMatch(matchId, List.of(first.id(), second.id()));
        return matchId;
    }

    private JsonNode get(final Player player, final String path) {
        final HttpHeaders headers = new HttpHeaders();
        headers.setBearerAuth(player.accessToken());
        return rest.exchange(path, HttpMethod.GET, new HttpEntity<>(headers), JsonNode.class).getBody();
    }

    private Player register(final String username) {
        final String suffix = UUID.randomUUID().toString().substring(0, 6);
        final JsonNode tokens = rest.postForEntity("/api/auth/register",
                Map.of("username", username + "-" + suffix, "displayName", "Игрок",
                        "password", "very-secret-password", "inviteCode", "bardak-2026"),
                JsonNode.class).getBody();
        return new Player(UUID.fromString(tokens.get("user").get("id").asText()),
                tokens.get("accessToken").asText());
    }

    private record Player(UUID id, String accessToken) {
    }
}
