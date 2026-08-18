package kz.bardak.social;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.util.UUID;
import kz.bardak.TestPostgres;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.common.web.ApiException;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.testcontainers.containers.PostgreSQLContainer;

/**
 * Друзья: заявки, взаимность и присутствие.
 *
 * <p>⭐ Главное здесь — что дружба одна на пару. Проверяется с обеих сторон: список друга
 * обязан совпадать с моим, иначе однажды окажется, что он у меня есть, а я у него нет.
 */
@Tag("integration")
@SpringBootTest
class FriendsIT {

    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

    @Autowired
    private FriendService friends;

    @Autowired
    private Presence presence;

    @Autowired
    private UserRepository users;

    @DisplayName("Should show the request on both sides When one player asks another")
    @Test
    void shouldShowTheRequestOnBothSidesWhenOnePlayerAsksAnother() {
        final UUID asker = newUser("fr-asker");
        final UUID target = newUser("fr-target");

        friends.request(asker, "fr-target");

        assertThat(friends.list(asker).outgoing()).extracting("username").containsExactly("fr-target");
        assertThat(friends.list(asker).friends()).isEmpty();
        assertThat(friends.list(target).incoming()).extracting("username").containsExactly("fr-asker");
    }

    @DisplayName("Should make them friends for both When the request is accepted")
    @Test
    void shouldMakeThemFriendsForBothWhenTheRequestIsAccepted() {
        final UUID asker = newUser("fr-a2");
        final UUID target = newUser("fr-b2");
        friends.request(asker, "fr-b2");

        friends.accept(target, asker);

        assertThat(friends.list(asker).friends()).extracting("username").containsExactly("fr-b2");
        assertThat(friends.list(target).friends()).extracting("username").containsExactly("fr-a2");
        assertThat(friends.isFriend(asker, target)).isTrue();
        assertThat(friends.isFriend(target, asker)).isTrue();
    }

    /**
     * ⚠️ Двое нажали «добавить» одновременно. Встречная заявка — это согласие, а не вторая
     * заявка: иначе оба остались бы с висящими приглашениями и без дружбы.
     */
    @DisplayName("Should become friends at once When both send a request to each other")
    @Test
    void shouldBecomeFriendsAtOnceWhenBothSendARequestToEachOther() {
        final UUID one = newUser("fr-mutual1");
        final UUID two = newUser("fr-mutual2");
        friends.request(one, "fr-mutual2");

        friends.request(two, "fr-mutual1");

        assertThat(friends.isFriend(one, two)).isTrue();
        assertThat(friends.list(one).incoming()).isEmpty();
        assertThat(friends.list(two).outgoing()).isEmpty();
    }

    @DisplayName("Should refuse the acceptance When the asker tries to accept his own request")
    @Test
    void shouldRefuseTheAcceptanceWhenTheAskerTriesToAcceptHisOwnRequest() {
        final UUID asker = newUser("fr-self-a");
        final UUID target = newUser("fr-self-b");
        friends.request(asker, "fr-self-b");

        assertThatThrownBy(() -> friends.accept(asker, target))
                .isInstanceOf(ApiException.class)
                .hasMessageContaining("принимаешь не ты");
    }

    @DisplayName("Should refuse the friendship When a player adds himself")
    @Test
    void shouldRefuseTheFriendshipWhenAPlayerAddsHimself() {
        final UUID alone = newUser("fr-alone");

        assertThatThrownBy(() -> friends.request(alone, "fr-alone"))
                .isInstanceOf(ApiException.class)
                .hasMessageContaining("самим собой");
    }

    @DisplayName("Should find the player ignoring case When the login is typed differently")
    @Test
    void shouldFindThePlayerIgnoringCaseWhenTheLoginIsTypedDifferently() {
        final UUID asker = newUser("fr-case-a");
        newUser("fr-Case-B");

        friends.request(asker, "FR-CASE-B");

        assertThat(friends.list(asker).outgoing()).extracting("username").containsExactly("fr-Case-B");
    }

    @DisplayName("Should drop the pair for both When one of them removes the other")
    @Test
    void shouldDropThePairForBothWhenOneOfThemRemovesTheOther() {
        final UUID one = newUser("fr-drop1");
        final UUID two = newUser("fr-drop2");
        friends.request(one, "fr-drop2");
        friends.accept(two, one);

        friends.remove(two, one);

        assertThat(friends.list(one).friends()).isEmpty();
        assertThat(friends.list(two).friends()).isEmpty();
    }

    @DisplayName("Should mark the friend online When his socket is connected")
    @Test
    void shouldMarkTheFriendOnlineWhenHisSocketIsConnected() {
        final UUID watcher = newUser("fr-watch");
        final UUID player = newUser("fr-player");
        friends.request(watcher, "fr-player");
        friends.accept(player, watcher);

        presence.connected(player, "session-1", message -> { });

        assertThat(friends.list(watcher).friends()).singleElement()
                .extracting("online").isEqualTo(true);

        presence.disconnected(player, "session-1");
        assertThat(friends.list(watcher).friends()).singleElement()
                .extracting("online").isEqualTo(false);
    }

    /**
     * ⚠️ У человека бывает несколько вкладок. Закрытая вкладка — не уход: онлайн
     * заканчивается на последнем соединении, а не на первом закрытом.
     */
    @DisplayName("Should keep him online When one of his two sockets closes")
    @Test
    void shouldKeepHimOnlineWhenOneOfHisTwoSocketsCloses() {
        final UUID player = newUser("fr-two-tabs");
        presence.connected(player, "tab-1", message -> { });
        presence.connected(player, "tab-2", message -> { });

        presence.disconnected(player, "tab-1");

        assertThat(presence.isOnline(player)).isTrue();
        presence.disconnected(player, "tab-2");
        assertThat(presence.isOnline(player)).isFalse();
    }

    @DisplayName("Should refuse the invite When the players are not friends")
    @Test
    void shouldRefuseTheInviteWhenThePlayersAreNotFriends() {
        final UUID host = newUser("fr-host");
        final UUID stranger = newUser("fr-stranger");

        assertThatThrownBy(() -> friends.invite(host, stranger, UUID.randomUUID(), "Стол", "ABC123"))
                .isInstanceOf(ApiException.class)
                .hasMessageContaining("только друзей");
    }

    private UUID newUser(final String username) {
        return users.save(new User(UUID.randomUUID(), username, username, null, "hash")).id();
    }
}
