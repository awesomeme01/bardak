package kz.bardak.game.rules;

import java.util.List;
import java.util.Objects;
import java.util.Optional;

/**
 * Личная шкала навесов: {@code 6 → 7 → … → A → Joker} (§2.3). Счёта в очках в игре нет —
 * шкала и есть счёт (ADR-017).
 *
 * <p>Уровень игрока кодируется индексом:
 * <pre>
 *   -1            навесов ещё не было, летит первая ступень
 *   0 … size-1    навешен ranks[level]
 *   size          навешен джокер — игрок проиграл (§0.2)
 * </pre>
 *
 * <p>Длина шкалы — параметр стола: укоротить её до {@code 9…A} должно быть правкой конфига,
 * а не кода (§1.6).
 *
 * @param ranks ступени по возрастанию; джокер — терминальная ступень сверх списка
 */
public record NavesScale(List<Rank> ranks) {

    /** Уровень игрока, которому ещё ничего не навешивали. */
    public static final int NO_NAVES = -1;

    public NavesScale {
        ranks = List.copyOf(Objects.requireNonNull(ranks, "ranks"));
        if (ranks.isEmpty()) {
            throw new IllegalArgumentException("Шкала навесов не может быть пустой");
        }
    }

    /** Полная шкала из всех девяти рангов — как играют вживую (OQ-13). */
    public static NavesScale full() {
        return new NavesScale(List.of(Rank.values()));
    }

    /** Уровень, после которого следующая ступень — джокер. */
    public int jokerLevel() {
        return ranks.size();
    }

    /** Джокер уже навешен: игрок проиграл, навешивать ему больше нечего. */
    public boolean isFinished(final int level) {
        return level >= jokerLevel();
    }

    /** Следующая ступень — джокер, а не обычный ранг. */
    public boolean nextIsJoker(final int level) {
        return level + 1 == jokerLevel();
    }

    /**
     * Ранг, который сейчас «летит» игроку. Пусто, если следующая ступень — джокер либо
     * шкала уже пройдена.
     */
    public Optional<Rank> nextRank(final int level) {
        if (level + 1 >= jokerLevel()) {
            return Optional.empty();
        }
        return Optional.of(ranks.get(level + 1));
    }

    /** Подходит ли карта под текущую ступень жертвы. Масть значения не имеет (§2.3). */
    public boolean isFlyingCard(final int level, final Card card) {
        Objects.requireNonNull(card, "card");
        if (isFinished(level)) {
            return false;
        }
        if (nextIsJoker(level)) {
            return card instanceof JokerCard;
        }
        return card instanceof PipCard pip && nextRank(level).filter(pip.rank()::equals).isPresent();
    }
}
