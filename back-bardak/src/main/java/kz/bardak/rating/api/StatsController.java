package kz.bardak.rating.api;

import java.util.Objects;
import java.util.UUID;
import kz.bardak.rating.StatsService;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

/** Статистика: своя и чужая. Чужая открыта — за столом и так всё друг про друга знают. */
@RestController
@RequestMapping("/api/stats")
public class StatsController {

    private final StatsService stats;

    public StatsController(final StatsService stats) {
        this.stats = Objects.requireNonNull(stats, "stats");
    }

    @GetMapping("/me")
    public StatsDtos.PlayerStats mine(@AuthenticationPrincipal final Jwt jwt) {
        return stats.of(UUID.fromString(jwt.getSubject()));
    }

    @GetMapping("/users/{id}")
    public StatsDtos.PlayerStats of(@PathVariable final UUID id) {
        return stats.of(id);
    }
}
