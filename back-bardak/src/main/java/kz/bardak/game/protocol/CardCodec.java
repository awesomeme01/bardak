package kz.bardak.game.protocol;

import java.util.Locale;
import java.util.Objects;
import kz.bardak.game.rules.Card;
import kz.bardak.game.rules.JokerCard;
import kz.bardak.game.rules.PipCard;
import kz.bardak.game.rules.Rank;
import kz.bardak.game.rules.Suit;

/**
 * Код карты в протоколе: {@code 6-diamonds}, {@code 10-hearts}, {@code Joker-1}.
 *
 * <p>⭐ Это единственное место, где движок встречается с внешним представлением. Коды —
 * неизменяемый контракт (ADR-009): на них завязаны манифесты наборов и весь исторический
 * лог, поэтому менять их нельзя даже «косметически».
 *
 * <p>Джокеры нумеруются: их в колоде несколько, и без номера нельзя сказать, какой именно
 * джокер играют. Картинка при этом у всех одна — за неё отвечает манифест, а не код.
 */
public final class CardCodec {

    private static final String JOKER_PREFIX = "Joker-";

    private CardCodec() {
    }

    public static String encode(final Card card) {
        Objects.requireNonNull(card, "card");
        if (card instanceof JokerCard joker) {
            return JOKER_PREFIX + joker.number();
        }
        final PipCard pip = (PipCard) card;
        return pip.rank().code() + "-" + pip.suit().name().toLowerCase(Locale.ROOT);
    }

    public static Card decode(final String code) {
        if (code == null || code.isBlank()) {
            throw new IllegalArgumentException("Пустой код карты");
        }
        if (code.startsWith(JOKER_PREFIX)) {
            return new JokerCard(parseJokerNumber(code));
        }
        final int separator = code.lastIndexOf('-');
        if (separator < 0) {
            throw new IllegalArgumentException("Неизвестный код карты: " + code);
        }
        return PipCard.of(rankOf(code.substring(0, separator)), suitOf(code.substring(separator + 1)));
    }

    private static int parseJokerNumber(final String code) {
        try {
            return Integer.parseInt(code.substring(JOKER_PREFIX.length()));
        } catch (final NumberFormatException e) {
            throw new IllegalArgumentException("Неизвестный код джокера: " + code, e);
        }
    }

    private static Rank rankOf(final String code) {
        for (final Rank rank : Rank.values()) {
            if (rank.code().equals(code)) {
                return rank;
            }
        }
        throw new IllegalArgumentException("Неизвестный ранг: " + code);
    }

    private static Suit suitOf(final String code) {
        for (final Suit suit : Suit.values()) {
            if (suit.name().equalsIgnoreCase(code)) {
                return suit;
            }
        }
        throw new IllegalArgumentException("Неизвестная масть: " + code);
    }
}
