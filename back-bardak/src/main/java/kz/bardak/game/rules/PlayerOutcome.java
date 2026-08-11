package kz.bardak.game.rules;

import java.util.List;
import java.util.Objects;
import java.util.Optional;

/**
 * Итог раздачи для одного игрока.
 *
 * <p>⭐ Итог самодостаточен: раздача после подсчёта исчезает — карты собираются в колоду,
 * и следующая сдача занимает её место. Всё, что нужно истории, обязано лежать здесь,
 * иначе восстановить прошлое можно будет только переигрыванием всего матча.
 *
 * @param seatNo      место за столом
 * @param levelBefore уровень до подсчёта
 * @param levelAfter  уровень после всех сдвигов и нижней границы
 * @param lossDegree  степень проигрыша или {@code null}, если игрок не проиграл игру
 * @param place       место в раздаче: вышедший первым — первый, оставшийся с картами —
 *                    последний. {@link #UNPLACED}, когда итог собран не движком
 * @param hungCards   что ему навесили в этой раздаче
 * @param changes     из чего сложился сдвиг уровня: слагаемые, а не сумма
 */
public record PlayerOutcome(int seatNo, int levelBefore, int levelAfter, LossDegree lossDegree,
                            int place, List<Card> hungCards, List<LevelChange> changes) {

    /** Места нет: так выглядит итог, собранный вручную в тесте, а не движком. */
    public static final int UNPLACED = 0;

    public PlayerOutcome {
        hungCards = List.copyOf(Objects.requireNonNull(hungCards, "hungCards"));
        changes = List.copyOf(Objects.requireNonNull(changes, "changes"));
    }

    public PlayerOutcome(final int seatNo, final int levelBefore, final int levelAfter,
                         final LossDegree lossDegree) {
        this(seatNo, levelBefore, levelAfter, lossDegree, UNPLACED, List.of(), List.of());
    }

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
