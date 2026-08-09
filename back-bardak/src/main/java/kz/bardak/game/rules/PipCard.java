package kz.bardak.game.rules;

import java.util.Objects;

/**
 * Обычная карта — ранг и масть. Таких в колоде 36 при любом числе игроков (§1.1).
 */
public record PipCard(Rank rank, Suit suit) implements Card {

    public PipCard {
        Objects.requireNonNull(rank, "rank");
        Objects.requireNonNull(suit, "suit");
    }

    public static PipCard of(final Rank rank, final Suit suit) {
        return new PipCard(rank, suit);
    }

    @Override
    public boolean sameRankAs(final Card other) {
        Objects.requireNonNull(other, "other");
        return other instanceof PipCard pip && pip.rank() == rank;
    }

    @Override
    public String code() {
        return rank.code() + suit.symbol();
    }
}
