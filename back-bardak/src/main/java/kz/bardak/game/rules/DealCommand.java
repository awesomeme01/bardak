package kz.bardak.game.rules;

import java.util.Objects;

/**
 * Намерение игрока. Клиент присылает намерение, решает сервер (ADR-003), поэтому команда
 * не несёт ничего, кроме места и карт: всё остальное движок берёт из состояния.
 *
 * <p>У игрока одно основное действие — положить карту на стол (§2.1); смысл действия
 * задаёт роль, поэтому здесь оно разложено на три разные команды, а не на одну с флагом.
 */
public sealed interface DealCommand
        permits DealCommand.Attack, DealCommand.Defend, DealCommand.Transfer,
        DealCommand.Pass, DealCommand.Take, DealCommand.HangCard, DealCommand.HangSkip,
        DealCommand.ChooseTrump, DealCommand.RevealFaceDown, DealCommand.RevealFaceDownToDefend {

    int seatNo();

    /** Положить карту в атаку — первой картой раунда или подкидом. */
    record Attack(int seatNo, Card card) implements DealCommand {

        public Attack {
            Objects.requireNonNull(card, "card");
        }
    }

    /**
     * Отбить конкретную атакующую карту. Цель обязательна: при нескольких картах на столе
     * иначе не зафиксировать, что чем отбито (§2.1).
     */
    record Defend(int seatNo, Card card, Card target) implements DealCommand {

        public Defend {
            Objects.requireNonNull(card, "card");
            Objects.requireNonNull(target, "target");
        }
    }

    /** Перевести атаку дальше по кругу (§2.2). */
    record Transfer(int seatNo, Card card) implements DealCommand {

        public Transfer {
            Objects.requireNonNull(card, "card");
        }
    }

    /**
     * «Пас» — явная фиксация того, что игрок больше не подкидывает. Раунд не завершается
     * сам по себе, пока обладатель права не спасовал (§2.1).
     */
    record Pass(int seatNo) implements DealCommand {
    }

    /** «Взял» — защищающийся забирает стол в руку. */
    record Take(int seatNo) implements DealCommand {
    }

    /**
     * «Навесить» — заявка в открытом окне (§2.3). Карта называется сразу: иначе после
     * броска кости победитель мог бы передумать, а заявка должна быть обязательством.
     */
    record HangCard(int seatNo, Card card) implements DealCommand {

        public HangCard {
            Objects.requireNonNull(card, "card");
        }
    }

    /** «Пропустить» — навес всегда выбор, даже когда карта есть. */
    record HangSkip(int seatNo) implements DealCommand {
    }

    /**
     * ⭐ Вскрыть скрытую карту и пойти ею (§1.8). Карта не называется: игрок сам её не
     * видит (ADR-026). Вскрытие необратимо и происходит даже тогда, когда ход ею не
     * проходит по рангу, — тогда карта просто остаётся в руке.
     */
    record RevealFaceDown(int seatNo) implements DealCommand {
    }

    /** То же для защиты: вскрыть скрытую карту и попробовать побить ею цель. */
    record RevealFaceDownToDefend(int seatNo, Card target) implements DealCommand {

        public RevealFaceDownToDefend {
            Objects.requireNonNull(target, "target");
        }
    }

    /**
     * ⭐ Победитель кости называет козырную масть, когда нижней картой колоды оказался
     * джокер (§1.2). Это команда, а не вычисление: победитель именно <b>выбирает</b>,
     * глядя в свои карты, и может назвать масть, которой у него нет.
     */
    record ChooseTrump(int seatNo, Suit suit) implements DealCommand {

        public ChooseTrump {
            Objects.requireNonNull(suit, "suit");
        }
    }
}
