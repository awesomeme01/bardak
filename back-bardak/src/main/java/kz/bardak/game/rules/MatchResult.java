package kz.bardak.game.rules;

import java.util.List;
import java.util.Objects;

/**
 * Итог команды на уровне матча. Как и в раздаче, отказ возвращается без состояния:
 * отклонённая команда не меняет ничего (§4.2).
 */
public sealed interface MatchResult permits MatchResult.Applied, MatchResult.Rejected {

    static MatchResult applied(final MatchState state, final List<DealEvent> events) {
        return new Applied(state, events);
    }

    static MatchResult rejected(final RejectionReason reason) {
        return new Rejected(reason);
    }

    default boolean isApplied() {
        return this instanceof Applied;
    }

    record Applied(MatchState state, List<DealEvent> events) implements MatchResult {

        public Applied {
            Objects.requireNonNull(state, "state");
            events = List.copyOf(Objects.requireNonNull(events, "events"));
        }
    }

    record Rejected(RejectionReason reason) implements MatchResult {

        public Rejected {
            Objects.requireNonNull(reason, "reason");
        }
    }
}
