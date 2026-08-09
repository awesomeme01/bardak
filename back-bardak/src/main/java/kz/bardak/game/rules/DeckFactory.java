package kz.bardak.game.rules;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Random;

/**
 * Сборка колоды раздачи: 36 обычных карт плюс по одному джокеру на игрока (§1.1, §3).
 *
 * <p>Состав не зависит от числа игроков сверх этого: OQ-1 закрыт в пользу {@code 36 + N}
 * при любом столе. Короткая раздача на пятерых — штатный сценарий, а не вырождение.
 *
 * <p>⭐ Перемешивание детерминировано: одна и та же пара {@code (playerCount, seed)} даёт
 * один и тот же порядок карт. Без этого невозможен реплей матча (§6). Сам {@code seed}
 * рождается из {@code SecureRandom} на уровне матча — здесь он уже данность.
 */
public final class DeckFactory {

    /** Границы стола. Переедут в {@code rules_config}, когда появится конфиг стола (§1.6). */
    private static final int MIN_PLAYERS = 2;
    private static final int MAX_PLAYERS = 5;

    /**
     * Колода в каноническом порядке: все обычные карты по мастям и рангам, затем джокеры.
     * Нужна для тестов и для проверки состава — играть полагается перемешанной.
     */
    public List<Card> buildOrdered(final int playerCount) {
        validatePlayerCount(playerCount);
        final List<Card> cards = new ArrayList<>();
        for (final Suit suit : Suit.values()) {
            for (final Rank rank : Rank.values()) {
                cards.add(PipCard.of(rank, suit));
            }
        }
        for (int number = 1; number <= playerCount; number++) {
            cards.add(new JokerCard(number));
        }
        return List.copyOf(cards);
    }

    /**
     * Колода, перемешанная генератором от {@code seed}. Порядок воспроизводим.
     */
    public List<Card> buildShuffled(final int playerCount, final long seed) {
        final List<Card> cards = new ArrayList<>(buildOrdered(playerCount));
        Collections.shuffle(cards, new Random(seed));
        return List.copyOf(cards);
    }

    private void validatePlayerCount(final int playerCount) {
        if (playerCount < MIN_PLAYERS || playerCount > MAX_PLAYERS) {
            throw new IllegalArgumentException(
                    "Игроков за столом должно быть от %d до %d, получено: %d"
                            .formatted(MIN_PLAYERS, MAX_PLAYERS, playerCount));
        }
    }
}
