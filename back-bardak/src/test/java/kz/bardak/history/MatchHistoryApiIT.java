package kz.bardak.history;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import kz.bardak.game.rules.DealEvent;
import kz.bardak.game.rules.DealOutcome;
import kz.bardak.game.rules.JokerCard;
import kz.bardak.game.rules.LevelChange;
import kz.bardak.game.rules.LevelChangeReason;
import kz.bardak.game.rules.LossDegree;
import kz.bardak.game.rules.MatchPhase;
import kz.bardak.game.rules.MatchState;
import kz.bardak.game.rules.NavesScale;
import kz.bardak.game.rules.PipCard;
import kz.bardak.game.rules.PlayerOutcome;
import kz.bardak.game.rules.Rank;
import kz.bardak.game.rules.Suit;
import kz.bardak.game.rules.DealStateFixtureAccess;
import kz.bardak.lobby.LobbyService;
import kz.bardak.lobby.domain.CardSetRepository;
import kz.bardak.lobby.domain.TableThemeRepository;
import kz.bardak.rating.MatchResultService;
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
import org.springframework.http.ResponseEntity;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

/**
 * История и рейтинг через REST.
 *
 * <p>⭐ Главная проверка — реплей: он обязан показывать матч <b>глазами спрашивающего</b>.
 * Чужая вскрытая карта не должна всплыть задним числом — иначе история сдаёт то, что
 * правила прятали весь матч (§1.8).
 */
@Tag("integration")
@Testcontainers
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = "bardak.rating.season-admins=season-boss")
class MatchHistoryApiIT {

    @Container
    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = new PostgreSQLContainer<>("postgres:16-alpine");

    @Autowired
    private TestRestTemplate rest;

    @Autowired
    private MatchLog matchLog;

    @Autowired
    private DealHistory dealHistory;

    @Autowired
    private MatchResultService results;

    @Autowired
    private LobbyService lobby;

    @Autowired
    private CardSetRepository cardSets;

    @Autowired
    private TableThemeRepository themes;

    @DisplayName("Should list the finished match with my place When history is asked for")
    @Test
    void shouldListTheFinishedMatchWithMyPlaceWhenHistoryIsAskedFor() {
        final Player host = register("hist-a");
        final Player guest = register("hist-b");
        finishedMatch(host, guest);

        final JsonNode list = get(host, "/api/matches").getBody();

        assertThat(list).hasSize(1);
        assertThat(list.get(0).get("status").asText()).isEqualTo("FINISHED");
        assertThat(list.get(0).get("myPlace").asInt()).isEqualTo(1);
        assertThat(list.get(0).get("ratingCounted").asBoolean()).isTrue();
        assertThat(list.get(0).get("myRatingDelta").asDouble()).isPositive();
        assertThat(list.get(0).get("players")).hasSize(2);
    }

    @DisplayName("Should show the deal breakdown with reasons When a match is opened")
    @Test
    void shouldShowTheDealBreakdownWithReasonsWhenAMatchIsOpened() {
        final Player host = register("hist-c");
        final Player guest = register("hist-d");
        final UUID matchId = finishedMatch(host, guest);

        final JsonNode details = get(host, "/api/matches/" + matchId).getBody();

        assertThat(details.get("deals")).hasSize(1);
        final JsonNode deal = details.get("deals").get(0);
        assertThat(deal.get("trumpSuit").asText()).isEqualTo("SPADES");
        assertThat(deal.get("lastAttackCards").get(0).asText()).isEqualTo("8-hearts");
        assertThat(deal.get("seats").get(1).get("levelChanges").get(0).get("reason").asText())
                .isEqualTo("LOST_DEAL");
    }

    @DisplayName("Should hide the hidden card of the other player When a replay is watched")
    @Test
    void shouldHideTheHiddenCardOfTheOtherPlayerWhenAReplayIsWatched() {
        final Player host = register("hist-e");
        final Player guest = register("hist-f");
        final UUID matchId = finishedMatch(host, guest);
        // Место 1 вскрыло свою скрытую карту: это событие видит только оно (§1.8).
        matchLog.append(matchId, 1, 1, List.of(
                new DealEvent.FaceDownRevealed(1, PipCard.of(Rank.ACE, Suit.CLUBS))));

        final JsonNode mine = get(guest, "/api/matches/" + matchId + "/replay").getBody();
        final JsonNode theirs = get(host, "/api/matches/" + matchId + "/replay").getBody();

        assertThat(mine.get("events")).anySatisfy(event ->
                assertThat(event.get("type").asText()).isEqualTo("FACE_DOWN_REVEALED"));
        assertThat(theirs.get("events")).noneSatisfy(event ->
                assertThat(event.get("type").asText()).isEqualTo("FACE_DOWN_REVEALED"));
    }

