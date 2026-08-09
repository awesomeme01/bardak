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
        DealCommand.Pass, DealCommand.Take {

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
}
