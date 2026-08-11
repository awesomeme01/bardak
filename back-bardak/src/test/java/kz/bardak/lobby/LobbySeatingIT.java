package kz.bardak.lobby;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.Callable;
import java.util.concurrent.CyclicBarrier;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.common.web.ApiException;
import kz.bardak.lobby.domain.CardSetRepository;
import kz.bardak.lobby.domain.GameTable;
import kz.bardak.lobby.domain.TablePlayer;
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
 * Посадка за стол на настоящем Postgres.
 *
 * <p>⭐ Главный тест здесь — гонка за последнее место: двое жмут «сесть» одновременно,
 * и сесть обязан ровно один.
 */
@Tag("integration")
@Testcontainers
@SpringBootTest
class LobbySeatingIT {

    @Container
    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = new PostgreSQLContainer<>("postgres:16-alpine");

    @Autowired
    private LobbyService lobby;

    @Autowired
    private UserRepository users;

    @Autowired
    private CardSetRepository cardSets;

    @Autowired
    private TableThemeRepository themes;

    @DisplayName("Should seat the host at the first seat When a table is created")
    @Test
    void shouldSeatTheHostAtTheFirstSeatWhenATableIsCreated() {
        final UUID host = newUser("host");

        final GameTable table = createTable(host, 4);

        assertThat(lobby.seats(table.id())).singleElement().satisfies(seat -> {
            assertThat(seat.userId()).isEqualTo(host);
            assertThat(seat.seatNo()).isZero();
        });
        assertThat(table.code()).hasSize(6);
    }

    @DisplayName("Should give the next free seat When another player joins")
    @Test
    void shouldGiveTheNextFreeSeatWhenAnotherPlayerJoins() {
        final GameTable table = createTable(newUser("host2"), 4);

        final TablePlayer second = lobby.join(table.id(), newUser("guest2"));

        assertThat(second.seatNo()).isEqualTo(1);
        assertThat(lobby.seats(table.id())).hasSize(2);
    }

    @DisplayName("Should reuse the same seat When a player returns to the table")
    @Test
    void shouldReuseTheSameSeatWhenAPlayerReturnsToTheTable() {
        final GameTable table = createTable(newUser("host3"), 4);
        final UUID guest = newUser("guest3");
        final int seatNo = lobby.join(table.id(), guest).seatNo();

        lobby.leave(table.id(), guest);
        final TablePlayer back = lobby.join(table.id(), guest);

        assertThat(back.seatNo()).isEqualTo(seatNo);
        assertThat(lobby.seats(table.id())).hasSize(2);
    }

    @DisplayName("Should free the seat for others When a player leaves")
    @Test
    void shouldFreeTheSeatForOthersWhenAPlayerLeaves() {
        final GameTable table = createTable(newUser("host4"), 2);
        final UUID guest = newUser("guest4");
        lobby.join(table.id(), guest);
        lobby.leave(table.id(), guest);

        final TablePlayer replacement = lobby.join(table.id(), newUser("guest4b"));

        assertThat(replacement.seatNo()).isEqualTo(1);
    }

    @DisplayName("Should refuse the join When every seat is taken")
    @Test
    void shouldRefuseTheJoinWhenEverySeatIsTaken() {
        final GameTable table = createTable(newUser("host5"), 2);
        lobby.join(table.id(), newUser("guest5"));

        assertThatThrownBy(() -> lobby.join(table.id(), newUser("latecomer")))
                .isInstanceOf(ApiException.class)
                .satisfies(thrown -> assertThat(((ApiException) thrown).code()).isEqualTo("TABLE_FULL"));
    }