    @DisplayName("Should refuse the replay When the match is still going")
    @Test
    void shouldRefuseTheReplayWhenTheMatchIsStillGoing() {
        final Player host = register("hist-g");
        final UUID tableId = createTable(host);
        final UUID matchId = matchLog.startMatch(tableId, 2, 5L, "{}").id();

        final ResponseEntity<JsonNode> response = get(host, "/api/matches/" + matchId + "/replay");

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.CONFLICT);
        assertThat(response.getBody().get("code").asText()).isEqualTo("MATCH_NOT_FINISHED");
    }

    @DisplayName("Should show the starting rating When the player has never played")
    @Test
    void shouldShowTheStartingRatingWhenThePlayerHasNeverPlayed() {
        final Player rookie = register("hist-rookie");

        final JsonNode view = get(rookie, "/api/rating/me").getBody();

        assertThat(view.get("rating").asDouble()).isEqualTo(1000.0);
        assertThat(view.get("matchesPlayed").asInt()).isZero();
        assertThat(view.get("history")).isEmpty();
    }

    @DisplayName("Should refuse to close the season When the player is not an admin")
    @Test
    void shouldRefuseToCloseTheSeasonWhenThePlayerIsNotAnAdmin() {
        final Player player = register("hist-nobody");

        final ResponseEntity<JsonNode> response = rest.exchange("/api/rating/seasons",
                HttpMethod.POST, new HttpEntity<>(Map.of("name", "Второй"), authorized(player)),
                JsonNode.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.FORBIDDEN);
        assertThat(response.getBody().get("code").asText()).isEqualTo("NOT_SEASON_ADMIN");
    }

    @DisplayName("Should open the next season When an admin closes the current one")
    @Test
    void shouldOpenTheNextSeasonWhenAnAdminClosesTheCurrentOne() {
        final Player admin = registerAs("season-boss");

        final JsonNode opened = rest.exchange("/api/rating/seasons", HttpMethod.POST,
                new HttpEntity<>(Map.of("name", "Второй сезон"), authorized(admin)),
                JsonNode.class).getBody();

        assertThat(opened.get("open").asBoolean()).isTrue();
        assertThat(opened.get("startedAt").isNull()).isFalse();
        // ⭐ Открытый сезон обязан быть ровно один: закрытие и открытие — одно действие.
        final JsonNode all = get(admin, "/api/rating/seasons").getBody();
        assertThat(all).hasSize(2);
        assertThat(all).filteredOn(season -> season.get("open").asBoolean()).hasSize(1);
    }

    /** Матч из одной раздачи: место 0 вышло первым, место 1 добралось до джокера. */
    private UUID finishedMatch(final Player host, final Player guest) {
        final UUID tableId = createTable(host);
        final UUID matchId = matchLog.startMatch(tableId, 2, 11L, "{}").id();
        final List<UUID> seats = List.of(host.id(), guest.id());
        final DealOutcome outcome = new DealOutcome(List.of(
                new PlayerOutcome(0, 1, 0, null, 1, List.of(),
                        List.of(new LevelChange(LevelChangeReason.FIRST_OUT, -1))),
                new PlayerOutcome(1, 8, 9, LossDegree.SUPER_MEGA_FAIL, 2, List.of(new JokerCard(1)),
                        List.of(new LevelChange(LevelChangeReason.LOST_DEAL, 1)))),
                1, Suit.SPADES, List.of(PipCard.of(Rank.EIGHT, Suit.HEARTS)));

        results.startMatch(matchId, seats);
        dealHistory.record(matchId, 1, outcome, NavesScale.full());
        results.finishMatch(matchId, seats, new MatchState(MatchPhase.MATCH_OVER, List.of(0, 9), 1,
                11L, DealStateFixtureAccess.finished(), List.of(outcome)), NavesScale.full());
        return matchId;
    }

    private ResponseEntity<JsonNode> get(final Player player, final String path) {
        return rest.exchange(path, HttpMethod.GET, new HttpEntity<>(authorized(player)),
                JsonNode.class);
    }

    private HttpHeaders authorized(final Player player) {
        final HttpHeaders headers = new HttpHeaders();
        headers.setBearerAuth(player.accessToken());
        headers.setContentType(org.springframework.http.MediaType.APPLICATION_JSON);
        return headers;
    }

    private UUID createTable(final Player host) {
        return lobby.create(host.id(), "История", 2,
                cardSets.findByIsDefaultTrue().orElseThrow().id(),
                themes.findByIsDefaultTrue().orElseThrow().id(), "{}", false).id();
    }

    private Player register(final String username) {
        return registerAs(username + "-" + UUID.randomUUID().toString().substring(0, 6));
    }

    private Player registerAs(final String username) {
        final JsonNode tokens = rest.postForEntity("/api/auth/register",
                Map.of("username", username, "displayName", "Игрок",
                        "password", "very-secret-password", "inviteCode", "bardak-2026"),
                JsonNode.class).getBody();
        final JsonNode profile = rest.exchange("/api/profile", HttpMethod.GET,
                new HttpEntity<>(bearer(tokens.get("accessToken").asText())), JsonNode.class).getBody();
        return new Player(UUID.fromString(profile.get("id").asText()),
                tokens.get("accessToken").asText());
    }

    private HttpHeaders bearer(final String token) {
        final HttpHeaders headers = new HttpHeaders();
        headers.setBearerAuth(token);
        return headers;
    }

    private record Player(UUID id, String accessToken) {
    }
}
