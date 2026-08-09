package kz.bardak.game.rules;

import java.util.ArrayList;
import java.util.List;
import java.util.OptionalInt;

/**
 * Кому переходит право подкидывать после паса. Вынесено в стратегию, чтобы правило менялось
 * без правки автомата (§2.1): у бардака оно жёстче классического и вполне может быть
 * настройкой стола.
 */
public interface AttackOrderPolicy {

    /**
     * Следующий обладатель права подкидывать или пусто, если подкидывать больше некому
     * и раунд пора закрывать.
     */
    OptionalInt nextAttacker(DealState state);

    /**
     * Боевое правило бардака: подкидывает начавший раунд, после его паса — второй сосед,
     * то есть следующий по часовой за защищающимся. Право назад не возвращается, остальные
     * не подкидывают вообще.
     */
    final class BardakStrictNeighbours implements AttackOrderPolicy {

        @Override
        public OptionalInt nextAttacker(final DealState state) {
            for (final int seat : candidates(state)) {
                if (isEligible(state, seat)) {
                    return OptionalInt.of(seat);
                }
            }
            return OptionalInt.empty();
        }

        /**
         * Порядок очереди. Второй сосед считается от защищающегося, а не от начавшего раунд,
         * — поэтому перевод, сдвигая защиту по кругу, меняет и состав подкидывающих
         * (ADR-031).
         */
        private List<Integer> candidates(final DealState state) {
            final List<Integer> seats = new ArrayList<>();
            seats.add(state.roundStarterSeat());
            final int secondNeighbour = state.nextActiveSeatAfter(state.defenderSeat());
            if (!seats.contains(secondNeighbour)) {
                seats.add(secondNeighbour);
            }
            return seats;
        }

        private boolean isEligible(final DealState state, final int seat) {
            return seat != state.defenderSeat()
                    && state.playerAt(seat).inDeal()
                    && !state.hasPassed(seat);
        }
    }
}
