package kz.bardak.social.api;

import jakarta.validation.Valid;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.lobby.LobbyService;
import kz.bardak.lobby.domain.GameTable;
import kz.bardak.social.FriendService;
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
 * Друзья: список, заявки и приглашение за стол.
 *
 * <p>Всё вне живой партии ходит по REST (ADR-001) — друзья не исключение. По сокету уходит
 * только само приглашение, потому что его надо услышать сразу.
 */
@RestController
@RequestMapping("/api/friends")
public class FriendController {

    private final FriendService friends;
    private final LobbyService lobby;

    public FriendController(final FriendService friends, final LobbyService lobby) {
        this.friends = Objects.requireNonNull(friends, "friends");
        this.lobby = Objects.requireNonNull(lobby, "lobby");
    }

    @GetMapping
    public FriendDtos.FriendList list(@AuthenticationPrincipal final Jwt jwt) {
        return friends.list(userId(jwt));
    }

    /** Позвать в друзья по логину. Логин ищется без учёта регистра — как и при входе. */
    @PostMapping("/requests")
    public FriendDtos.Friend request(@Valid @RequestBody final FriendDtos.AddFriendRequest request,
                                     @AuthenticationPrincipal final Jwt jwt) {
        return friends.request(userId(jwt), request.username());
    }

    @PostMapping("/{friendId}/accept")
    public FriendDtos.Friend accept(@PathVariable final UUID friendId,
                                    @AuthenticationPrincipal final Jwt jwt) {
        return friends.accept(userId(jwt), friendId);
    }

    /** Убрать из друзей или отклонить заявку — для сервера это одно и то же. */
    @DeleteMapping("/{friendId}")
    public ResponseEntity<Void> remove(@PathVariable final UUID friendId,
                                       @AuthenticationPrincipal final Jwt jwt) {
        friends.remove(userId(jwt), friendId);
        return ResponseEntity.status(HttpStatus.NO_CONTENT).build();
    }

    /**
     * Позвать друга за свой стол.
     *
     * <p>Ответ говорит, услышал ли друг прямо сейчас: приглашение не хранится, и экрану
     * надо честно показать «позвал» или «его нет в сети — позвали уведомлением».
     */
    @PostMapping("/{friendId}/invite")
    public Map<String, Object> invite(@PathVariable final UUID friendId,
                                      @Valid @RequestBody final FriendDtos.InviteRequest request,
                                      @AuthenticationPrincipal final Jwt jwt) {
        final GameTable table = lobby.byId(UUID.fromString(request.tableId()));
        final boolean delivered = friends.invite(userId(jwt), friendId, table.id(),
                table.name(), table.code());
        return Map.of("delivered", delivered);
    }

    private static UUID userId(final Jwt jwt) {
        return UUID.fromString(jwt.getSubject());
    }
}
