package kz.bardak.lobby;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
import java.util.UUID;
import kz.bardak.TestPostgres;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.lobby.domain.CardSetRepository;
import kz.bardak.lobby.domain.GameTable;
import kz.bardak.lobby.domain.TableThemeRepository;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.boot.testcontainers.service.connection.ServiceConnection;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.testcontainers.containers.PostgreSQLContainer;

/**
 * Приглашение за стол по ссылке.
 *
 * <p>⭐ Ручка отдаётся <b>без токена</b> намеренно: по ссылке приходит тот, у кого учётки
 * ещё нет, и он должен увидеть, куда его зовут, до регистрации. Проверка закрепляет это
 * как решение, а не как случайность конфигурации.
 *
 * <p>⚠️ И тут же закрепляет обратное: имён игроков в ответе быть не должно. Код стола
 * короткий и живёт в переписке — всё, что попадёт в этот ответ, публично.
 */
@Tag("integration")
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class TableInviteLinkIT {

    @ServiceConnection
    static final PostgreSQLContainer<?> POSTGRES = TestPostgres.INSTANCE;

    @Autowired
    private TestRestTemplate rest;

    @Autowired
    private LobbyService lobby;

    @Autowired
    private UserRepository users;

    @Autowired
    private CardSetRepository cardSets;

    @Autowired
    private TableThemeRepository themes;

    @DisplayName("Should show the table by its code When nobody is signed in")
    @Test
    void shouldShowTheTableByItsCodeWhenNobodyIsSignedIn() {
        final GameTable table = createTable("Стол на вечер", 4);

        final ResponseEntity<JsonNode> response =
                rest.getForEntity("/api/tables/invite/" + table.code(), JsonNode.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
        assertThat(response.getBody().get("name").asText()).isEqualTo("Стол на вечер");
        assertThat(response.getBody().get("maxPlayers").asInt()).isEqualTo(4);
        assertThat(response.getBody().get("seatsTaken").asInt()).isEqualTo(1);
        assertThat(response.getBody().get("joinable").asBoolean()).isTrue();
    }

    @DisplayName("Should hide who is at the table When the invite is read without a token")
    @Test
    void shouldHideWhoIsAtTheTableWhenTheInviteIsReadWithoutAToken() {
        final GameTable table = createTable("Секретный", 2);

        final JsonNode body = rest.getForEntity("/api/tables/invite/" + table.code(),
                JsonNode.class).getBody();

        // Ни списка мест, ни имён, ни идентификаторов: код видел кто угодно.
        assertThat(body.has("seats")).isFalse();
        assertThat(body.has("hostUserId")).isFalse();
        assertThat(body.has("id")).isFalse();
    }

    @DisplayName("Should find the table When the code is written in lower case")
    @Test
    void shouldFindTheTableWhenTheCodeIsWrittenInLowerCase() {
        final GameTable table = createTable("Регистр", 2);

        final ResponseEntity<JsonNode> response = rest.getForEntity(
                "/api/tables/invite/" + table.code().toLowerCase(), JsonNode.class);

        // ⭐ Ссылку пересылают через мессенджеры, а те охотно ломают регистр.
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
    }

    @DisplayName("Should say the table is not joinable When every seat is taken")
    @Test
    void shouldSayTheTableIsNotJoinableWhenEverySeatIsTaken() {
        final GameTable table = createTable("Полный", 2);
        lobby.join(table.id(), newUser("guest"));

        final JsonNode body = rest.getForEntity("/api/tables/invite/" + table.code(),
                JsonNode.class).getBody();

        // ⚠️ Честно до регистрации: иначе человек заводит учётку и упирается в отказ.
        assertThat(body.get("seatsTaken").asInt()).isEqualTo(2);
        assertThat(body.get("joinable").asBoolean()).isFalse();
    }

    @DisplayName("Should answer not found When the code belongs to no table")
    @Test
    void shouldAnswerNotFoundWhenTheCodeBelongsToNoTable() {
        final ResponseEntity<JsonNode> response =
                rest.getForEntity("/api/tables/invite/ZZZZZZ", JsonNode.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
    }

    @DisplayName("Should still require a token When the full table view is asked for")
    @Test
    void shouldStillRequireATokenWhenTheFullTableViewIsAskedFor() {
        final GameTable table = createTable("Закрытый", 2);

        // ⚠️ Открыли ровно одну ручку. Полный вид со списком игроков остаётся за токеном.
        assertThat(rest.getForEntity("/api/tables/by-code/" + table.code(), JsonNode.class)
                .getStatusCode()).isEqualTo(HttpStatus.UNAUTHORIZED);
    }

    private GameTable createTable(final String name, final int maxPlayers) {
        return lobby.create(newUser("host"), name, maxPlayers,
                cardSets.findByIsDefaultTrue().orElseThrow().id(),
                themes.findByIsDefaultTrue().orElseThrow().id(), "{}", false);
    }

    /** Логин ограничен 32 символами, поэтому суффикс короткий, а не целый UUID. */
    private UUID newUser(final String username) {
        final String suffix = UUID.randomUUID().toString().substring(0, 8);
        return users.save(new User(UUID.randomUUID(), username + "-" + suffix,
                "Игрок " + username, null, "hash")).id();
    }
}
