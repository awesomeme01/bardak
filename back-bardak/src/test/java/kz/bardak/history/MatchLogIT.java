package kz.bardak.history;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.util.List;
import java.util.UUID;
import kz.bardak.TestPostgres;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.game.rules.DealEvent;
import kz.bardak.game.rules.PipCard;
import kz.bardak.game.rules.Rank;
import kz.bardak.game.rules.Suit;
import kz.bardak.history.domain.MatchEventRecord;
import kz.bardak.history.domain.MatchEventRepository;
import kz.bardak.history.domain.MatchRecord;
import kz.bardak.history.domain.MatchRecordRepository;
import kz.bardak.history.domain.MatchRecordStatus;
import kz.bardak.lobby.LobbyService;
import kz.bardak.lobby.domain.CardSetRepository;
import kz.bardak.lobby.domain.TableThemeRepository;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.springframework.dao.DataIntegrityViolationException;
import org.testcontainers.containers.PostgreSQLContainer;

/**
 * Лог матча (ADR-004): только вставка, сквозной номер, никаких дыр и дублей.
 */
@Tag("integration")
@SpringBootTest
class MatchLogIT {

    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

    @Autowired
    private MatchLog matchLog;

    @Autowired
    private MatchEventRepository events;

    @Autowired
    private MatchRecordRepository matches;

    @Autowired
    private LobbyService lobby;

    @Autowired
    private UserRepository users;

    @Autowired
    private CardSetRepository cardSets;

    @Autowired
    private TableThemeRepository themes;

    @DisplayName("Should number the events straight through the match When they are appended")
    @Test
    void shouldNumberTheEventsStraightThroughTheMatchWhenTheyAreAppended() {
        final MatchRecord match = startMatch();

        final int last = matchLog.append(match.id(), 1, 1, List.of(
                new DealEvent.CardAttacked(0, PipCard.of(Rank.SIX, Suit.CLUBS)),
                new DealEvent.CardDefended(1, PipCard.of(Rank.NINE, Suit.CLUBS),
                        PipCard.of(Rank.SIX, Suit.CLUBS))));
        matchLog.append(match.id(), last + 1, 1, List.of(new DealEvent.Passed(0)));

        assertThat(events.findByMatchIdOrderBySeqAsc(match.id()))
                .extracting(MatchEventRecord::seq)
                .containsExactly(1, 2, 3);
        assertThat(events.findByMatchIdOrderBySeqAsc(match.id()))
                .extracting(MatchEventRecord::type)
                .containsExactly("CARD_ATTACKED", "CARD_DEFENDED", "PASSED");
    }

    @DisplayName("Should keep the full card in the log When an event is written")
    @Test
    void shouldKeepTheFullCardInTheLogWhenAnEventIsWritten() {
        final MatchRecord match = startMatch();

        matchLog.append(match.id(), 1, 1, List.of(
                new DealEvent.FaceDownRevealed(2, PipCard.of(Rank.QUEEN, Suit.SPADES))));

        // ⭐ В логе скрытая информация есть: это внутренняя запись, наружу она уходит
        // только через проекцию.
        assertThat(events.findByMatchIdOrderBySeqAsc(match.id()))
                .singleElement()
                .satisfies(record -> assertThat(record.payload()).contains("Q-spades"));
    }

    @DisplayName("Should refuse a duplicate sequence number When the same seq is written twice")
    @Test
    void shouldRefuseADuplicateSequenceNumberWhenTheSameSeqIsWrittenTwice() {
        final MatchRecord match = startMatch();
        matchLog.append(match.id(), 1, 1, List.of(new DealEvent.Passed(0)));

        // Уникальный индекс (match_id, seq) — гарантия отсутствия дыр и дублей при гонках.
        assertThatThrownBy(() -> matchLog.append(match.id(), 1, 1, List.of(new DealEvent.Passed(1))))
                .isInstanceOf(DataIntegrityViolationException.class);
    }

    @DisplayName("Should record a rejected move When the engine refuses it")
    @Test
    void shouldRecordARejectedMoveWhenTheEngineRefusesIt() {
        final MatchRecord match = startMatch();

        matchLog.appendRejected(match.id(), 1, 1, 3, "PLAY_CARD", "CARD_DOES_NOT_BEAT");

        assertThat(events.findByMatchIdOrderBySeqAsc(match.id()))
                .singleElement()
                .satisfies(record -> {
                    assertThat(record.type()).isEqualTo("MOVE_REJECTED");
                    assertThat(record.payload()).contains("CARD_DOES_NOT_BEAT");
                });
    }

    @DisplayName("Should keep the rules of the moment When the match is started")
    @Test
    void shouldKeepTheRulesOfTheMomentWhenTheMatchIsStarted() {
        final MatchRecord match = matchLog.startMatch(someTableId(), 3, 42L,
                "{\"dealSize\": 6, \"naves\": {\"enabled\": true}}");

        assertThat(matches.findById(match.id())).get().satisfies(saved -> {
            assertThat(saved.status()).isEqualTo(MatchRecordStatus.IN_PROGRESS);
            assertThat(saved.rngSeed()).isEqualTo(42L);
        });
    }

    @DisplayName("Should mark the match finished When it ends with a loser")
    @Test
    void shouldMarkTheMatchFinishedWhenItEndsWithALoser() {
        final MatchRecord match = startMatch();

        matchLog.finish(match.id(), newUser("loser"));

        assertThat(matches.findById(match.id())).get()
                .satisfies(saved -> assertThat(saved.status()).isEqualTo(MatchRecordStatus.FINISHED));
    }

    private MatchRecord startMatch() {
        return matchLog.startMatch(someTableId(), 2, 7L, "{}");
    }

    private UUID someTableId() {
        return lobby.create(newUser("host"), "Стол лога", 4,
                cardSets.findByIsDefaultTrue().orElseThrow().id(),
                themes.findByIsDefaultTrue().orElseThrow().id(), "{}", false).id();
    }

    private UUID newUser(final String username) {
        final String suffix = UUID.randomUUID().toString().substring(0, 8);
        return users.save(new User(UUID.randomUUID(), username + "-" + suffix,
                "Игрок " + username, null, "hash")).id();
    }
}
