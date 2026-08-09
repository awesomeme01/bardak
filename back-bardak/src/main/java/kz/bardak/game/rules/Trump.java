package kz.bardak.game.rules;

import java.util.Objects;

/**
 * Козырь раздачи и вытекающая из него защищённая масть — вместе, потому что порознь они
 * бессмысленны: защищённая масть определяется козырем и меняется вместе с ним
 * (§1.1.1, §1.9).
 *
 * <p>⭐ Базовое допущение дурака «козырь бьёт всё» здесь неверно. Защищённую масть козырь
 * не берёт, поэтому старшинство — отдельная функция {@link #beats(Card, Card)}, а не
 * выражение из двух условий.
 */
public record Trump(Suit suit) {

    /** Защищённая масть по умолчанию — пики. */
    private static final Suit DEFAULT_PROTECTED_SUIT = Suit.SPADES;

    /** Если козырь сам оказался пиками, роль защищённой масти переходит к трефам. */
    private static final Suit FALLBACK_PROTECTED_SUIT = Suit.CLUBS;

    public Trump {
        Objects.requireNonNull(suit, "suit");
    }

    public static Trump of(final Suit suit) {
        return new Trump(suit);
    }

    /**
     * Масть, которую козырь не берёт: пики, а при козырных пиках — трефы (§1.1.1).
     * Защищённая масть есть всегда, и она всегда одна.
     */
    public Suit protectedSuit() {
        return suit == DEFAULT_PROTECTED_SUIT ? FALLBACK_PROTECTED_SUIT : DEFAULT_PROTECTED_SUIT;
    }

    /**
     * Бьёт ли карта защиты карту атаки (§1.1.1, §3.1). Правило применяется исключительно
     * в момент защиты: на атаку, подкидывание и перевод защищённая масть не влияет.
     *
     * @param defence карта, которой пытаются отбиться
     * @param attack  карта на столе, которую пытаются побить
     */
    public boolean beats(final Card defence, final Card attack) {
        Objects.requireNonNull(defence, "defence");
        Objects.requireNonNull(attack, "attack");
        if (defence instanceof JokerCard) {
            return true;
        }
        if (attack instanceof JokerCard) {
            return false;
        }
        final PipCard defencePip = (PipCard) defence;
        final PipCard attackPip = (PipCard) attack;
        if (defencePip.suit() == attackPip.suit()) {
            return defencePip.rank().isHigherThan(attackPip.rank());
        }
        if (attackPip.suit() == protectedSuit()) {
            return false;
        }
        return defencePip.suit() == suit;
    }
}
