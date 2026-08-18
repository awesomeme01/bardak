package kz.bardak.rating;

import static org.assertj.core.api.Assertions.assertThat;

import java.math.BigDecimal;
import java.util.List;
import java.util.UUID;
import kz.bardak.TestPostgres;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.game.rules.DealOutcome;
import kz.bardak.game.rules.DealStateFixtureAccess;
import kz.bardak.game.rules.LossDegree;
import kz.bardak.game.rules.MatchPhase;
import kz.bardak.game.rules.MatchState;
import kz.bardak.game.rules.NavesScale;
import kz.bardak.game.rules.PlayerOutcome;
import kz.bardak.history.MatchLog;
import kz.bardak.history.domain.MatchPlayerRecord;
import kz.bardak.history.domain.MatchPlayerRepository;
import kz.bardak.history.domain.MatchRecordRepository;
import kz.bardak.history.domain.MatchRecordStatus;
import kz.bardak.lobby.LobbyService;
import kz.bardak.lobby.domain.CardSetRepository;
import kz.bardak.lobby.domain.GameTable;
import kz.bardak.lobby.domain.TableThemeRepository;
import kz.bardak.rating.domain.RatingHistoryRepository;
import kz.bardak.rating.domain.SeasonRepository;
import kz.bardak.rating.domain.UserRatingRepository;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.testcontainers.containers.PostgreSQLContainer;

/**
 * Итог матча на настоящем Postgres: места, уровни, рейтинг.
 *
 * <p>⭐ Главное здесь — что результат и рейтинг ложатся вместе и ровно один раз. Матч,
 * посчитанный дважды, раздул бы рейтинг, а починить это потом можно только руками.
 */
@Tag("integration")
@SpringBootTest
class MatchResultIT {

    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

    @Autowired
    private MatchResultService results;

    @Autowired
    private MatchLog matchLog;

    @Autowired
    private MatchRecordRepository matches;

    @Autowired
    private MatchPlayerRepository matchPlayers;

    @Autowired
    private UserRatingRepository ratings;

    @Autowired
    private RatingHistoryRepository histories;

    @Autowired
    private LobbyService lobby;

    @Autowired
    private UserRepository users;

    @Autowired
    private CardSetRepository cardSets;

    @Autowired
    private TableThemeRepository themes;

    @DisplayName("Should write result and rating together When a match is over")
    @Test
    void shouldWriteResultAndRatingTogetherWhenAMatchIsOver() {
        final List<UUID> seats = List.of(newUser("res-a"), newUser("res-b"));
        final UUID matchId = startMatch(seats);
        // Место 0 закончил на «десятке», место 1 получил джокер картой.
        final MatchState state = matchOver(List.of(
                new PlayerOutcome(0, 3, 4, null),
                new PlayerOutcome(1, 8, 9, LossDegree.SUPER_MEGA_FAIL)));

        final List<MatchResultService.RatingChange> changes =
                results.finishMatch(matchId, seats, state, NavesScale.full());

        assertThat(changes).extracting(MatchResultService.RatingChange::place)
                .containsExactly(1, 2);
        assertThat(changes.get(0).delta()).isPositive();
        assertThat(changes.get(1).delta()).isNegative();

        assertThat(matchPlayers.findByMatchIdOrderBySeatNo(matchId))
                .satisfiesExactly(
                        winner -> {
                            assertThat(winner.place()).isEqualTo(1);
                            assertThat(winner.navesLevel()).isEqualTo("10");
                            assertThat(winner.lossType()).isNull();
                            assertThat(winner.ratingDelta()).isPositive();
                        },
                        loser -> {
                            assertThat(loser.place()).isEqualTo(2);
                            assertThat(loser.navesLevel()).isEqualTo("Jk");
                            assertThat(loser.lossType()).isEqualTo(LossDegree.SUPER_MEGA_FAIL);
                        });

        assertThat(matches.findById(matchId)).get().satisfies(record -> {
            assertThat(record.status()).isEqualTo(MatchRecordStatus.FINISHED);
        });
        assertThat(histories.findByUserIdOrderByCreatedAtDesc(seats.get(1))).singleElement()
                .satisfies(entry -> assertThat(entry.ratingAfter())
                        .isLessThan(entry.ratingBefore()));
        // Сезон открыт один, и матч обязан в него попасть.
        assertThat(ratings.findById(seats.get(0))).get()
                .satisfies(rating -> assertThat(rating.matchesPlayed()).isEqualTo(1));
    }

