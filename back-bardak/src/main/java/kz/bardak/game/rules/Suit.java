package kz.bardak.game.rules;

/**
 * Масть обычной карты. Порядок объявления значения не имеет — масти между собой не
 * сравниваются, старшинство существует только внутри одной масти ({@link Rank}).
 */
public enum Suit {

    DIAMONDS("♦"),
    HEARTS("♥"),
    SPADES("♠"),
    CLUBS("♣");

    private final String symbol;

    Suit(final String symbol) {
        this.symbol = symbol;
    }

    public String symbol() {
        return symbol;
    }
}
