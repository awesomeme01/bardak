package kz.bardak.rating;

import java.util.ArrayList;
import java.util.List;
import java.util.Objects;

/**
 * Elo с попарным разложением (`07-rating-system.md`).
 *
 * <p>⭐ Классический Elo придуман для двоих с бинарным исходом, а у нас 2–5 игроков
 * и результат — <b>ранжирование</b>. Матч разбирается на все пары: у кого место лучше,
 * тот «выиграл» у другого.
 *
 * <p>⭐ Деление на {@code N-1} обязательно. Без него матч на пятерых двигал бы рейтинг
 * вчетверо сильнее, чем матч на двоих, — при том что игра та же.
 *
 * <p>Класс чистый: ни базы, ни Spring. Формулу так можно проверить на бумаге.
 */
public final class EloRating {

    /** Новичок быстро находит свой уровень. */
    private static final double K_NEWCOMER = 40;
    private static final double K_REGULAR = 20;
    /** Наверху шкала плотная: резкие скачки там означали бы шум, а не силу. */
    private static final double K_TOP = 10;

    private static final int NEWCOMER_MATCHES = 20;
    private static final double TOP_RATING = 2200;
    /** В дураке сильна роль карт, поэтому рейтинг не должен уходить в минус от серии неудач. */
    private static final double MIN_RATING = 100;

    private EloRating() {
    }

    /**
     * Новые рейтинги участников матча.
     *
     * @param players участники: текущий рейтинг, число сыгранных матчей и итоговое место
     * @return новые рейтинги в том же порядке
     */
    public static List<Double> recalculate(final List<Participant> players) {
        Objects.requireNonNull(players, "players");
        if (players.size() < 2) {
            throw new IllegalArgumentException("Рейтинг считается минимум для двоих");
        }
        final List<Double> updated = new ArrayList<>(players.size());
        for (final Participant player : players) {
            double delta = 0;
            for (final Participant other : players) {
                if (other == player) {
                    continue;
                }
                final double expected = 1 / (1 + Math.pow(10, (other.rating() - player.rating()) / 400));
                final double actual = player.place() < other.place() ? 1 : 0;
                delta += kFor(player) * (actual - expected);
            }
            updated.add(Math.max(MIN_RATING, player.rating() + delta / (players.size() - 1)));
        }
        return List.copyOf(updated);
    }

    private static double kFor(final Participant player) {
        if (player.rating() > TOP_RATING) {
            return K_TOP;
        }
        return player.matchesPlayed() < NEWCOMER_MATCHES ? K_NEWCOMER : K_REGULAR;
    }

    /**
     * @param place итоговое место, с первого; получивший джокер — последний
     */
    public record Participant(double rating, int matchesPlayed, int place) {
    }
}
