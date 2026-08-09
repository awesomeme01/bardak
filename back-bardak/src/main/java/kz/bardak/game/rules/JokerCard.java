package kz.bardak.game.rules;

/**
 * Джокер. В колоде их ровно по одному на игрока (§3) — {@code number} различает экземпляры
 * для лога и для выбора картинки ({@code Joker-1} … {@code Joker-5}), но на правила
 * не влияет: старшинства между джокерами нет (ADR-032).
 */
public record JokerCard(int number) implements Card {

    public JokerCard {
        if (number < 1) {
            throw new IllegalArgumentException("Номер джокера начинается с 1, получен: " + number);
        }
    }

    @Override
    public boolean sameRankAs(final Card other) {
        return other instanceof JokerCard;
    }

    @Override
    public String code() {
        return "Joker-" + number;
    }
}
