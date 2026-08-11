package kz.bardak.lobby.api;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.validation.Valid;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.common.web.ApiException;
import kz.bardak.lobby.LobbyService;
import kz.bardak.lobby.domain.CardSetRepository;
import kz.bardak.lobby.domain.GameTable;
import kz.bardak.lobby.domain.TablePlayer;
import kz.bardak.lobby.domain.TableThemeRepository;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

/**
 * Столы. Создание и просмотр — REST; вход за стол — WebSocket (`02-architecture.md`):
 * вход меняет живое состояние, которое видят остальные, и потому идёт по тому же каналу,
 * что и события.
 */
@RestController
@RequestMapping("/api/tables")
public class TableController {

    private final LobbyService lobby;
    private final UserRepository users;
    private final CardSetRepository cardSets;
    private final TableThemeRepository themes;
    private final ObjectMapper objectMapper;

    public TableController(final LobbyService lobby, final UserRepository users,
                           final CardSetRepository cardSets, final TableThemeRepository themes,
                           final ObjectMapper objectMapper) {
        this.lobby = Objects.requireNonNull(lobby, "lobby");
        this.users = Objects.requireNonNull(users, "users");
        this.cardSets = Objects.requireNonNull(cardSets, "cardSets");
        this.themes = Objects.requireNonNull(themes, "themes");
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper");
    }

    @GetMapping
    public List<LobbyDtos.TableView> openTables() {
        return lobby.openTables().stream().map(this::toView).toList();
    }

    @PostMapping
    public LobbyDtos.TableView create(@Valid @RequestBody final LobbyDtos.CreateTableRequest request,
                                      @AuthenticationPrincipal final Jwt jwt) {
        final UUID cardSetId = request.cardSetId() != null ? request.cardSetId()
                : cardSets.findByIsDefaultTrue().orElseThrow(() -> missing("набор карт")).id();
        final UUID themeId = request.themeId() != null ? request.themeId()
                : themes.findByIsDefaultTrue().orElseThrow(() -> missing("тема стола")).id();

        return toView(lobby.create(userId(jwt), request.name(), request.maxPlayers(),
                cardSetId, themeId, asJson(request.rulesConfig()), request.isPrivate()));
    }

    @GetMapping("/{id}")
    public LobbyDtos.TableView byId(@PathVariable final UUID id) {
        return toView(lobby.byId(id));
    }

    @GetMapping("/by-code/{code}")
    public LobbyDtos.TableView byCode(@PathVariable final String code) {
        return toView(lobby.byCode(code));
    }

    @DeleteMapping("/{id}")
    public ResponseEntity<Void> close(@PathVariable final UUID id, @AuthenticationPrincipal final Jwt jwt) {
        lobby.close(id, userId(jwt));
        return ResponseEntity.status(HttpStatus.NO_CONTENT).build();
    }

    private LobbyDtos.TableView toView(final GameTable table) {
        final List<LobbyDtos.SeatView> seats = lobby.seats(table.id()).stream()
                .map(this::toSeatView)
                .toList();
        return new LobbyDtos.TableView(table.id().toString(), table.code(), table.name(),
                table.hostUserId().toString(), table.maxPlayers(), table.status().name(),
                table.cardSetId().toString(), table.themeId().toString(), table.isPrivate(), seats);
    }

    private LobbyDtos.SeatView toSeatView(final TablePlayer seat) {
        final String displayName = users.findById(seat.userId())
                .map(User::displayName)
                .orElse("—");
        // online появится вместе с presence на M3/M8; пока за столом — значит на связи.
        return new LobbyDtos.SeatView(seat.seatNo(), seat.userId().toString(), displayName,
                seat.isReady(), true);
    }

    private String asJson(final Map<String, Object> rulesConfig) {
        try {
            return objectMapper.writeValueAsString(rulesConfig == null ? Map.of() : rulesConfig);
        } catch (final JsonProcessingException e) {
            throw new ApiException(HttpStatus.BAD_REQUEST, "INVALID_RULES_CONFIG",
                    "Не удалось разобрать правила стола");
        }
    }

    private static UUID userId(final Jwt jwt) {
        return UUID.fromString(jwt.getSubject());
    }

    private static ApiException missing(final String what) {
        return new ApiException(HttpStatus.INTERNAL_SERVER_ERROR, "NO_DEFAULT",
                "Не настроен %s по умолчанию".formatted(what));
    }
}
