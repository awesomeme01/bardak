package kz.bardak.history;

import static org.assertj.core.api.Assertions.assertThat;

import java.util.List;
import java.util.UUID;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.game.runtime.MatchService;
import kz.bardak.game.runtime.MatchSession;
import kz.bardak.game.rules.DealCommand;
import kz.bardak.game.rules.DealPhase;
import kz.bardak.game.rules.MatchResult;
import kz.bardak.game.rules.Suit;
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
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

/**
 * Восстановление матча из снимка — то, ради чего снимки и пишутся: рестарт сервера
 * не должен убивать партию.
 */
@Tag("integration")
@Testcontainers
@SpringBootTest
class SnapshotRestoreIT {

    @Container
    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = new PostgreSQLContainer<>("postgres:16-alpine");

    @Autowired
    private MatchService matches;

    @Autowired
    private MatchLog matchLog;

    @Autowired
    private LobbyService lobby;

    @Autowired
    private UserRepository users;

    @Autowired
    private CardSetRepository cardSets;

    @Autowired
    private TableThemeRepository themes;

    @DisplayName("Should bring the match back exactly When the server forgot it")
    @Test
    void shouldBringTheMatchBackExactlyWhenTheServerForgotIt() {
        final UUID tableId = readyTable("snap-a", "snap-b");
        final MatchSession session = matches.start(tableId);
        resolveDice(session);

        // Один ход и снимок — как это делает боевой обработчик команд.
        final int attacker = session.state().deal().attackRightSeat();
        final var card = session.state().deal().playerAt(attacker).hand().get(0);
        final MatchResult result = session.apply(new DealCommand.Attack(attacker, card));
        assertThat(result.isApplied()).isTrue();
        session.lastSeq(1);
        matchLog.snapshot(session.matchId(), 1, matches.stateCodec().encode(session.state()));

        // Сервер «перезапустился»: в памяти матча больше нет.
        matches.finish(tableId);
        assertThat(matches.find(tableId)).isPresent();

        final MatchSession restored = matches.find(tableId).orElseThrow();
        assertThat(restored.state()).isEqualTo(session.state());
        assertThat(restored.matchId()).isEqualTo(session.matchId());
        assertThat(restored.players()).isEqualTo(session.players());
        assertThat(restored.lastSeq()).isEqualTo(1);
    }

    /**
     * ⭐ Довести раздачу до фазы атаки.
     *
     * <p>Нижней картой колоды может выпасть джокер — тогда масть козыря разыгрывается
     * костью (§1.2), и до её выбора любой ход отклоняется. Сдача случайна: без этого шага
     * тест падает примерно раз в восемь прогонов, и падение выглядит как поломка снимков.
     */
    private void resolveDice(final MatchSession session) {
        if (session.state().deal().phase() != DealPhase.DICE) {
            return;
        }
        final int chooser = session.state().deal().attackRightSeat();
        for (final Suit suit : Suit.values()) {
            if (session.apply(new DealCommand.ChooseTrump(chooser, suit)).isApplied()) {
                return;
            }
        }
        throw new AssertionError("Козырь разыгрывается костью, но ни одна масть не подошла");
    }

    @DisplayName("Should keep every hand intact When the match is restored")
    @Test
    void shouldKeepEveryHandIntactWhenTheMatchIsRestored() {
        final UUID tableId = readyTable("snap-c", "snap-d");
        final MatchSession session = matches.start(tableId);
        matchLog.snapshot(session.matchId(), 0, matches.stateCodec().encode(session.state()));
        final var handsBefore = session.state().deal().players();

        matches.finish(tableId);

        assertThat(matches.find(tableId).orElseThrow().state().deal().players())
                .isEqualTo(handsBefore);
    }

    @DisplayName("Should stay empty When the table never played a match")
    @Test
    void shouldStayEmptyWhenTheTableNeverPlayedAMatch() {
        assertThat(matches.find(readyTable("snap-e", "snap-f"))).isEmpty();
    }

    private UUID readyTable(final String hostName, final String guestName) {
        final UUID host = newUser(hostName);
        final UUID guest = newUser(guestName);
        final UUID tableId = lobby.create(host, "Снимки", 2,
                cardSets.findByIsDefaultTrue().orElseThrow().id(),
                themes.findByIsDefaultTrue().orElseThrow().id(), "{}", false).id();
        lobby.join(tableId, guest);
        lobby.setReady(tableId, host, true);
        lobby.setReady(tableId, guest, true);
        return tableId;
    }

    private UUID newUser(final String username) {
        final String suffix = UUID.randomUUID().toString().substring(0, 8);
        return users.save(new User(UUID.randomUUID(), username + "-" + suffix,
                "Игрок " + username, null, "hash")).id();
    }
}
