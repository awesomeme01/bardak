package kz.bardak.rating.api;

import java.util.List;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.rating.RatingService;
import org.springframework.security.core.annotation.AuthenticationPrincipal;
import org.springframework.security.oauth2.jwt.Jwt;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

/** Рейтинг и сезоны. */
@RestController
@RequestMapping("/api/rating")
public class RatingController {

    private final RatingService rating;

    public RatingController(final RatingService rating) {
        this.rating = Objects.requireNonNull(rating, "rating");
    }

    @GetMapping("/me")
    public RatingDtos.RatingView mine(@AuthenticationPrincipal final Jwt jwt) {
        return rating.of(UUID.fromString(jwt.getSubject()));
    }

    @GetMapping("/users/{id}")
    public RatingDtos.RatingView of(@PathVariable final UUID id) {
        return rating.of(id);
    }

    @GetMapping("/top")
    public List<RatingDtos.LeaderRow> top() {
        return rating.leaderboard();
    }

    @GetMapping("/seasons")
    public RatingDtos.SeasonsView seasons(@AuthenticationPrincipal final Jwt jwt) {
        return rating.seasons(jwt.getClaimAsString("username"));
    }

    /** Закрыть текущий сезон и открыть следующий (ADR-037). */
    @PostMapping("/seasons")
    public RatingDtos.SeasonView newSeason(@RequestBody final RatingDtos.CreateSeasonRequest request,
                                           @AuthenticationPrincipal final Jwt jwt) {
        return rating.closeAndOpen(jwt.getClaimAsString("username"), request.name());
    }
}
