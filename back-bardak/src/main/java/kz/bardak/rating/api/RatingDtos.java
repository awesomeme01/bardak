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

    /**
     * Сезоны и право спрашивающего ими управлять.
     *
     * <p>⭐ `canManage` едет вместе со списком, а не отдельной ручкой и не флагом в профиле:
     * право живёт в настройках рейтинга, и знать о нём должен модуль рейтинга, а не
     * авторизация. Без этого признака экран показывал бы кнопку «закрыть сезон» всем,
     * а отказ прилетал бы уже после нажатия.
     *
     * @param canManage спрашивающий вправе закрыть сезон и открыть следующий (ADR-037)
     */
    public record SeasonsView(List<SeasonView> seasons, boolean canManage) {
    }

    public record CreateSeasonRequest(String name) {
    }
}
