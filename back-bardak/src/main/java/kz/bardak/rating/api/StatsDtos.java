package kz.bardak.rating.api;

import java.math.BigDecimal;
import java.util.List;

/** Формы статистики игрока. */
public final class StatsDtos {

    private StatsDtos() {
    }

    /**
     * @param matches       сыгранных матчей; отменённые не в счёт (§5.3)
     * @param avgPlace      среднее место — честнее, чем «побед», когда за столом пятеро
     * @param dealsPlayed   сколько раздач сыграно за все матчи
     * @param streak        текущая серия: подряд выигранных или подряд проигранных
     * @param bestRating    высшая точка рейтинга; {@code null}, если история пуста
     * @param degrees       чем именно заканчивались проигрыши, от тяжёлой степени к обычной
     */
    public record PlayerStats(int matches, int wins, int losses, BigDecimal avgPlace,
                              int dealsPlayed, Streak streak, BigDecimal bestRating,
                              BigDecimal worstRating, List<DegreeCount> degrees) {

        public static PlayerStats empty() {
            return new PlayerStats(0, 0, 0, null, 0, new Streak("NONE", 0), null, null, List.of());
        }
    }

    /** @param kind WIN, LOSS или NONE — до первого матча серии нет вовсе */
    public record Streak(String kind, int length) {
    }

    public record DegreeCount(String degree, int count) {
    }
}
