package kz.bardak.history.api;

import java.util.List;
import java.util.Set;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.common.web.ApiException;
import kz.bardak.social.FriendService;
import org.springframework.http.HttpStatus;
import kz.bardak.history.MatchHistoryService;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

/** История матчей (`05-api-contracts.md`). */
@RestController
@RequestMapping("/api/matches")
public class MatchHistoryController {

    private final MatchHistoryService history;
    private final FriendService friends;

    public MatchHistoryController(final MatchHistoryService history, final FriendService friends) {
        this.history = Objects.requireNonNull(history, "history");
        this.friends = Objects.requireNonNull(friends, "friends");
    }

    /**
     * Матчи игрока, новые сверху.
     *
     * @param userId чей список; по умолчанию свой
     */
    @GetMapping
    public List<HistoryDtos.MatchSummary> matches(@RequestParam(required = false) final UUID userId,
                                                  @RequestParam(required = false) final String status,
                                                  @AuthenticationPrincipal final Jwt jwt) {
        final UUID me = userId(jwt);
        final UUID whose = userId != null ? userId : me;
        requireVisible(me, whose);
        return history.matchesOf(whose, status);
    }

    @GetMapping("/{id}")
    public HistoryDtos.MatchDetails details(@PathVariable final UUID id,
                                            @AuthenticationPrincipal final Jwt jwt) {
        requireMatchVisible(id, userId(jwt));
        return history.details(id, userId(jwt));
    }

    /** ⚠️ Только после матча и только с точки зрения спрашивающего. */
    @GetMapping("/{id}/replay")
    public HistoryDtos.Replay replay(@PathVariable final UUID id,
                                     @AuthenticationPrincipal final Jwt jwt) {
        requireMatchVisible(id, userId(jwt));
        return history.replay(id, userId(jwt));
    }

    /**
     * Чью историю можно смотреть: свою и друзей.
     *
     * <p>⭐ Рейтинг и статистика остаются открытыми — на них держится таблица лидеров,
     * это агрегаты. История же — это «с кем и когда я играл», и посторонним её знать
     * незачем. Раньше её отдавало кому угодно по идентификатору игрока.
     */
    private void requireVisible(final UUID me, final UUID whose) {
        if (me.equals(whose) || friends.isFriend(me, whose)) {
            return;
        }
        throw new ApiException(HttpStatus.FORBIDDEN, "NOT_FRIENDS",
                "История матчей видна своим и друзьям");
    }

    /**
     * Матч виден участнику и другу любого из участников.
     *
     * <p>⚠️ Проверка стоит на границе, а не в сервисе: модулю истории незачем знать
     * про дружбу, а решение «показывать ли» — это авторизация, то есть работа границы.
     */
    private void requireMatchVisible(final UUID matchId, final UUID me) {
        final Set<UUID> players = history.participantsOf(matchId);
        if (players.contains(me) || players.stream().anyMatch(player -> friends.isFriend(me, player))) {
            return;
        }
        throw new ApiException(HttpStatus.FORBIDDEN, "NOT_FRIENDS",
                "Этот матч видят его игроки и их друзья");
    }

    private UUID userId(final Jwt jwt) {
        return UUID.fromString(jwt.getSubject());
    }
}