    @DisplayName("Should seat exactly one player When two race for the last seat")
    @Test
    void shouldSeatExactlyOnePlayerWhenTwoRaceForTheLastSeat() throws Exception {
        final GameTable table = createTable(newUser("host6"), 2);
        final UUID first = newUser("racer-a");
        final UUID second = newUser("racer-b");
        final CyclicBarrier startTogether = new CyclicBarrier(2);

        final ExecutorService pool = Executors.newFixedThreadPool(2);
        try {
            final List<Future<Boolean>> results = pool.invokeAll(List.of(
                    seatAttempt(table.id(), first, startTogether),
                    seatAttempt(table.id(), second, startTogether)));

            final long seated = results.stream().filter(LobbySeatingIT::resultOf).count();
            assertThat(seated)
                    .withFailMessage("За последнее место должен сесть ровно один, село %d", seated)
                    .isEqualTo(1);
            assertThat(lobby.seats(table.id())).hasSize(2);
        } finally {
            pool.shutdownNow();
        }
    }

    @DisplayName("Should hold the table ready When everybody is ready")
    @Test
    void shouldHoldTheTableReadyWhenEverybodyIsReady() {
        final GameTable table = createTable(newUser("host7"), 4);
        final UUID guest = newUser("guest7");
        lobby.join(table.id(), guest);

        lobby.setReady(table.id(), table.hostUserId(), true);
        assertThat(lobby.isReadyToStart(table.id())).isFalse();

        lobby.setReady(table.id(), guest, true);
        assertThat(lobby.isReadyToStart(table.id())).isTrue();
    }

    @DisplayName("Should refuse to close somebody else's table When a guest tries")
    @Test
    void shouldRefuseToCloseSomebodyElsesTableWhenAGuestTries() {
        final GameTable table = createTable(newUser("host8"), 4);
        final UUID guest = newUser("guest8");
        lobby.join(table.id(), guest);

        assertThatThrownBy(() -> lobby.close(table.id(), guest))
                .isInstanceOf(ApiException.class)
                .satisfies(thrown -> assertThat(((ApiException) thrown).code()).isEqualTo("NOT_TABLE_HOST"));
    }

    @DisplayName("Should find the table by its invite code When the code is known")
    @Test
    void shouldFindTheTableByItsInviteCodeWhenTheCodeIsKnown() {
        final GameTable table = createTable(newUser("host9"), 4);

        assertThat(lobby.byCode(table.code().toLowerCase()).id()).isEqualTo(table.id());
    }

    @DisplayName("Should keep private tables out of the list When tables are listed")
    @Test
    void shouldKeepPrivateTablesOutOfTheListWhenTablesAreListed() {
        final GameTable secret = lobby.create(newUser("host10"), "Только свои", 4,
                defaultCardSet(), defaultTheme(), "{}", true);

        assertThat(lobby.openTables()).extracting(GameTable::id).doesNotContain(secret.id());
    }

    private Callable<Boolean> seatAttempt(final UUID tableId, final UUID userId,
                                          final CyclicBarrier startTogether) {
        return () -> {
            startTogether.await();
            try {
                lobby.join(tableId, userId);
                return true;
            } catch (final RuntimeException e) {
                return false;
            }
        };
    }

    private static boolean resultOf(final Future<Boolean> future) {
        try {
            return future.get();
        } catch (final Exception e) {
            return false;
        }
    }

    private GameTable createTable(final UUID host, final int maxPlayers) {
        return lobby.create(host, "Стол " + host, maxPlayers, defaultCardSet(), defaultTheme(),
                "{}", false);
    }

    private UUID defaultCardSet() {
        return cardSets.findByIsDefaultTrue().orElseThrow().id();
    }

    private UUID defaultTheme() {
        return themes.findByIsDefaultTrue().orElseThrow().id();
    }

    /** Логин ограничен 32 символами, поэтому суффикс короткий, а не целый UUID. */
    private UUID newUser(final String username) {
        final String suffix = UUID.randomUUID().toString().substring(0, 8);
        return users.save(new User(UUID.randomUUID(), username + "-" + suffix,
                "Игрок " + username, null, "hash")).id();
    }
}
