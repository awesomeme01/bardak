package kz.bardak.rating.api;

import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;
import java.util.UUID;

/** Формы рейтинга и сезонов. */
public final class RatingDtos {

    private RatingDtos() {
    }

    public record RatingView(UUID userId, String displayName, BigDecimal rating, int matchesPlayed,
                             List<RatingPoint> history) {
    }

    /** Точка графика: рейтинг после матча. */
    public record RatingPoint(UUID matchId, BigDecimal ratingBefore, BigDecimal ratingAfter,
                              int place, int playersCount, Instant playedAt) {
    }

    public record LeaderRow(UUID userId, String displayName, BigDecimal rating, int matchesPlayed) {
    }

    public record SeasonView(UUID id, String name, Instant startedAt, Instant closedAt, boolean open) {
    }

    public record CreateSeasonRequest(String name) {
    }
}
