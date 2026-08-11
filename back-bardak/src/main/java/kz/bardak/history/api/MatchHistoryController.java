package kz.bardak.history.api;

import java.util.List;
import java.util.Objects;
import java.util.UUID;
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

    public MatchHistoryController(final MatchHistoryService history) {
        this.history = Objects.requireNonNull(history, "history");
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
        return history.matchesOf(userId != null ? userId : userId(jwt), status);
    }

    @GetMapping("/{id}")
    public HistoryDtos.MatchDetails details(@PathVariable final UUID id,
                                            @AuthenticationPrincipal final Jwt jwt) {
        return history.details(id, userId(jwt));
    }

    @GetMapping("/{id}/deals")
    public List<HistoryDtos.DealSummary> deals(@PathVariable final UUID id) {
        return history.dealsOf(id);
    }

    /** ⚠️ Только после матча и только с точки зрения спрашивающего. */
    @GetMapping("/{id}/replay")
    public HistoryDtos.Replay replay(@PathVariable final UUID id,
                                     @AuthenticationPrincipal final Jwt jwt) {
        return history.replay(id, userId(jwt));
    }

    private UUID userId(final Jwt jwt) {
        return UUID.fromString(jwt.getSubject());
    }
}