    @DisplayName("Should leave the rating alone When the same match is finished twice")
    @Test
    void shouldLeaveTheRatingAloneWhenTheSameMatchIsFinishedTwice() {
        final List<UUID> seats = List.of(newUser("twice-a"), newUser("twice-b"));
        final UUID matchId = startMatch(seats);
        final MatchState state = matchOver(List.of(
                new PlayerOutcome(0, 2, 2, null),
                new PlayerOutcome(1, 8, 9, LossDegree.FAIL)));
        results.finishMatch(matchId, seats, state, NavesScale.full());
        final BigDecimal afterFirst = ratings.findById(seats.get(0)).orElseThrow().rating();

        final List<MatchResultService.RatingChange> repeat =
                results.finishMatch(matchId, seats, state, NavesScale.full());

        assertThat(repeat).isEmpty();
        assertThat(ratings.findById(seats.get(0)).orElseThrow().rating()).isEqualTo(afterFirst);
        assertThat(histories.findByUserIdOrderByCreatedAtDesc(seats.get(0))).hasSize(1);
    }

    @DisplayName("Should put the heavier loss last When two players finish with the joker")
    @Test
    void shouldPutTheHeavierLossLastWhenTwoPlayersFinishWithTheJoker() {
        final List<UUID> seats = List.of(newUser("deg-a"), newUser("deg-b"), newUser("deg-c"));
        final UUID matchId = startMatch(seats);
        final MatchState state = matchOver(List.of(
                new PlayerOutcome(0, 8, 9, LossDegree.ROYAL),
                new PlayerOutcome(1, 8, 9, LossDegree.FAIL),
                new PlayerOutcome(2, 4, 5, null)));

        final List<MatchResultService.RatingChange> changes =
                results.finishMatch(matchId, seats, state, NavesScale.full());

        // Не проигравший — первый; из двоих с джокером ROYAL тяжелее и уходит в конец.
        assertThat(changes).extracting(MatchResultService.RatingChange::place)
                .containsExactly(3, 2, 1);
        assertThat(matches.findById(matchId).orElseThrow().status())
                .isEqualTo(MatchRecordStatus.FINISHED);
    }

    @DisplayName("Should keep the rating untouched When a match is aborted")
    @Test
    void shouldKeepTheRatingUntouchedWhenAMatchIsAborted() {
        final List<UUID> seats = List.of(newUser("ab-a"), newUser("ab-b"));
        final UUID matchId = startMatch(seats);

        matchLog.abort(matchId, "Игрок не вернулся");

        assertThat(matches.findById(matchId).orElseThrow().status())
                .isEqualTo(MatchRecordStatus.ABORTED);
        assertThat(histories.existsByMatchId(matchId)).isFalse();
        assertThat(ratings.findById(seats.get(0))).isEmpty();
        assertThat(matchPlayers.findByMatchIdOrderBySeatNo(matchId))
                .extracting(MatchPlayerRecord::place)
                .containsOnlyNulls();
    }

    private MatchState matchOver(final List<PlayerOutcome> players) {
        final List<Integer> levels = players.stream().map(PlayerOutcome::levelAfter).toList();
        return new MatchState(MatchPhase.MATCH_OVER, levels, 1, 42L,
                DealStateFixtureAccess.finished(), List.of(new DealOutcome(players, lastLoser(players))));
    }

    private int lastLoser(final List<PlayerOutcome> players) {
        return players.stream().filter(PlayerOutcome::isLoser).findFirst()
                .map(PlayerOutcome::seatNo).orElse(0);
    }

    private UUID startMatch(final List<UUID> seats) {
        final GameTable table = lobby.create(seats.get(0), "Итог", 5,
                cardSets.findByIsDefaultTrue().orElseThrow().id(),
                themes.findByIsDefaultTrue().orElseThrow().id(), "{}", false);
        final UUID matchId = matchLog.startMatch(table.id(), seats.size(), 42L, "{}").id();
        results.startMatch(matchId, seats);
        return matchId;
    }

    private UUID newUser(final String username) {
        final String suffix = UUID.randomUUID().toString().substring(0, 8);
        return users.save(new User(UUID.randomUUID(), username + "-" + suffix,
                "Игрок " + username, null, "hash")).id();
    }
}
