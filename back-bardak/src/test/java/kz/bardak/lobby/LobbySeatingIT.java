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
import kz.bardak.TestPostgres;
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

/**
 * Посадка за стол на настоящем Postgres.
 *
 * <p>⭐ Главный тест здесь — гонка за последнее место: двое жмут «сесть» одновременно,
 * и сесть обязан ровно один.
 */
@Tag("integration")
@SpringBootTest
class LobbySeatingIT {

    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

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

    @DisplayName("Should keep the seat When a player tries to leave in the middle of a match")
    @Test
    void shouldKeepTheSeatWhenAPlayerTriesToLeaveInTheMiddleOfAMatch() {
        final GameTable table = createTable(newUser("host-mid"), 2);
        final UUID guest = newUser("guest-mid");
        lobby.join(table.id(), guest);
        lobby.startMatch(table.id());

        assertThatThrownBy(() -> lobby.leave(table.id(), guest))
                .isInstanceOf(ApiException.class)
                .satisfies(thrown -> assertThat(((ApiException) thrown).code())
                        .isEqualTo("MATCH_IN_PROGRESS"));

        // ⭐ Место осталось за игроком: иначе его занял бы посторонний, а движок
        // продолжал бы ждать ушедшего.
        assertThat(lobby.seats(table.id())).hasSize(2);
    }

    @DisplayName("Should point back to the table When the player is seated at one")
    @Test
    void shouldPointBackToTheTableWhenThePlayerIsSeatedAtOne() {
        final UUID guest = newUser("guest-back");
        final GameTable table = createTable(newUser("host-back"), 4);
        lobby.join(table.id(), guest);

        assertThat(lobby.currentTableOf(guest)).get()
                .satisfies(found -> assertThat(found.id()).isEqualTo(table.id()));
    }

    @DisplayName("Should point nowhere When the player left the table")
    @Test
    void shouldPointNowhereWhenThePlayerLeftTheTable() {
        final UUID guest = newUser("guest-away");
        final GameTable table = createTable(newUser("host-away"), 4);
        lobby.join(table.id(), guest);

        lobby.leave(table.id(), guest);

        assertThat(lobby.currentTableOf(guest)).isEmpty();
    }

    @DisplayName("Should point back to the table When the match is already running")
    @Test
    void shouldPointBackToTheTableWhenTheMatchIsAlreadyRunning() {
        final UUID guest = newUser("guest-inmatch");
        final GameTable table = createTable(newUser("host-inmatch"), 2);
        lobby.join(table.id(), guest);
        lobby.startMatch(table.id());

        // Закрытая вкладка не должна стоить партии: стол находится сам.
        assertThat(lobby.currentTableOf(guest)).get()
                .satisfies(found -> assertThat(found.status().name()).isEqualTo("IN_MATCH"));
    }

    /**
     * ⚠️ Регрессия: нетерпеливый двойной клик по «Создать стол» оставлял в лобби по столу
     * на каждое нажатие — у одного игрока их накапливался десяток, и убирать их было некому.
     */
    @DisplayName("Should keep only the newest table When the same player creates twice")
    @Test
    void shouldKeepOnlyTheNewestTableWhenTheSamePlayerCreatesTwice() {
        final UUID host = newUser("host-twice");
        final GameTable first = createTable(host, 4);

        final GameTable second = createTable(host, 4);

        assertThat(second.id()).isNotEqualTo(first.id());
        assertThat(lobby.currentTableOf(host)).get()
                .satisfies(found -> assertThat(found.id()).isEqualTo(second.id()));
        assertThat(lobby.openTables())
                .as("брошенный пустой стол не остаётся висеть в лобби")
                .noneSatisfy(open -> assertThat(open.id()).isEqualTo(first.id()));
    }

    @DisplayName("Should keep the guests table alive When the host leaves it for a new one")
    @Test
    void shouldKeepTheGuestsTableAliveWhenTheHostLeavesItForANewOne() {
        final UUID host = newUser("host-moves");
        final UUID guest = newUser("guest-stays");
        final GameTable abandoned = createTable(host, 4);
        lobby.join(abandoned.id(), guest);

        createTable(host, 4);

        assertThat(lobby.openTables())
                .as("за столом остался игрок — закрывать его нельзя")
                .anySatisfy(open -> assertThat(open.id()).isEqualTo(abandoned.id()));
    }

    /**
     * ⚠️ Гонка, а не двойной клик: пять одновременных «Создать стол» успевали прочитать
     * «я нигде не сижу» раньше, чем сосед вставлял строку, и игрок оказывался сразу за
     * пятью столами. Ловится уникальным индексом на {@code table_players.user_id}.
     */
    @DisplayName("Should seat the player at exactly one table When creates race each other")
    @Test
    void shouldSeatThePlayerAtExactlyOneTableWhenCreatesRaceEachOther() throws Exception {
        final UUID host = newUser("host-race");
        final int attempts = 5;
        final CyclicBarrier startTogether = new CyclicBarrier(attempts);
        final ExecutorService pool = Executors.newFixedThreadPool(attempts);
        try {
            final List<Callable<Boolean>> creates = new java.util.ArrayList<>();
            for (int attempt = 0; attempt < attempts; attempt++) {
                creates.add(() -> {
                    startTogether.await();
                    try {
                        createTable(host, 4);
                        return true;
                    } catch (final RuntimeException expected) {
                        // Проигравшие гонку получают отказ — это и есть правильный исход.
                        return false;
                    }
                });
            }
            pool.invokeAll(creates);

            assertThat(lobby.currentTableOf(host))
                    .as("игрок сидит за одним столом за раз, чем бы ни кончилась гонка")
                    .isPresent();
            assertThat(seatedTablesOf(host))
                    .withFailMessage("Игрок оказался сразу за %d столами", seatedTablesOf(host))
                    .isEqualTo(1);
        } finally {
            pool.shutdownNow();
        }
    }

    /** Сколько столов реально держат этого игрока — считаем по открытым столам лобби. */
    private long seatedTablesOf(final UUID userId) {
        return lobby.openTables().stream()
                .filter(open -> lobby.seats(open.id()).stream()
                        .anyMatch(seat -> seat.userId().equals(userId)))
                .count();
    }

    @DisplayName("Should refuse a new table When the player is in the middle of a match")
    @Test
    void shouldRefuseANewTableWhenThePlayerIsInTheMiddleOfAMatch() {
        final UUID host = newUser("host-busy");
        final GameTable table = createTable(host, 2);
        lobby.join(table.id(), newUser("guest-busy"));
        lobby.startMatch(table.id());

        assertThatThrownBy(() -> createTable(host, 4))
                .isInstanceOf(ApiException.class)
                .hasMessageContaining("доиграй");
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
