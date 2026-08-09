package kz.bardak.game.rules;

import java.util.Objects;

/**
 * Заявка на навес: игрок нажал «Навесить» в окне. Карта названа сразу — иначе после броска
 * кости победитель мог бы передумать, а заявка должна быть обязательством.
 */
public record HangClaim(int seatNo, Card card) {

    public HangClaim {
        Objects.requireNonNull(card, "card");
    }
}
