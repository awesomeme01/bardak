package kz.bardak.game.rules;

import java.util.List;
import java.util.Objects;

/**
 * Итог применения команды: либо новое состояние с событиями, либо отказ с причиной.
 *
 * <p>⭐ Отклонённый ход не меняет состояние вообще (§4.2) — поэтому у отказа состояния нет
 * даже в виде «того же самого»: вернуть его означало бы разрешить случайно его сохранить.
 */
public sealed interface MoveResult permits MoveResult.Applied, MoveResult.Rejected {

    static MoveResult applied(final DealState state, final List<DealEvent> events) {
        return new Applied(state, events);
    }

    static MoveResult rejected(final RejectionReason reason) {
        return new Rejected(reason);
    }

    static MoveResult from(final MoveVerdict verdict, final DealState state, final List<DealEvent> events) {
        Objects.requireNonNull(verdict, "verdict");
        if (verdict instanceof MoveVerdict.Rejected rejected) {
            return rejected(rejected.reason());
        }
        return applied(state, events);
    }

    default boolean isApplied() {
        return this instanceof Applied;
    }

    record Applied(DealState state, List<DealEvent> events) implements MoveResult {

        public Applied {
            Objects.requireNonNull(state, "state");
            events = List.copyOf(Objects.requireNonNull(events, "events"));
        }
    }

    record Rejected(RejectionReason reason) implements MoveResult {

        public Rejected {
            Objects.requireNonNull(reason, "reason");
        }
    }
}
