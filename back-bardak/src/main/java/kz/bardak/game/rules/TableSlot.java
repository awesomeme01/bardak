package kz.bardak.game.rules;

import java.util.Objects;
import java.util.Optional;

/**
 * Пара «атакующая карта — чем отбита» на столе. Защищающийся всегда указывает, какую именно
 * карту он бьёт (§2.1), поэтому стол — это список пар, а не две кучи.
 *
 * @param attack  карта атаки, всегда есть
 * @param defence чем отбита; {@code null}, пока карта не бита
 */
public record TableSlot(Card attack, Card defence) {

    public TableSlot {
        Objects.requireNonNull(attack, "attack");
    }

    public static TableSlot of(final Card attack) {
        return new TableSlot(attack, null);
    }

    public boolean isBeaten() {
        return defence != null;
    }

    public Optional<Card> defenceCard() {
        return Optional.ofNullable(defence);
    }

    /** Карта, побившая атаку. */
    public TableSlot beatenWith(final Card card) {
        Objects.requireNonNull(card, "card");
        if (isBeaten()) {
            throw new IllegalStateException("Карта %s уже бита картой %s"
                    .formatted(attack.code(), defence.code()));
        }
        return new TableSlot(attack, card);
    }
}
