package kz.bardak.game.rules;

import java.util.List;
import java.util.Objects;
import java.util.Random;

/**
 * Бросок кости — общая подсистема на все четыре повода (§1.10, ADR-030): козырь-джокер,
 * потайной козырь, спор за джокер и спор за обычный навес.
 *
 * <p>Вынесено в интерфейс не ради подмены правила, а ради тестов: движок обязан оставаться
 * детерминированным (§6), поэтому случайность не берётся из воздуха, а выводится из seed
 * раздачи и номера броска.
 */
public interface DiceResolver {

    /**
     * Кто выиграл бросок среди участников спора.
     *
     * @param seats  участники, в стабильном порядке
     * @param seed   seed раздачи
     * @param rollNo номер броска внутри раздачи — два спора подряд не должны давать
     *               одинаковый результат
     */
    int winnerAmong(List<Integer> seats, long seed, int rollNo);

    /**
     * Шестигранная кость от seed раздачи. Переброс при ничьей отдельно моделировать не нужно:
     * результат здесь уже единственный, а сам факт равных бросков — деталь показа в UI.
     */
    final class Seeded implements DiceResolver {

        @Override
        public int winnerAmong(final List<Integer> seats, final long seed, final int rollNo) {
            Objects.requireNonNull(seats, "seats");
            if (seats.isEmpty()) {
                throw new IllegalArgumentException("Бросок кости без участников");
            }
            final Random random = new Random(seed * 31L + rollNo);
            return seats.get(random.nextInt(seats.size()));
        }
    }
}
