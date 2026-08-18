package kz.bardak.history;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.util.List;
import java.util.UUID;
import kz.bardak.TestPostgres;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.game.rules.DealOutcome;
import kz.bardak.game.rules.JokerCard;
import kz.bardak.game.rules.LevelChange;
import kz.bardak.game.rules.LevelChangeReason;
import kz.bardak.game.rules.LossDegree;
import kz.bardak.game.rules.NavesScale;
import kz.bardak.game.rules.PipCard;
import kz.bardak.game.rules.PlayerOutcome;
import kz.bardak.game.rules.Rank;
import kz.bardak.game.rules.Suit;
import kz.bardak.history.domain.DealRecord;
import kz.bardak.history.domain.DealRepository;
import kz.bardak.history.domain.DealResultRepository;
import kz.bardak.lobby.LobbyService;
import kz.bardak.lobby.domain.CardSetRepository;
import kz.bardak.lobby.domain.TableThemeRepository;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.testcontainers.containers.PostgreSQLContainer;

/**
 * Запись сыгранной раздачи на настоящем Postgres.
 *
 * <p>⭐ Проверяется именно то, чего нет в логе событий: почему уровень сдвинулся. Разница
 * уровней сама по себе объяснением не является — {@code +1} и {@code −1} в одной раздаче
 * выглядят как отсутствие сдвига.
 */
@Tag("integration")
@SpringBootTest
class DealHistoryIT {

    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

    @Autowired
    private DealHistory dealHistory;

    @Autowired
    private ObjectMapper objectMapper;

    @Autowired
    private MatchLog matchLog;

    @Autowired
    private DealRepository deals;

    @Autowired
    private DealResultRepository dealResults;

    @Autowired
    private LobbyService lobby;

    @Autowired
    private UserRepository users;

    @Autowired
    private CardSetRepository cardSets;

    @Autowired
    private TableThemeRepository themes;

    @DisplayName("Should write the deal with its reasons When a deal is scored")
    @Test
    void shouldWriteTheDealWithItsReasonsWhenADealIsScored() {
        final UUID matchId = newMatch();

        dealHistory.record(matchId, 1, outcome(), NavesScale.full());

        final List<DealRecord> written = deals.findByMatchIdOrderByDealNo(matchId);
        assertThat(written).singleElement().satisfies(deal -> {
            assertThat(deal.dealNo()).isEqualTo(1);
            assertThat(deal.trumpSuit()).isEqualTo("SPADES");
            assertThat(deal.loserSeat()).isEqualTo(1);
            assertThat(deal.lastAttackCards()).isEqualTo("[\"8-hearts\"]");
        });

        assertThat(dealResults.findByDealIdOrderBySeatNo(written.get(0).id())).satisfiesExactly(
                first -> {
                    assertThat(first.place()).isEqualTo(1);
                    assertThat(first.navesLevelBefore()).isEqualTo("7");
                    assertThat(first.navesLevelAfter()).isEqualTo("6");
                    // jsonb хранит объект, а не текст: порядок ключей и пробелы свои.
                    assertThat(json(first.levelChanges()))
                            .isEqualTo(json("[{\"reason\":\"FIRST_OUT\",\"amount\":-1}]"));
                },
                loser -> {
                    assertThat(loser.place()).isEqualTo(2);
                    assertThat(loser.navesLevelAfter()).isEqualTo("Jk");
                    assertThat(loser.hungCards()).isEqualTo("[\"Joker-1\"]");
                    assertThat(json(loser.levelChanges()))
                            .isEqualTo(json("[{\"reason\":\"LOST_DEAL\",\"amount\":1}]"));
                });
    }

    @DisplayName("Should keep a single row When the same deal is recorded twice")
    @Test
    void shouldKeepASingleRowWhenTheSameDealIsRecordedTwice() {
        final UUID matchId = newMatch();
        dealHistory.record(matchId, 1, outcome(), NavesScale.full());

        dealHistory.record(matchId, 1, outcome(), NavesScale.full());

        assertThat(deals.findByMatchIdOrderByDealNo(matchId)).hasSize(1);
    }

    @DisplayName("Should write no level before When nothing was hung on the player yet")
    @Test
    void shouldWriteNoLevelBeforeWhenNothingWasHungOnThePlayerYet() {
        final UUID matchId = newMatch();
        final DealOutcome fresh = new DealOutcome(List.of(
                new PlayerOutcome(0, NavesScale.NO_NAVES, NavesScale.NO_NAVES, null, 1, List.of(),
                        List.of()),
                new PlayerOutcome(1, NavesScale.NO_NAVES, 0, null, 2, List.of(),
                        List.of(new LevelChange(LevelChangeReason.LOST_DEAL, 1)))),
                1, Suit.CLUBS, List.of());

        dealHistory.record(matchId, 1, fresh, NavesScale.full());

        final UUID dealId = deals.findByMatchIdOrderByDealNo(matchId).get(0).id();
        // «Летит 6» — это отсутствие навесов, а не ступень: в базе оно и есть null.
        assertThat(dealResults.findByDealIdOrderBySeatNo(dealId).get(0).navesLevelBefore()).isNull();
        assertThat(dealResults.findByDealIdOrderBySeatNo(dealId).get(1).navesLevelAfter())
                .isEqualTo("6");
    }

    private JsonNode json(final String raw) {
        try {
            return objectMapper.readTree(raw);
        } catch (final JsonProcessingException e) {
            throw new AssertionError("Не разобрался JSON: " + raw, e);
        }
    }

    /** Место 0 вышло первым, место 1 осталось с картами и добралось до джокера. */
    private DealOutcome outcome() {
        return new DealOutcome(List.of(
                new PlayerOutcome(0, 1, 0, null, 1, List.of(),
                        List.of(new LevelChange(LevelChangeReason.FIRST_OUT, -1))),
                new PlayerOutcome(1, 8, 9, LossDegree.SUPER_MEGA_FAIL, 2,
                        List.of(new JokerCard(1)),
                        List.of(new LevelChange(LevelChangeReason.LOST_DEAL, 1)))),
                1, Suit.SPADES, List.of(PipCard.of(Rank.EIGHT, Suit.HEARTS)));
    }

    private UUID newMatch() {
        final UUID host = users.save(new User(UUID.randomUUID(),
                "dh-" + UUID.randomUUID().toString().substring(0, 8), "Хозяин", null, "hash")).id();
        final UUID tableId = lobby.create(host, "Раздачи", 2,
                cardSets.findByIsDefaultTrue().orElseThrow().id(),
                themes.findByIsDefaultTrue().orElseThrow().id(), "{}", false).id();
        return matchLog.startMatch(tableId, 2, 7L, "{}").id();
    }
}
