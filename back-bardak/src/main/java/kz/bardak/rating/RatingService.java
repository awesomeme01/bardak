package kz.bardak.rating;

import java.time.Clock;
import java.util.List;
import java.util.Objects;
import java.util.UUID;
import kz.bardak.auth.domain.User;
import kz.bardak.auth.domain.UserRepository;
import kz.bardak.common.web.ApiException;
import kz.bardak.rating.api.RatingDtos;
import kz.bardak.rating.domain.RatingHistoryEntry;
import kz.bardak.rating.domain.RatingHistoryRepository;
import kz.bardak.rating.domain.Season;
import kz.bardak.rating.domain.SeasonRepository;
import kz.bardak.rating.domain.UserRating;
import kz.bardak.rating.domain.UserRatingRepository;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

/** Чтение рейтинга и управление сезонами. Пересчёт живёт в {@link MatchResultService}. */
@Service
public class RatingService {

    private final UserRatingRepository ratings;
    private final RatingHistoryRepository histories;
    private final SeasonRepository seasons;
    private final UserRepository users;
    private final RatingProperties properties;
    private final Clock clock;

    public RatingService(final UserRatingRepository ratings, final RatingHistoryRepository histories,
                         final SeasonRepository seasons, final UserRepository users,
                         final RatingProperties properties, final Clock clock) {
        this.ratings = Objects.requireNonNull(ratings, "ratings");
        this.histories = Objects.requireNonNull(histories, "histories");
        this.seasons = Objects.requireNonNull(seasons, "seasons");
        this.users = Objects.requireNonNull(users, "users");
        this.properties = Objects.requireNonNull(properties, "properties");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    /**
     * Рейтинг игрока с графиком.
     *
     * <p>Не игравший ни разу рейтинга в базе не имеет — и это не ошибка: ему показывается
     * стартовое значение, а строка появится после первого матча.
     */
    @Transactional(readOnly = true)
    public RatingDtos.RatingView of(final UUID userId) {
        final User user = users.findById(userId).orElseThrow(() ->
                new ApiException(HttpStatus.NOT_FOUND, "USER_NOT_FOUND", "Такого игрока нет"));
        final UserRating rating = ratings.findById(userId).orElse(null);
        final List<RatingDtos.RatingPoint> history = histories
                .findByUserIdOrderByCreatedAtDesc(userId).stream()
                .map(this::toPoint)
                .toList();
        return new RatingDtos.RatingView(userId, user.displayName(),
                rating == null ? UserRating.INITIAL : rating.rating(),
                rating == null ? 0 : rating.matchesPlayed(), history);
    }

    @Transactional(readOnly = true)
    public List<RatingDtos.LeaderRow> leaderboard() {
        final List<UserRating> rows = ratings.findAllByOrderByRatingDesc();
        return rows.stream()
                .map(row -> new RatingDtos.LeaderRow(row.userId(),
                        users.findById(row.userId()).map(User::displayName).orElse("—"),
                        row.rating(), row.matchesPlayed()))
                .toList();
    }

    @Transactional(readOnly = true)
    public List<RatingDtos.SeasonView> seasons() {
        return seasons.findAllByOrderByStartedAtDesc().stream().map(this::toView).toList();
    }

    /**
     * Закрыть текущий сезон и открыть следующий.
     *
     * <p>⭐ Одним действием: открытый сезон должен быть всегда, иначе матчи между закрытием
     * и открытием осели бы вне сезонов, и заметили бы это сильно позже.
     */
    @Transactional
    public RatingDtos.SeasonView closeAndOpen(final String username, final String nextName) {
        if (!properties.isSeasonAdmin(username)) {
            throw new ApiException(HttpStatus.FORBIDDEN, "NOT_SEASON_ADMIN",
                    "Сезон закрывает не каждый");
        }
        seasons.findFirstByClosedAtIsNull().ifPresent(open -> {
            open.close(clock.instant());
            seasons.save(open);
        });
        return toView(seasons.save(Season.open(UUID.randomUUID(), nextName, clock.instant())));
    }

    private RatingDtos.SeasonView toView(final Season season) {
        return new RatingDtos.SeasonView(season.id(), season.name(), season.startedAt(),
                season.closedAt(), season.isOpen());
    }

    private RatingDtos.RatingPoint toPoint(final RatingHistoryEntry entry) {
        return new RatingDtos.RatingPoint(entry.matchId(), entry.ratingBefore(), entry.ratingAfter(),
                entry.place(), entry.playersCount(), entry.createdAt());
    }
}
