package kz.bardak.game.rules;

import java.util.Objects;

/**
 * Потайной козырь оказался джокером и ждёт броска кости (§1.9, ADR-035).
 *
 * <p>⭐ Порядок здесь существенный: <b>сначала кость и выбор масти, потом карта в руку</b>.
 * Поэтому она и лежит отдельно — не в колоде, которая уже пуста, и не в руке добирающего,
 * который иначе выбирал бы козырь, уже держа её.
 *
 * @param card          сам джокер
 * @param recipientSeat кому он достанется после выбора масти
 * @param chooserSeat   победитель кости — он называет масть
 */
public record PendingHiddenTrump(Card card, int recipientSeat, int chooserSeat) {

    public PendingHiddenTrump {
        Objects.requireNonNull(card, "card");
    }
}
