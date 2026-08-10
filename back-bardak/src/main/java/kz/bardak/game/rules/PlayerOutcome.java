package kz.bardak.game.rules;

import java.util.Optional;

/**
 * Итог раздачи для одного игрока.
 *
 * @param seatNo      место за столом
 * @param levelBefore уровень до подсчёта
 * @param levelAfter  уровень после всех сдвигов и нижней границы
 * @param lossDegree  степень проигрыша или {@code null}, если игрок не проиграл игру
 */
public record PlayerOutcome(int seatNo, int levelBefore, int levelAfter, LossDegree lossDegree) {

    public Optional<LossDegree> degree() {
        return Optional.ofNullable(lossDegree);
    }

    public boolean isLoser() {
        return lossDegree != null;
    }

    /** На сколько ступеней сдвинулся уровень: отрицательное значение — удачная раздача. */
    public int shift() {
        return levelAfter - levelBefore;
    }
}
