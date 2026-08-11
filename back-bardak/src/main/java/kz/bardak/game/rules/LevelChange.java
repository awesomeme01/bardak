package kz.bardak.game.rules;

import java.util.Objects;

/**
 * Один сдвиг уровня навесов с причиной.
 *
 * @param reason почему сдвинули
 * @param amount на сколько ступеней; отрицательное значение — вниз по шкале
 */
public record LevelChange(LevelChangeReason reason, int amount) {

    public LevelChange {
        Objects.requireNonNull(reason, "reason");
    }
}
