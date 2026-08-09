package kz.bardak.game.rules;

/**
 * Ранг обычной карты. Старшинство задано порядком объявления: {@code 6 < 7 < … < A}
 * (§1.1 правил). Джокер рангом не является — у него собственный ранг вне этой шкалы
 * (ADR-032), см. {@link JokerCard}.
 */
public enum Rank {

    SIX("6"),
    SEVEN("7"),
    EIGHT("8"),
    NINE("9"),
    TEN("10"),
    JACK("J"),
    QUEEN("Q"),
    KING("K"),
    ACE("A");

    private final String code;

    Rank(final String code) {
        this.code = code;
    }

    public String code() {
        return code;
    }

    /**
     * Строго старше другого ранга. Сравнение осмысленно только внутри одной масти —
     * межмастевое старшинство определяет {@link Trump}.
     */
    public boolean isHigherThan(final Rank other) {
        return ordinal() > other.ordinal();
    }
}
